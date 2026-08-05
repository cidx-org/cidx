package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CIDX tells a checkout that failed on a worktree conflict apart from one that
// really failed, and a push that had nothing to delete apart from one that was
// refused, by matching git's own sentences -- git offers no exit code that
// separates them. Those sentences are translated, so on a French machine the
// matches miss and CIDX reports failures for the cases it was written to
// forgive (issue #364).
//
// vcs.Git pins LC_ALL=C, which makes git's output an interface CIDX controls.
// That only holds while it is the one place a git command is built: a call site
// spelling out exec.Command("git", ...) is born reading whatever git was
// configured to say, and the next parse written against it would be wrong on
// the same machines -- silently, since CI runs in English.
func TestEveryGitInvocationPinsTheLocale(t *testing.T) {
	// The helper is where the pinning lives, so it is the one file that builds
	// a git command by hand.
	helper := filepath.Join("pkg", "vcs", "git.go")

	scanned := 0
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
		// Tests are exempt, and one of them depends on being exempt:
		// TestWorktreeHoldingMatchesRealGit builds its fixture with the
		// developer's own locale on purpose, because a fixture that pinned the
		// locale too would prove nothing about the machines this breaks on.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if filepath.Clean(path) == helper {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		scanned++
		for i, line := range strings.Split(string(src), "\n") {
			if strings.Contains(line, `exec.Command("git"`) {
				t.Errorf("%s:%d runs git without pinning the locale: %s\n"+
					"Use vcs.Git(dir, args...) instead. A raw git command reads whatever\n"+
					"language the user's git speaks, and CIDX matches on the English (#364).",
					path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking sources: %v", err)
	}
	if scanned == 0 {
		t.Fatal("no source was read — the guard would pass vacuously")
	}
}
