package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The `cidx action ...` tree is hidden and deprecated (see
// internal/commands/app.go), so nothing the user can read may still point at
// it. Guards issue #174 against regressions: new hints must name the current
// paths (`cidx release ...`, `cidx repo ...`, or the top-level aliases
// `cidx pr ...` / `cidx cpw`).
func TestNoDeprecatedActionHintsInGoSources(t *testing.T) {
	// The deprecation warning is the one source that must name the deprecated
	// form: it names it to steer the user off it, and its own tests check that
	// every mention is followed by the replacement (issue #235).
	exempt := filepath.Join("internal", "commands", "action_deprecated.go")

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
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
		if filepath.Clean(path) == exempt {
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
