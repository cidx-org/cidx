package github

import (
	"context"
	"errors"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// The shape of issue #367, as observed on PR #366: ci.yml declares
//
//	security: needs: [bootstrap]
//	code:     needs: [bootstrap]
//	test:     needs: [bootstrap]
//	build:    needs: [security, code, test]
//
// so between the moment Bootstrap goes green and the moment the three jobs it
// gates become eligible, the commit carries exactly one check run, successful,
// and nothing pending. `cpw` announced a green CI there, and `pr merge` would
// have merged on it.

// TestRunsOnHead_CountsARunThePRCausedAndIsStillGoing is the missing signal:
// the run has not finished, so more checks are coming, whatever the checks that
// exist say.
func TestRunsOnHead_CountsARunThePRCausedAndIsStillGoing(t *testing.T) {
	_, inProgress := runsOnHead(context.Background(), "abc1234def", listing([]*github.WorkflowRun{
		runWithStatus("pull_request", 1, "in_progress"),
	}, nil, nil))

	if inProgress != 1 {
		t.Errorf("in progress = %d, want 1 — Bootstrap being green does not make the run over", inProgress)
	}
}

// TestRunsOnHead_IgnoresRunsThePRDidNotCause: a hand-dispatched workflow lands
// on the same SHA (#240). Its checks are already excluded from the PR's
// counters, so waiting for it to finish would hold the gate on something that
// was never gating.
func TestRunsOnHead_IgnoresRunsThePRDidNotCause(t *testing.T) {
	_, inProgress := runsOnHead(context.Background(), "abc1234def", listing([]*github.WorkflowRun{
		runWithStatus("workflow_dispatch", 1, "in_progress"),
		runWithStatus("schedule", 2, "queued"),
		runWithStatus("pull_request", 3, "completed"),
	}, nil, nil))

	if inProgress != 0 {
		t.Errorf("in progress = %d, want 0 — only the runs the PR caused can add checks to its gate", inProgress)
	}
}

// TestRunsOnHead_CountsQueuedRunsToo: a run that has not started is the most
// extreme version of "its checks do not exist yet".
func TestRunsOnHead_CountsQueuedRunsToo(t *testing.T) {
	_, inProgress := runsOnHead(context.Background(), "abc1234def", listing([]*github.WorkflowRun{
		runWithStatus("pull_request", 1, "queued"),
		runWithStatus("push", 2, "in_progress"),
		runWithStatus("pull_request", 3, "completed"),
	}, nil, nil))

	if inProgress != 2 {
		t.Errorf("in progress = %d, want 2 (the queued one and the push one)", inProgress)
	}
}

// TestRunsOnHead_FailureCountsNothing keeps the #240 promise: the listing is an
// extra request that must never break the read of the checks. A failure there
// leaves the count at zero, which is the pre-#367 behaviour — degraded, not
// broken, and never a false red.
func TestRunsOnHead_FailureCountsNothing(t *testing.T) {
	_, inProgress := runsOnHead(context.Background(), "abc1234def", listing(nil, errors.New("502 bad gateway"), nil))

	if inProgress != 0 {
		t.Errorf("in progress = %d, want 0 when the listing fails", inProgress)
	}
}

// TestComplete_IsNotSatisfiedByAnEmptyGap is the assertion the whole issue
// reduces to, stated on the type every watcher and the merge gate consult.
func TestComplete_IsNotSatisfiedByAnEmptyGap(t *testing.T) {
	tests := []struct {
		name  string
		state remote.PRChecks
		want  bool
	}{
		{
			name:  "the gap between two stages: Bootstrap green, the rest not created",
			state: remote.PRChecks{TotalCount: 1, Success: 1, Pending: 0, RunsInProgress: 1},
			want:  false,
		},
		{
			name:  "genuinely finished",
			state: remote.PRChecks{TotalCount: 5, Success: 5, Pending: 0, RunsInProgress: 0},
			want:  true,
		},
		{
			name:  "checks still running",
			state: remote.PRChecks{TotalCount: 5, Success: 1, Pending: 4, RunsInProgress: 1},
			want:  false,
		},
		{
			name:  "no Actions at all, only another CI's commit statuses",
			state: remote.PRChecks{TotalCount: 2, Success: 2, Pending: 0, RunsInProgress: 0},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Complete(); got != tt.want {
				t.Errorf("Complete() = %v, want %v", got, tt.want)
			}
		})
	}
}
