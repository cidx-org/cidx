package remote

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GitHubWorkflowDir is the directory where GitHub Actions workflow files live.
const GitHubWorkflowDir = ".github/workflows"

// CandidateWorkflowFiles lists the workflow filenames cidx recognizes, in
// preference order. `cidx generate github` writes cidx.yml; ci.yml is the
// conventional name for hand-written workflows (cidx's own repo uses it).
// Every code path that needs "the cidx workflow" must derive its candidates
// from this list instead of hardcoding a filename (issue #170).
var CandidateWorkflowFiles = []string{"cidx.yml", "ci.yml"}

// ResolveWorkflowFile returns the path of the first candidate workflow file
// that exists in dir. When none is found, the error names every candidate
// tried so the user knows what was searched.
func ResolveWorkflowFile(dir string) (string, error) {
	for _, name := range CandidateWorkflowFiles {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no cidx workflow found in %s (tried %s)", dir, strings.Join(CandidateWorkflowFiles, ", "))
}
