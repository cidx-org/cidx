package generate

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestGeneratedActionsTrackThisRepositorysWorkflows keeps ActionVersions level
// with this repository's own workflows (#424).
//
// Those are pinned by SHA with the version in a trailing comment, and
// Dependabot moves them — the generator's table it cannot move, which is how
// checkout and setup-go sat a major behind for weeks. When Dependabot bumps an
// action here to a new major, this fails until the generator follows, so the
// drift surfaces at the next dependency PR rather than in a project's
// workflow.
func TestGeneratedActionsTrackThisRepositorysWorkflows(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", ".github", "workflows", "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows found to compare against: %v", err)
	}

	pinned := regexp.MustCompile(`uses:\s*([\w.-]+/[\w.-]+)@[0-9a-f]{40}\s*#\s*(v\d+)`)
	ours := map[string]string{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pinned.FindAllStringSubmatch(string(data), -1) {
			ours[m[1]] = m[2]
		}
	}

	compared := 0
	for action, generated := range ActionVersions {
		major, used := ours[action]
		if !used {
			continue // nothing here to take a version from
		}
		compared++
		if generated != major {
			t.Errorf("the generator emits %s@%s, this repository's workflows run %s: update ActionVersions (pkg/generate/github.go)",
				action, generated, strings.Join([]string{action, major}, "@"))
		}
	}
	if compared == 0 {
		t.Fatal("no generated action is used by this repository's workflows: the guard compares nothing")
	}
}
