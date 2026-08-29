package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
		t.Fatalf("%s is out of date (-committed +generated):\n%s\nRegenerate it with `cidx preset catalogue` and commit the result.",
			defaultCatalogueReferenceFile, cmp.Diff(string(committed), rendered))
	}
}
