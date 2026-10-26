package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureGitignored(t *testing.T) {
	read := func(d string) string { b, _ := os.ReadFile(filepath.Join(d, ".gitignore")); return string(b) }

	t.Run("not a git work tree: nothing written", func(t *testing.T) {
		d := t.TempDir()
		if added, err := ensureGitignored(d, ".cidx/"); added || err != nil || read(d) != "" {
			t.Fatalf("added=%v err=%v content=%q", added, err, read(d))
		}
	})

	t.Run("appends once, keeping what is there", func(t *testing.T) {
		d := t.TempDir()
		_ = os.Mkdir(filepath.Join(d, ".git"), 0o755)
		_ = os.WriteFile(filepath.Join(d, ".gitignore"), []byte("bin/"), 0o644)
		for i := 0; i < 2; i++ {
			if _, err := ensureGitignored(d, ".cidx/"); err != nil {
				t.Fatal(err)
			}
		}
		if got := read(d); got != "bin/\n.cidx/\n" {
			t.Fatalf("content = %q", got)
		}
	})

	t.Run("an equivalent line counts", func(t *testing.T) {
		d := t.TempDir()
		_ = os.Mkdir(filepath.Join(d, ".git"), 0o755)
		_ = os.WriteFile(filepath.Join(d, ".gitignore"), []byte("/.cidx\n"), 0o644)
		if added, _ := ensureGitignored(d, ".cidx/"); added {
			t.Fatal("wrote a duplicate")
		}
	})
}
