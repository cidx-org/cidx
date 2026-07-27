package main

import (
	"runtime/debug"
	"testing"
)

// stubBuildInfo replaces the debug.ReadBuildInfo seam for one test and restores
// it afterwards. An empty version means "no build info available".
func stubBuildInfo(t *testing.T, moduleVersion string, ok bool) {
	t.Helper()
	old := readBuildInfo
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		if !ok {
			return nil, false
		}
		return &debug.BuildInfo{Main: debug.Module{Version: moduleVersion}}, true
	}
	t.Cleanup(func() { readBuildInfo = old })
}

// TestResolveVersion locks in the version resolution for every install path.
//
// The case that matters for issue #205 is the go-installed binary: no ldflags,
// but the module version is recorded in the binary, so cidx must report it —
// otherwise generate.BootstrapVersion() sees "dev" and emits `@latest` instead
// of pinning the running release.
func TestResolveVersion(t *testing.T) {
	tests := []struct {
		name       string
		ldflags    string
		buildInfo  string
		hasInfo    bool
		want       string
		wantReason string
	}{
		{
			name:       "ldflags version wins",
			ldflags:    "2.1.2",
			buildInfo:  "v9.9.9",
			hasInfo:    true,
			want:       "2.1.2",
			wantReason: "release build: ldflags are authoritative, build info is never consulted",
		},
		{
			name:       "go install from a release tag",
			ldflags:    "dev",
			buildInfo:  "v2.1.2",
			hasInfo:    true,
			want:       "2.1.2",
			wantReason: "no ldflags, but the module version is the real release (#205)",
		},
		{
			name:       "go run from sources",
			ldflags:    "dev",
			buildInfo:  "(devel)",
			hasInfo:    true,
			want:       "dev",
			wantReason: "source build: keep the dev fallback (@latest + warning)",
		},
		{
			name:       "no build info",
			ldflags:    "dev",
			hasInfo:    false,
			want:       "dev",
			wantReason: "nothing to resolve from",
		},
		{
			name:       "empty module version",
			ldflags:    "dev",
			buildInfo:  "",
			hasInfo:    true,
			want:       "dev",
			wantReason: "nothing to resolve from",
		},
		{
			name:      "go build from sources (VCS pseudo-version)",
			ldflags:   "dev",
			buildInfo: "v2.1.3-0.20260727135606-daea424bf280+dirty",
			hasInfo:   true,
			want:      "dev",
			wantReason: "since Go 1.24 `go build` stamps a pseudo-version; it is not an " +
				"installable release, so the dev fallback must stay intact",
		},
		{
			name:      "go install from a branch (pseudo-version)",
			ldflags:   "dev",
			buildInfo: "v2.1.3-0.20260727135606-daea424bf280",
			hasInfo:   true,
			want:      "dev",
			wantReason: "a pseudo-version is not a release a third party can install " +
				"meaningfully: keep `@latest` + the dev-build warning",
		},
		{
			name:       "pre-release tag",
			ldflags:    "dev",
			buildInfo:  "v2.1.3-rc.1",
			hasInfo:    true,
			want:       "dev",
			wantReason: "only clean release versions pin the bootstrap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubBuildInfo(t, tt.buildInfo, tt.hasInfo)

			if got := resolveVersion(tt.ldflags); got != tt.want {
				t.Errorf("resolveVersion(%q) with build info %q = %q, want %q (%s)",
					tt.ldflags, tt.buildInfo, got, tt.want, tt.wantReason)
			}
		})
	}
}
