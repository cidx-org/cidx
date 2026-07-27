package main

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// readBuildInfo is a seam for tests; production code always reads the real
// build info embedded in the binary.
var readBuildInfo = debug.ReadBuildInfo

// buildInfoReleaseRe matches the module version debug.ReadBuildInfo reports for
// a binary installed from a release tag ("v2.1.2"). Everything else is not an
// installable release: "(devel)" for `go run`, and the VCS pseudo-version
// `go build` stamps since Go 1.24 ("v2.1.3-0.20260727135606-daea424bf280+dirty").
var buildInfoReleaseRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// resolveVersion returns the version cidx reports at runtime.
//
// Release binaries carry it in ldflags (`-X main.Version=2.1.2`). `go install
// github.com/cidx-org/cidx/v2/cmd/cidx@v2.1.2` applies no ldflags, so such a
// binary reported "dev" and generated workflows fell back to `@latest` instead
// of pinning the running release (issue #205). The module version recorded in
// the binary is the real one, so use it — normalized to the ldflags convention
// (no leading "v"), which is what config.CheckVersion and BootstrapVersion
// already expect.
//
// Anything that is not a clean release tag keeps "dev", so the existing
// dev-build behavior stays intact: `@latest` bootstrap plus warning
// (generate.BootstrapVersion) and the required_version bypass
// (config.CheckVersion).
func resolveVersion(ldflagsVersion string) string {
	if ldflagsVersion != "dev" {
		return ldflagsVersion
	}

	info, ok := readBuildInfo()
	if !ok || !buildInfoReleaseRe.MatchString(info.Main.Version) {
		return "dev"
	}

	return strings.TrimPrefix(info.Main.Version, "v")
}
