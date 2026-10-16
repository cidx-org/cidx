package executor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLogKeepsTheOutputOfARun(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteLog(dir, "cargo-audit", "Loaded 900 advisories\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".cidx", "logs", "cargo-audit.log"); path != want {
		t.Fatalf("path = %s, want %s", path, want)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "Loaded 900 advisories\n" {
		t.Fatalf("content = %q", got)
	}
	// the next run replaces it: the file is the last run, not a history
	if _, err := WriteLog(dir, "cargo-audit", "second"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("content = %q", got)
	}
}

func TestWriteLogNameCannotEscapeTheDirectory(t *testing.T) {
	dir := t.TempDir()
	path, err := WriteLog(dir, "../evil", "x")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != filepath.Join(dir, ".cidx", "logs") {
		t.Fatalf("escaped: %s", path)
	}
}
