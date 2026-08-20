package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryDocIsReachable fails on a page no index links to.
//
// A document nobody can navigate to is one nobody reads, and it fails silently:
// nothing errors, the file is right there, and the reader who needed it never
// learns it exists. Three had accumulated — the two product guardrail documents
// among them, which CLAUDE.md calls the first thing to check before any feature
// and which the documentation index did not mention at all.
//
// Reachability is deliberately shallow: linked from docs/README.md, from the
// repository README, or from another page those reach. A page linked only from
// a page that is itself an orphan is still lost.
func TestEveryDocIsReachable(t *testing.T) {
	docs := filepath.Join(projectRoot, "docs")

	// Everything that links: the two indexes, plus every page under docs/.
	var corpus strings.Builder
	for _, entry := range []string{
		filepath.Join(projectRoot, "README.md"),
		filepath.Join(docs, "README.md"),
	} {
		body, err := os.ReadFile(entry)
		if err != nil {
			t.Fatalf("failed to read %s: %v", entry, err)
		}
		corpus.Write(body)
	}

	var pages []string
	err := filepath.Walk(docs, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		pages = append(pages, path)
		body, readErr := os.ReadFile(path)
		if readErr == nil {
			corpus.Write(body)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk %s: %v", docs, err)
	}
	if len(pages) == 0 {
		t.Fatal("no documentation found — this guard would pass by reading nothing")
	}

	text := corpus.String()
	for _, page := range pages {
		name := filepath.Base(page)
		if name == "README.md" {
			continue // the indexes themselves
		}
		// security.md is a redirect, not a destination: sarif.go wrote its URL
		// into the helpUri of every alert the audit has ever uploaded, and the
		// ones already in the Security tab still carry it. It exists to answer
		// those, so linking it from an index would advertise a page whose only
		// content is four pointers elsewhere (#425).
		if strings.HasSuffix(page, filepath.Join("core-concepts", "security.md")) {
			continue
		}
		if !strings.Contains(text, name) {
			t.Errorf("docs/%s is linked from nothing — add it to docs/README.md.\n"+
				"A page no index reaches is a page nobody reads, and it says so nowhere.",
				mustRel(t, docs, page))
		}
	}
}

func mustRel(t *testing.T, base, path string) string {
	t.Helper()

	rel, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return rel
}
