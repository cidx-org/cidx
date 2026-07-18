package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
)

// cpwFakeProvider extends fakeProvider with call-count-aware workflow lookup
// and a configurable PR-by-branch answer, to exercise the cpw retry loop.
type cpwFakeProvider struct {
	fakeProvider
	latestFn func(call int) (*remote.Workflow, error)
	calls    int
	prNumber int
	prErr    error
}

func (f *cpwFakeProvider) GetLatestWorkflow(_ context.Context, _ string) (*remote.Workflow, error) {
	f.calls++
	return f.latestFn(f.calls)
}

func (f *cpwFakeProvider) GetPullRequestByBranch(_ context.Context, _ string) (int, string, error) {
	if f.prErr != nil {
		return 0, "", f.prErr
	}
	return f.prNumber, "https://example.test/pr", nil
}

// newCPWAction builds an action wired for unit tests: no repo (the retry and
// hint logic never touch it), instant sleeps recorded into slept.
func newCPWAction(provider remote.Provider, timeout time.Duration, slept *[]time.Duration) *CommitPushWatchAction {
	return &CommitPushWatchAction{
		provider:       provider,
		ciStartTimeout: timeout,
		sleepFn: func(d time.Duration) {
			*slept = append(*slept, d)
		},
	}
}

func TestCPWWaitForWorkflow_RetriesUntilRunAppears(t *testing.T) {
	found := &remote.Workflow{ID: "42", Status: "in_progress", URL: "https://example.test/runs/42"}
	provider := &cpwFakeProvider{
		latestFn: func(call int) (*remote.Workflow, error) {
			if call < 4 {
				return nil, errors.New("no workflow runs found")
			}
			return found, nil
		},
	}

	var slept []time.Duration
	action := newCPWAction(provider, 60*time.Second, &slept)

	workflow, err := action.waitForWorkflow(context.Background(), "feat/x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workflow.ID != "42" {
		t.Errorf("expected workflow 42, got %s", workflow.ID)
	}
	if provider.calls != 4 {
		t.Errorf("expected 4 lookup attempts, got %d", provider.calls)
	}

	// Backoff: 3s, then x1.5 capped at 10s.
	want := []time.Duration{3 * time.Second, 4500 * time.Millisecond, 6750 * time.Millisecond, 10 * time.Second}
	if len(slept) != len(want) {
		t.Fatalf("expected %d sleeps, got %d: %v", len(want), len(slept), slept)
	}
	for i, d := range want {
		if slept[i] != d {
			t.Errorf("sleep %d: expected %s, got %s", i, d, slept[i])
		}
	}
}

func TestCPWWaitForWorkflow_GivesUpAfterTimeout(t *testing.T) {
	lookupErr := errors.New("no workflow runs found")
	provider := &cpwFakeProvider{
		latestFn: func(int) (*remote.Workflow, error) { return nil, lookupErr },
	}

	var slept []time.Duration
	action := newCPWAction(provider, 60*time.Second, &slept)

	_, err := action.waitForWorkflow(context.Background(), "feat/x")
	if !errors.Is(err, lookupErr) {
		t.Fatalf("expected last lookup error, got: %v", err)
	}

	var total time.Duration
	for _, d := range slept {
		total += d
	}
	if total < 60*time.Second {
		t.Errorf("expected cumulative wait >= 60s before giving up, got %s over %d attempts", total, provider.calls)
	}
	if provider.calls < 5 {
		t.Errorf("expected several retry attempts over the timeout window, got %d", provider.calls)
	}
}

func TestCPWWaitForWorkflow_ContextCancelled(t *testing.T) {
	provider := &cpwFakeProvider{
		latestFn: func(int) (*remote.Workflow, error) { return nil, errors.New("should not be called") },
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var slept []time.Duration
	action := newCPWAction(provider, 60*time.Second, &slept)

	_, err := action.waitForWorkflow(ctx, "feat/x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
	if provider.calls != 0 {
		t.Errorf("expected no lookup after cancellation, got %d", provider.calls)
	}
}

func TestCPWNoWorkflowHint_PRExists(t *testing.T) {
	provider := &cpwFakeProvider{prNumber: 172}

	var slept []time.Duration
	action := newCPWAction(provider, 60*time.Second, &slept)

	warning, hint := action.noWorkflowHint(context.Background(), "feat/x")
	if !strings.Contains(warning, "PR #172") {
		t.Errorf("expected warning to mention the existing PR, got: %q", warning)
	}
	if strings.Contains(hint, "Create a PR") {
		t.Errorf("hint must not suggest creating a PR that already exists, got: %q", hint)
	}
	if !strings.Contains(hint, "pr watch") {
		t.Errorf("expected hint to point at pr watch, got: %q", hint)
	}
}

func TestCPWNoWorkflowHint_NoPR(t *testing.T) {
	provider := &cpwFakeProvider{prErr: errors.New("no PR found")}

	var slept []time.Duration
	action := newCPWAction(provider, 60*time.Second, &slept)

	warning, hint := action.noWorkflowHint(context.Background(), "feat/x")
	if !strings.Contains(warning, "No CI workflow found") {
		t.Errorf("expected 'No CI workflow found' warning, got: %q", warning)
	}
	if !strings.Contains(hint, "Create a PR first") {
		t.Errorf("expected 'Create a PR first' hint, got: %q", hint)
	}
}
