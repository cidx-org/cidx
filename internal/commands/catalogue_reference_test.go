package commands

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogueReferenceIsCurrent regenerates the committed page offline and
// fails on any difference — the same guard TestSecurityBaselineIsCurrent
// holds for the baseline (#310), for the same reason: a page nothing
// regenerates rots, and this one exists to replace one that had.
func TestCatalogueReferenceIsCurrent(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", defaultCatalogueReferenceFile))
	if err != nil {
		t.Fatalf("could not read the committed page: %v", err)
	}
	rendered, err := RenderCatalogueReference()
	if err != nil {
		t.Fatalf("could not render the page: %v", err)
	}
	if string(committed) != rendered {
		t.Fatalf("%s is not an output of the current catalogue.\n\nRegenerate it — `cidx preset catalogue` — and commit the result.", defaultCatalogueReferenceFile)
	}
}
