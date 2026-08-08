package commands

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/branch"
)

// stageLocalHead makes the guard read sha instead of the working tree.
func stageLocalHead(t *testing.T, sha string) {
	t.Helper()

	original := localHeadSHA
	localHeadSHA = func() string { return sha }
	t.Cleanup(func() { localHeadSHA = original })
}

// The decision itself is specified in features/cli/watch_target.feature. What
// these cover is the wiring: that the commit compared against local HEAD is the
// pull request's, and that a refusal reaches the caller as an error rather than
// a log line nobody's exit code sees.

func TestCheckWatchTarget_RefusesWhenThePushNeverHappened(t *testing.T) {
	stageLocalHead(t, "c11466b1a2b3c4d5e6f7089a0b1c2d3e4f506172")

	err := checkWatchTarget(&branch.PRInfo{HeadSHA: "dd86dc41a2b3c4d5e6f7089a0b1c2d3e4f506172"})
	if err == nil {
		t.Fatal("a watch on a commit the remote has never seen returned nil, so the caller exits 0 " +
			"and the report reads as an answer about the local commit")
	}
	if !strings.Contains(err.Error(), "c11466b") || !strings.Contains(err.Error(), "dd86dc4") {
		t.Errorf("the refusal has to name both commits to be actionable, got: %v", err)
	}
}

func TestCheckWatchTarget_ProceedsOnTheCommitInHand(t *testing.T) {
	const sha = "c54d85c1a2b3c4d5e6f7089a0b1c2d3e4f506172"
	stageLocalHead(t, sha)

	if err := checkWatchTarget(&branch.PRInfo{HeadSHA: sha}); err != nil {
		t.Fatalf("the pushed commit is the one in hand, so the watch must run: %v", err)
	}
}

// A pull request carrying no head SHA is the shape this guard degrades into if
// PRInfo ever stops being populated — it must not become a silent pass, and it
// must not become a refusal either, or every watch breaks at once.
func TestCheckWatchTarget_AnUnknownCommitNeitherPassesNorRefuses(t *testing.T) {
	stageLocalHead(t, "c54d85c1a2b3c4d5e6f7089a0b1c2d3e4f506172")

	if err := checkWatchTarget(&branch.PRInfo{HeadSHA: ""}); err != nil {
		t.Fatalf("an unverifiable commit must not stop a watch: %v", err)
	}

	proceed, message := JudgeWatchTarget("c54d85c", "")
	if !proceed || !strings.Contains(message, "Could not verify") {
		t.Errorf("it has to say the check could not be made, got proceed=%v %q", proceed, message)
	}
}
