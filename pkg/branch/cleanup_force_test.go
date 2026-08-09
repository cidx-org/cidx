package branch

import (
	"os"
	"strings"
	"testing"
)

// TestCleanupConsultsMayForceDelete fails when the decision is written but not
// called (#417).
//
// This is the fourth guard in this sequence whose logic was covered and whose
// *wiring* was not, and every time the sabotage that went unnoticed was the same
// one: delete the call, keep the function. Nothing goes red. The scenarios still
// pass, because they drive the decision directly; the command simply stops
// asking, and force-deletes every branch GitHub calls merged again.
//
// Driving Cleanup itself needs a manager, a GitHub client and a repository whose
// branches have pull requests, so the call is asserted where it is visible.
func TestCleanupConsultsMayForceDelete(t *testing.T) {
	source, err := os.ReadFile("cleanup.go")
	if err != nil {
		t.Fatalf("failed to read cleanup.go: %v", err)
	}
	text := string(source)

	if !strings.Contains(text, "forceDelete := MayForceDelete(") {
		t.Error("Cleanup no longer asks MayForceDelete whether forcing is safe. Without it, every branch " +
			"GitHub reports merged is deleted with `git branch -D`, including one carrying commits that " +
			"never reached the remote (#417).")
	}

	// The expression it replaced, so a revert is named rather than merely absent.
	if strings.Contains(text, "opts.Force || branch.Status == StatusMerged") {
		t.Error("Cleanup forces on GitHub's merged verdict alone again. That verdict describes the pull " +
			"request, and says nothing about commits added to the local branch afterwards.")
	}
}

// TestMayForceDelete_TheResidualLimitIsDeliberate documents the one case that
// still forces on the merged verdict alone: the remote branch is gone, so there
// is no hash to compare against.
//
// Written as a test rather than a comment because it is the case someone will
// later read as an oversight and "fix" -- and refusing there would make cleanup
// keep every branch it exists to remove, which is how a guard gets deleted
// instead of tightened.
func TestMayForceDelete_TheResidualLimitIsDeliberate(t *testing.T) {
	if !MayForceDelete(false, StatusMerged, "6414eb1", "") {
		t.Error("a merged branch whose remote is already gone must still be deleted: once the remote ref " +
			"goes, a squash-merged branch and one holding unique work look identical from here, and the " +
			"merged verdict is the only evidence there is")
	}

	if MayForceDelete(false, StatusMerged, "6414eb1", "a88cb3e") {
		t.Error("the remote is known and holds a different commit, which is the one case where the merged " +
			"verdict is demonstrably not the whole story")
	}
}
