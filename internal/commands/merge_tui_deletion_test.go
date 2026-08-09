package commands

import (
	"os"
	"strings"
	"testing"
)

// TestMergeTUINeverForcesABranchDeletion fails if `git branch -D` returns to the
// merge TUI (#417).
//
// This call site had no fallback to remove -- it forced outright, without ever
// asking git. There is no behavioural test beside it because driving the TUI
// needs a terminal, a provider and a merged pull request, which is exactly the
// combination that let this line sit unexamined: `git branch -d` refuses a
// branch only when it holds commits its upstream does not have, and `-D` here
// deleted the one copy of them without a word.
func TestMergeTUINeverForcesABranchDeletion(t *testing.T) {
	source, err := os.ReadFile("merge_tui.go")
	if err != nil {
		t.Fatalf("failed to read merge_tui.go: %v", err)
	}

	if strings.Contains(string(source), `"branch", "-D"`) {
		t.Error(`merge_tui.go runs "git branch -D". Use "-d": git refuses a branch that still holds ` +
			`commits its upstream has never seen, and that refusal is the only thing standing between ` +
			`post-merge housekeeping and someone's unpushed work.`)
	}
}
