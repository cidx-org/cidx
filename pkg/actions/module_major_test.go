package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withGoMod writes a go.mod declaring path and returns its directory.
func withGoMod(t *testing.T, path string) string {
	t.Helper()

	dir := t.TempDir()
	content := "module " + path + "\n\ngo 1.26\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("writing go.mod: %v", err)
	}

	return dir
}

// TestCheckModuleMajor covers issue #395, and #187 before it: the module path
// and the tag's major have to agree, or the tag publishes a major nobody can
// import.
func TestCheckModuleMajor(t *testing.T) {
	tests := []struct {
		name    string
		module  string
		version string
		refused bool
	}{
		{
			// What v3.0.0 actually shipped: the path still said /v2, so
			// `go list -m .../v3` failed and every generated workflow's
			// `go install .../v2/cmd/cidx@latest` fetched the newest v2.
			name:    "a major tag on the previous major's path",
			module:  "github.com/cidx-org/cidx/v2",
			version: "3.0.0",
			refused: true,
		},
		{
			name:    "the path and the tag agree",
			module:  "github.com/cidx-org/cidx/v3",
			version: "3.0.0",
			refused: false,
		},
		{
			// The ordinary case, and the one that must never be interrupted:
			// a patch or minor on a matching path.
			name:    "a patch on a matching path",
			module:  "github.com/cidx-org/cidx/v3",
			version: "3.0.1",
			refused: false,
		},
		{
			// Go gives v0 and v1 the unsuffixed path. That is the convention,
			// not a missing suffix.
			name:    "v1 on an unsuffixed path",
			module:  "github.com/cidx-org/cidx",
			version: "1.4.0",
			refused: false,
		},
		{
			name:    "v0 on an unsuffixed path",
			module:  "github.com/cidx-org/cidx",
			version: "0.9.0",
			refused: false,
		},
		{
			// The mismatch in the other direction: still on the unsuffixed
			// path while tagging the first real major.
			name:    "v2 on an unsuffixed path",
			module:  "github.com/cidx-org/cidx",
			version: "2.0.0",
			refused: true,
		},
		{
			name:    "a leading v on the version is read the same way",
			module:  "github.com/cidx-org/cidx/v2",
			version: "v3.0.0",
			refused: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckModuleMajor(withGoMod(t, tt.module), tt.version)

			if tt.refused {
				if err == nil {
					t.Fatalf("tagging %s on %q was allowed", tt.version, tt.module)
				}
				for _, want := range []string{tt.module, tt.version} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("refusal = %q, want it to name %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("tagging %s on %q was refused: %v", tt.version, tt.module, err)
			}
		})
	}
}

// TestCheckModuleMajorIgnoresANonGoRepository: cidx tags repositories that are
// not Go modules, and this rule has nothing to say about them.
func TestCheckModuleMajorIgnoresANonGoRepository(t *testing.T) {
	if err := CheckModuleMajor(t.TempDir(), "3.0.0"); err != nil {
		t.Errorf("a repository with no go.mod was refused: %v", err)
	}
}

// TestCheckModuleMajorNamesThePathTheTagWouldNeed: the refusal has to be
// actionable, and what makes it actionable is the path to change to.
func TestCheckModuleMajorNamesThePathTheTagWouldNeed(t *testing.T) {
	err := CheckModuleMajor(withGoMod(t, "github.com/cidx-org/cidx/v2"), "3.0.0")
	if err == nil {
		t.Fatal("expected a refusal")
	}

	if !strings.Contains(err.Error(), "github.com/cidx-org/cidx/v3") {
		t.Errorf("refusal = %q, want it to name the path the tag requires", err)
	}
}
