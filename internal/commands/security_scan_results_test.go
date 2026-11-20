package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// A checkout of its own gets the results the shared store holds for the digests,
// and nothing an earlier run left behind.
func TestCopyScanResultsGivesTheCheckoutItsOwnCopy(t *testing.T) {
	store, checkout := t.TempDir(), t.TempDir()
	image := "example/tool:1.0@sha256:ab0000"

	for _, name := range scanResultFiles("trivy", image) {
		if err := os.WriteFile(filepath.Join(store, name), []byte(`{"a":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	staleFile := filepath.Join(checkout, scanResultFiles("grype", image)[0])
	if err := os.WriteFile(staleFile, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyScanResults(store, checkout, []string{image}); err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	for _, name := range scanResultFiles("trivy", image) {
		got, err := os.ReadFile(filepath.Join(checkout, name))
		if err != nil || string(got) != `{"a":1}` {
			t.Errorf("%s not copied into the checkout: %q, %v", name, got, err)
		}
	}
	if _, err := os.Stat(staleFile); !os.IsNotExist(err) {
		t.Errorf("a result the store does not hold was kept in the checkout: %v", err)
	}
}

func TestCopyScanResultsToTheSameDirectoryIsNothing(t *testing.T) {
	dir := t.TempDir()
	if err := copyScanResults(dir, dir, []string{"x@sha256:00"}); err != nil {
		t.Fatalf("same directory: %v", err)
	}
}
