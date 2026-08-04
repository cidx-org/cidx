package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createPullRequestAction is the action whose default is the problem: with no
// `add-paths`, it commits every file it finds modified in the runner's
// workspace.
const createPullRequestAction = "uses: peter-evans/create-pull-request@"

// TestPullRequestActionsNameWhatTheyCommit is the standing guard behind issue
// #325.
//
// `peter-evans/create-pull-request` commits whatever is dirty unless it is told
// what to commit. The container monitor downloads every scan artifact into its
// own workspace before deciding what to promote, so its workspace is dirty by
// construction — and the promotion PR carried 43 files and 201,615 lines of
// scanner JSON next to its one-line image bump (#323). Because gitleaks scans
// every ref, the Security job of every branch in the repository then failed on
// CVE text that pattern-matches an API token, and unblocking CI meant closing
// the PR and deleting the branch.
//
// `.gitignore` (#324) closes that particular hole. It does not close the next
// one: the day someone downloads artifacts into a directory that is not
// ignored, the default is back, silently. An allowlist has the opposite
// failure mode — a promotion that legitimately starts touching a new file is
// caught immediately, by a diff that is missing something rather than by a
// repository-wide CI outage.
//
// So the invariant is pinned here: any step that opens a PR from a runner
// workspace states which paths it commits.
func TestPullRequestActionsNameWhatTheyCommit(t *testing.T) {
	workflows, err := filepath.Glob(filepath.Join(".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatalf("failed to list the workflows: %v", err)
	}

	steps := 0
	for _, workflow := range workflows {
		content, err := os.ReadFile(workflow)
		if err != nil {
			t.Fatalf("failed to read %s: %v", workflow, err)
		}

		for _, step := range stepsUsing(string(content), createPullRequestAction) {
			steps++

			paths := addPaths(step)
			if len(paths) == 0 {
				t.Errorf("%s: this step opens a pull request without saying what to commit.\n"+
					"The action's default is to commit every modified file in the workspace,\n"+
					"which is how 201,615 lines of scanner JSON reached a one-line image bump\n"+
					"and broke the Security job of every branch in the repository (#323, #325).\n"+
					"List the files the change legitimately touches:\n\n"+
					"  add-paths: |\n    pkg/presets/presets.toml\n\n%s", workflow, step)
			}
		}
	}

	if steps == 0 {
		t.Fatalf("no %q step found in .github/workflows — the guard would pass vacuously", createPullRequestAction)
	}
}

// stepsUsing returns the text of every workflow step invoking action, from its
// `uses:` line to the end of the step. A step ends at the next line that is
// either outdented past the step's keys or starts a sibling list item.
func stepsUsing(workflow, action string) []string {
	var steps []string

	lines := strings.Split(workflow, "\n")
	for i, line := range lines {
		if !strings.Contains(line, action) {
			continue
		}

		keyIndent := indentOf(line)
		step := []string{line}

		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				step = append(step, next)
				continue
			}
			if indentOf(next) < keyIndent || strings.HasPrefix(strings.TrimSpace(next), "- ") {
				break
			}
			step = append(step, next)
		}

		steps = append(steps, strings.Join(step, "\n"))
	}

	return steps
}

// addPaths returns the paths a step's `add-paths:` block lists. Both YAML
// spellings count: the block scalar this repository uses, and a one-line value.
func addPaths(step string) []string {
	lines := strings.Split(step, "\n")

	for i, line := range lines {
		key, value, found := strings.Cut(strings.TrimSpace(line), "add-paths:")
		if !found || key != "" {
			continue
		}

		if inline := strings.TrimSpace(value); inline != "" && inline != "|" && inline != ">" {
			return []string{inline}
		}

		// A block scalar: every line indented deeper than the key belongs to it.
		var paths []string
		for _, next := range lines[i+1:] {
			if strings.TrimSpace(next) == "" {
				continue
			}
			if indentOf(next) <= indentOf(line) {
				break
			}
			paths = append(paths, strings.TrimSpace(next))
		}
		return paths
	}

	return nil
}

// indentOf counts the leading spaces of a line.
func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
