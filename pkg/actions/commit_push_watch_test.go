package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
)

// cpwFakeProvider extends fakeProvider with configurable PR lookup, checks
// wait, and checks watch behavior, to exercise the cpw CI watch flow.
type cpwFakeProvider struct {
	fakeProvider

	prNumber int
	prErr    error

	waitSHA     string
	waitChecks  *remote.PRChecks
	waitErr     error
	waitTimeout time.Duration // records the timeout cpw asked for

	checksUpdates []remote.PRChecksUpdate
	checksErr     error
	watchCalled   bool
}

func (f *cpwFakeProvider) GetPullRequestByBranch(_ context.Context, _ string) (int, string, error) {
	if f.prErr != nil {
		return 0, "", f.prErr
	}
	return f.prNumber, "https://example.test/pr", nil
}

func (f *cpwFakeProvider) WaitForChecksToStart(_ context.Context, _ int, timeout time.Duration) (string, *remote.PRChecks, error) {
	f.waitTimeout = timeout
	return f.waitSHA, f.waitChecks, f.waitErr
}

func (f *cpwFakeProvider) WatchPullRequestChecks(_ context.Context, _ int) (<-chan remote.PRChecksUpdate, error) {
	f.watchCalled = true
	if f.checksErr != nil {
		return nil, f.checksErr
	}
	ch := make(chan remote.PRChecksUpdate, len(f.checksUpdates))
	for _, u := range f.checksUpdates {
		ch <- u
	}
	close(ch)
	return ch, nil
}

func newCPWAction(provider remote.Provider) *CommitPushWatchAction {
	return &CommitPushWatchAction{
		provider:       provider,
		ciStartTimeout: defaultCIStartTimeout,
	}
}

func TestCPWWatchCI_NoPRIsNotAnError(t *testing.T) {
	// Without a PR there is nothing to watch: cpw explains and exits cleanly.
	// The fake panics on WaitForChecksToStart only via explicit stubs, so a
	// zero-value waitChecks would be returned if cpw wrongly proceeded; instead
	// we assert the watch was never started.
	provider := &cpwFakeProvider{prErr: errors.New("no PR found for branch")}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no checks watch without a PR")
	}
	if provider.waitTimeout != 0 {
		t.Error("expected WaitForChecksToStart not to be called without a PR")
	}
}

func TestCPWWatchCI_PRExistsButNoChecksWithinTimeout(t *testing.T) {
	// WaitForChecksToStart timed out with zero checks: cpw must not fail, and
	// must not pretend the PR is missing (issue #167).
	provider := &cpwFakeProvider{
		prNumber:   172,
		waitChecks: &remote.PRChecks{TotalCount: 0},
		waitErr:    errors.New("no CI checks found after 1m0s"),
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x"); err != nil {
		t.Fatalf("expected graceful exit when CI never starts, got: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no checks watch when no checks started")
	}
	if provider.waitTimeout != defaultCIStartTimeout {
		t.Errorf("expected cpw to wait %s for CI to start, got %s", defaultCIStartTimeout, provider.waitTimeout)
	}
}

func TestCPWWatchCI_WaitErrorIsPropagated(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitErr:  errors.New("API blew up"),
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "API blew up") {
		t.Fatalf("expected wrapped wait error, got: %v", err)
	}
}

func TestCPWWatchCI_ChecksAlreadyCompletedSuccess(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 2, Success: 2, Status: "success", HeadSHA: "abc1234def",
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no watch when checks already completed")
	}
}

func TestCPWWatchCI_ChecksAlreadyCompletedFailure(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 2, Success: 1, Failure: 1, Status: "failure", HeadSHA: "abc1234def",
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "checks failed") {
		t.Fatalf("expected checks-failed error, got: %v", err)
	}
}

func TestCPWWatchCI_WatchesPendingChecksToCompletion(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  sha,
		waitChecks: &remote.PRChecks{
			TotalCount: 2, Pending: 2, Status: "pending", HeadSHA: sha,
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 1, Pending: 1, Status: "pending", HeadSHA: sha}},
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 2, Pending: 0, Status: "success", HeadSHA: sha}},
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.watchCalled {
		t.Error("expected pending checks to be watched")
	}
}

func TestCPWWatchCI_FailureDuringWatchReturnsError(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  sha,
		waitChecks: &remote.PRChecks{
			TotalCount: 2, Pending: 2, Status: "pending", HeadSHA: sha,
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 1, Failure: 1, Pending: 0, Status: "failure", HeadSHA: sha}},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "1/2 checks failed") {
		t.Fatalf("expected failure summary in error, got: %v", err)
	}
}

func TestCPWWatchCI_HeadSHAChangeAborts(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 1, Pending: 1, Status: "pending", HeadSHA: "other000sha"}},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "HEAD SHA changed") {
		t.Fatalf("expected HEAD SHA change error, got: %v", err)
	}
}

func TestCPWWatchCI_StreamEndsWhilePendingIsAnError(t *testing.T) {
	// The update stream closing before checks complete (e.g. cancelled
	// context) must not be reported as success.
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: nil, // stream closes immediately
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "stopped watching") {
		t.Fatalf("expected 'stopped watching' error, got: %v", err)
	}
}

func TestCPWWatchCI_StreamErrorIsPropagated(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Error: errors.New("stream blew up")},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x")
	if err == nil || !strings.Contains(err.Error(), "stream blew up") {
		t.Fatalf("expected stream error, got: %v", err)
	}
}
