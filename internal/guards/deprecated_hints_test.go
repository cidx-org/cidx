package guards

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `cidx action ...` tree is gone -- hidden on 2026-04-09, deprecated with a
// warning on every invocation from #235 on, removed in v3.0.0 -- so nothing the
// user can read may still point at it. Guards issue #174 against regressions:
// hints must name the current paths (`cidx release ...`, `cidx repo ...`, or
// the top-level aliases `cidx pr ...` / `cidx cpw`). Until the removal, the
// warning itself was exempt, because it named the deprecated form in order to
// steer the user off it; with the tree gone there is nothing left to exempt.
func TestNoDeprecatedActionHintsInGoSources(t *testing.T) {
	err := filepath.WalkDir(projectRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "vendor" {
				return fs.SkipDir
			}
			return nil
		}
		// Test files are exempt: this one, and the forbidden-string lists in
		// pkg/actions, legitimately mention the deprecated form.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, "cidx action") {
				t.Errorf("%s:%d points at the deprecated 'cidx action' command: %s",
					path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking sources: %v", err)
	}
}
