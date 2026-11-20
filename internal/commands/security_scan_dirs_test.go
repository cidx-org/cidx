package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMakeScanDirsCreatesTheStoreAndTheScratchDirectories(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache")
	results := filepath.Join(t.TempDir(), "results")

	if err := makeScanDirs(cache, results); err != nil {
		t.Fatalf("makeScanDirs: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(cache, "results"),
		filepath.Join(cache, "trivy", "tmp"),
		filepath.Join(cache, "grype", "tmp"),
		results,
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Errorf("%s was not created (err = %v)", dir, err)
		}
	}
}

func TestMakeScanDirsNamesTheDirectoryItCouldNotCreate(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes anywhere")
	}
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.Mkdir(cache, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(cache, 0o700) })

	err := makeScanDirs(cache, "")
	if err == nil {
		t.Fatal("a cache that cannot be written to was accepted")
	}
	if want := filepath.Join(cache, "results"); !containsString(err.Error(), want) {
		t.Errorf("the error does not name %s: %v", want, err)
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
