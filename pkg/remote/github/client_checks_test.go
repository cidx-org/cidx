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
