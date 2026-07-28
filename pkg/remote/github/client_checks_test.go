package github

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// fastPolling shortens the wait loop so the tests exercise several rounds
// without waiting on the real clock.
func fastPolling(t *testing.T) {
	t.Helper()
	previous := checksPollInterval
	checksPollInterval = 2 * time.Millisecond
	t.Cleanup(func() { checksPollInterval = previous })
}

func TestIsWorkflowCheck(t *testing.T) {
	tests := []struct {
		name string
		run  *github.CheckRun
		want bool
	}{
		{
			name: "github actions check run",
			run:  &github.CheckRun{Name: github.Ptr("Security"), App: &github.App{Slug: github.Ptr("github-actions")}},
			want: true,
		},
		{
			// The check that made cpw declare PR #255 green (issue #257).
			name: "dependabot config check",
			run:  &github.CheckRun{Name: github.Ptr(".github/dependabot.yml"), App: &github.App{Slug: github.Ptr("dependabot")}},
			want: false,
		},
		{
			name: "check without app",
			run:  &github.CheckRun{Name: github.Ptr("mystery")},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWorkflowCheck(tt.run); got != tt.want {
				t.Errorf("isWorkflowCheck() = %v, want %v", got, tt.want)
			}
		})
	}
}

// run builds a workflow run as the repository-wide listing returns it: an
// event and the check suite its check runs carry.
func run(event string, checkSuiteID int64) *github.WorkflowRun {
	return &github.WorkflowRun{Event: github.Ptr(event), CheckSuiteID: github.Ptr(checkSuiteID)}
}

// checkRun builds a completed check run of the given suite.
func checkRun(name string, suiteID int64, conclusion string) *github.CheckRun {
	return &github.CheckRun{
		Name:       github.Ptr(name),
		Status:     github.Ptr("completed"),
		Conclusion: github.Ptr(conclusion),
		CheckSuite: &github.CheckSuite{ID: github.Ptr(suiteID)},
		App:        &github.App{Slug: github.Ptr(workflowAppSlug)},
	}
}

// listing returns a listRepoRunsFunc serving the given runs, recording the
// options it was asked for.
func listing(runs []*github.WorkflowRun, err error, seen *github.ListWorkflowRunsOptions) listRepoRunsFunc {
	return func(_ context.Context, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
		if seen != nil {
			*seen = *opts
		}
		if err != nil {
			return nil, nil, err
		}
		return &github.WorkflowRuns{WorkflowRuns: runs}, nil, nil
	}
}

// TestDispatchedCheckSuites_SinglesOutRunsThePRDidNotCause covers issue #240
// with the shape of PR #262: one pull_request run of 5 checks alongside a
// hand-dispatched Container Monitor of 43.
func TestDispatchedCheckSuites_SinglesOutRunsThePRDidNotCause(t *testing.T) {
	var asked github.ListWorkflowRunsOptions
	dispatched := dispatchedCheckSuites(context.Background(), "abc1234def", listing([]*github.WorkflowRun{
		run("workflow_dispatch", 82337504968),
		run("pull_request", 82337037898),
	}, nil, &asked))

	if !dispatched[82337504968] {
		t.Error("expected the dispatched run's check suite to be singled out")
	}
	if dispatched[82337037898] {
		t.Error("the pull_request run's check suite is the PR's own gate, it must be kept")
	}
	if asked.HeadSHA != "abc1234def" {
		t.Errorf("expected the listing to be filtered on the head SHA, got %q", asked.HeadSHA)
	}
}

// TestDispatchedCheckSuites_KeepsPushAndScheduledAlike: `push` stays because a
// repository whose CI triggers on push alone still gates its PRs with those
// checks; `schedule` goes because the clock is not the pull request.
func TestDispatchedCheckSuites_KeepsPushAndScheduledAlike(t *testing.T) {
	dispatched := dispatchedCheckSuites(context.Background(), "abc1234def", listing([]*github.WorkflowRun{
		run("push", 1),
		run("schedule", 2),
		run("repository_dispatch", 3),
		run("pull_request_target", 4),
	}, nil, nil))

	for _, kept := range []int64{1, 4} {
		if dispatched[kept] {
			t.Errorf("check suite %d gates the PR, it must not be filtered out", kept)
		}
	}
	for _, dropped := range []int64{2, 3} {
		if !dispatched[dropped] {
			t.Errorf("check suite %d was not caused by the PR, it must be filtered out", dropped)
		}
	}
}

// TestDispatchedCheckSuites_FailureCountsEverything: the run listing is an
// extra request that must never break the read of the checks. When it fails,
// nothing is filtered and the pre-#240 behaviour stands.
func TestDispatchedCheckSuites_FailureCountsEverything(t *testing.T) {
	dispatched := dispatchedCheckSuites(context.Background(), "abc1234def", listing(nil, errors.New("502 bad gateway"), nil))
	if len(dispatched) != 0 {
		t.Errorf("expected no suite to be filtered when the listing fails, got %v", dispatched)
	}
}

// TestAddCheckRuns_LeavesOutDispatchedSuites is the end of the #240 chain: the
// 43 red jobs of a dispatched workflow must not reach the PR's counters, which
// is what made cpw exit red and pr merge refuse.
func TestAddCheckRuns_LeavesOutDispatchedSuites(t *testing.T) {
	const prSuite, dispatchedSuite = int64(82337037898), int64(82337504968)

	checks := &remote.PRChecks{}
	addCheckRuns(checks, []*github.CheckRun{
		checkRun("Bootstrap", prSuite, "success"),
		checkRun("Security", prSuite, "success"),
		checkRun("Scan alpine", dispatchedSuite, "failure"),
		checkRun("Scan golang", dispatchedSuite, "failure"),
	}, map[int64]bool{dispatchedSuite: true})

	if checks.TotalCount != 2 || checks.WorkflowChecks != 2 {
		t.Errorf("expected only the PR's own 2 checks, got %+v", checks)
	}
	if checks.Failure != 0 || checks.Success != 2 {
		t.Errorf("expected the dispatched failures to be left out, got %d failure(s)", checks.Failure)
	}
	for _, check := range checks.Checks {
		if strings.HasPrefix(check.Name, "Scan ") {
			t.Errorf("dispatched check %q was reported as a check of the PR", check.Name)
		}
	}
}

// TestAddCheckRuns_WithoutDispatchedSuitesCountsEverything: on a PR whose SHA
// carries nothing but its own checks -- and whenever the run listing came back
// empty -- every check still counts.
func TestAddCheckRuns_WithoutDispatchedSuitesCountsEverything(t *testing.T) {
	checks := &remote.PRChecks{}
	addCheckRuns(checks, []*github.CheckRun{
		checkRun("Bootstrap", 1, "success"),
		checkRun("Test", 1, "failure"),
	}, nil)

	if checks.TotalCount != 2 || checks.Success != 1 || checks.Failure != 1 {
		t.Errorf("expected both checks to be counted, got %+v", checks)
	}
}

func TestWaitForWorkflowCheck_ForeignCheckDoesNotEndTheWait(t *testing.T) {
	// A repository whose only check comes from another app: the wait must not
	// stop on it, and after the timeout it hands back what exists with
	// WorkflowChecks == 0 so the caller can report it honestly (issue #257).
	fastPolling(t)

	calls := 0
	checks, err := waitForWorkflowCheck(context.Background(), "abc1234def", 40*time.Millisecond, func(context.Context) (*remote.PRChecks, error) {
		calls++
		return &remote.PRChecks{TotalCount: 1, WorkflowChecks: 0, Success: 1, Status: "success", HeadSHA: "abc1234def"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.WorkflowChecks != 0 || checks.TotalCount != 1 {
		t.Errorf("expected the foreign check to be returned as-is, got %+v", checks)
	}
	if calls < 3 {
		t.Errorf("expected the wait to keep polling, got %d calls", calls)
	}
}

func TestWaitForWorkflowCheck_ReturnsAsSoonAsWorkflowChecksAppear(t *testing.T) {
	fastPolling(t)

	calls := 0
	checks, err := waitForWorkflowCheck(context.Background(), "abc1234def", 5*time.Second, func(context.Context) (*remote.PRChecks, error) {
		calls++
		if calls < 3 {
			return &remote.PRChecks{TotalCount: 1, Success: 1, Status: "success", HeadSHA: "abc1234def"}, nil
		}
		return &remote.PRChecks{TotalCount: 6, WorkflowChecks: 5, Pending: 5, Success: 1, Status: "pending", HeadSHA: "abc1234def"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.WorkflowChecks != 5 {
		t.Errorf("expected the workflow checks to be returned, got %+v", checks)
	}
	if calls != 3 {
		t.Errorf("expected the wait to return on the first workflow check, got %d calls", calls)
	}
}

func TestWaitForWorkflowCheck_NoChecksAtAllReportsNoCI(t *testing.T) {
	// Repository without CI: unchanged contract -- an error plus an empty
	// checks value, which cpw and pr merge turn into "no CI configured".
	fastPolling(t)

	checks, err := waitForWorkflowCheck(context.Background(), "abc1234def", 20*time.Millisecond, func(context.Context) (*remote.PRChecks, error) {
		return &remote.PRChecks{HeadSHA: "abc1234def"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "no CI checks found") {
		t.Fatalf("expected a 'no CI checks found' error, got: %v", err)
	}
	if checks == nil || checks.TotalCount != 0 {
		t.Fatalf("expected empty checks to be returned alongside the error, got %+v", checks)
	}
}

func TestWaitForWorkflowCheck_IgnoresChecksOfAnotherCommit(t *testing.T) {
	// Behaviour from issue #167: checks belonging to a previous commit never
	// satisfy the wait, however complete they look.
	fastPolling(t)

	checks, err := waitForWorkflowCheck(context.Background(), "abc1234def", 20*time.Millisecond, func(context.Context) (*remote.PRChecks, error) {
		return &remote.PRChecks{TotalCount: 5, WorkflowChecks: 5, Success: 5, Status: "success", HeadSHA: "0000000old"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.HeadSHA != "0000000old" {
		t.Fatalf("expected the timeout to hand back the last read, got %+v", checks)
	}
}

func TestWaitForWorkflowCheck_TransientErrorsAreRetried(t *testing.T) {
	fastPolling(t)

	calls := 0
	checks, err := waitForWorkflowCheck(context.Background(), "abc1234def", 5*time.Second, func(context.Context) (*remote.PRChecks, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("502 bad gateway")
		}
		return &remote.PRChecks{TotalCount: 1, WorkflowChecks: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def"}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if checks.WorkflowChecks != 1 {
		t.Errorf("expected the retry to succeed, got %+v", checks)
	}
}
