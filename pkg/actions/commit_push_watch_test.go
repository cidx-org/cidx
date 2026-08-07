package actions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	log "github.com/sirupsen/logrus"
)

// cpwFakeProvider extends fakeProvider with configurable PR lookup, checks
// wait, and checks watch behavior, to exercise the cpw CI watch flow.
type cpwFakeProvider struct {
	fakeProvider

	prNumber int
	prErr    error

	waitSHA         string
	waitChecks      *remote.PRChecks
	waitErr         error
	waitTimeout     time.Duration // records the timeout cpw asked for
	waitExpectedSHA string        // records the expected SHA cpw pinned

	prTitle string

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

func (f *cpwFakeProvider) GetPullRequestTitle(_ context.Context, _ int) (string, error) {
	if f.prTitle == "" {
		return "", errors.New("no title staged")
	}
	return f.prTitle, nil
}

func (f *cpwFakeProvider) WaitForChecksToStart(_ context.Context, _ int, expectedSHA string, timeout time.Duration) (string, *remote.PRChecks, error) {
	f.waitExpectedSHA = expectedSHA
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

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def"); err != nil {
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

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def"); err != nil {
		t.Fatalf("expected graceful exit when CI never starts, got: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no checks watch when no checks started")
	}
	if provider.waitTimeout != defaultCIStartTimeout {
		t.Errorf("expected cpw to wait %s for CI to start, got %s", defaultCIStartTimeout, provider.waitTimeout)
	}
}

func TestCPWWatchCI_ForeignCheckAloneIsNotSuccess(t *testing.T) {
	// A green check from another app -- GitHub's own dependabot config check --
	// is not the CI. cpw must say no workflow ran instead of "All checks
	// passed", which would greenlight an unverified commit (issue #257).
	provider := &cpwFakeProvider{
		prNumber: 255,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, WorkflowChecks: 0, Success: 1, Status: "success", HeadSHA: "abc1234def",
		},
	}

	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def"); err != nil {
		t.Fatalf("expected graceful exit when no workflow started, got: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no checks watch when no workflow started")
	}
	if strings.Contains(buf.String(), "All checks passed") {
		t.Errorf("cpw announced success on a non-workflow check: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "No workflow started") {
		t.Errorf("expected an explicit 'no workflow started' warning, got: %s", buf.String())
	}
}

func TestCPWWatchCI_WorkflowChecksAlongsideForeignOneAreWatched(t *testing.T) {
	// Same PR once the workflow checks show up: the foreign check is still
	// there, but CI has really started, so watching proceeds to green.
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber: 255,
		waitSHA:  sha,
		waitChecks: &remote.PRChecks{
			TotalCount: 3, WorkflowChecks: 2, Success: 1, Pending: 2, Status: "pending", HeadSHA: sha,
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 3, WorkflowChecks: 2, Success: 3, Pending: 0, Status: "success", HeadSHA: sha}},
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", sha); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.watchCalled {
		t.Error("expected workflow checks to be watched to completion")
	}
}

func TestCPWWatchCI_WaitErrorIsPropagated(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitErr:  errors.New("API blew up"),
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
	if err == nil || !strings.Contains(err.Error(), "API blew up") {
		t.Fatalf("expected wrapped wait error, got: %v", err)
	}
}

func TestCPWWatchCI_ChecksAlreadyCompletedSuccess(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 2, WorkflowChecks: 2, Success: 2, Status: "success", HeadSHA: "abc1234def",
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.watchCalled {
		t.Error("expected no watch when checks already completed")
	}
	if provider.waitExpectedSHA != "abc1234def" {
		t.Errorf("expected cpw to pin the wait to the pushed SHA, got %q", provider.waitExpectedSHA)
	}
}

func TestCPWWatchCI_ChecksAlreadyCompletedFailure(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 2, WorkflowChecks: 2, Success: 1, Failure: 1, Status: "failure", HeadSHA: "abc1234def",
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
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
			TotalCount: 2, WorkflowChecks: 2, Pending: 2, Status: "pending", HeadSHA: sha,
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 1, Pending: 1, Status: "pending", HeadSHA: sha}},
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 2, Pending: 0, Status: "success", HeadSHA: sha}},
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def"); err != nil {
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
			TotalCount: 2, WorkflowChecks: 2, Pending: 2, Status: "pending", HeadSHA: sha,
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 2, Success: 1, Failure: 1, Pending: 0, Status: "failure", HeadSHA: sha}},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
	if err == nil || !strings.Contains(err.Error(), "1/2 checks failed") {
		t.Fatalf("expected failure summary in error, got: %v", err)
	}
}

func TestCPWWatchCI_HeadSHAChangeAborts(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, WorkflowChecks: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: &remote.PRChecks{TotalCount: 1, Pending: 1, Status: "pending", HeadSHA: "other000sha"}},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
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
			TotalCount: 1, WorkflowChecks: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: nil, // stream closes immediately
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
	if err == nil || !strings.Contains(err.Error(), "stopped watching") {
		t.Fatalf("expected 'stopped watching' error, got: %v", err)
	}
}

func TestCPWWatchCI_StreamErrorIsPropagated(t *testing.T) {
	provider := &cpwFakeProvider{
		prNumber: 172,
		waitSHA:  "abc1234def",
		waitChecks: &remote.PRChecks{
			TotalCount: 1, WorkflowChecks: 1, Pending: 1, Status: "pending", HeadSHA: "abc1234def",
		},
		checksUpdates: []remote.PRChecksUpdate{
			{Error: errors.New("stream blew up")},
		},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", "abc1234def")
	if err == nil || !strings.Contains(err.Error(), "stream blew up") {
		t.Fatalf("expected stream error, got: %v", err)
	}
}

// The gap issue #367 is about, in the numbers PR #366 actually produced:
// Bootstrap green, nothing pending, and four jobs that do not exist yet because
// a `needs:` has not made them eligible. Everything a check-counting watcher can
// see says the run is over.
func gapBetweenStages(sha string) *remote.PRChecks {
	return &remote.PRChecks{
		TotalCount: 1, WorkflowChecks: 1, Success: 1, Pending: 0,
		RunsInProgress: 1, Status: "pending", HeadSHA: sha,
	}
}

// TestCPWWatchCI_DoesNotConcludeInTheGapBetweenStages is the regression: cpw
// entered its watch on `Pending > 0`, so landing in the gap skipped the watch
// entirely and it announced a green CI having seen one job out of five.
func TestCPWWatchCI_DoesNotConcludeInTheGapBetweenStages(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber:   366,
		waitSHA:    sha,
		waitChecks: gapBetweenStages(sha),
		checksUpdates: []remote.PRChecksUpdate{
			{Checks: gapBetweenStages(sha)},
			{Checks: &remote.PRChecks{
				TotalCount: 5, WorkflowChecks: 5, Success: 5, Pending: 0,
				RunsInProgress: 0, Status: "success", HeadSHA: sha,
			}},
		},
	}

	if err := newCPWAction(provider).watchCI(context.Background(), "feat/x", sha); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.watchCalled {
		t.Error("cpw concluded without watching: one green check and a run still going " +
			"is the middle of a run, not the end of one (#367)")
	}
}

// TestCPWWatchCI_AGapThatNeverFillsIsNotSuccess: if the stream ends while the
// run is still going, the correct answer is the same one cpw already gives for
// a stream that ends with checks pending — not silence and exit 0.
func TestCPWWatchCI_AGapThatNeverFillsIsNotSuccess(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber:      366,
		waitSHA:       sha,
		waitChecks:    gapBetweenStages(sha),
		checksUpdates: []remote.PRChecksUpdate{{Checks: gapBetweenStages(sha)}},
	}

	err := newCPWAction(provider).watchCI(context.Background(), "feat/x", sha)
	if err == nil || !strings.Contains(err.Error(), "stopped watching before checks completed") {
		t.Fatalf("expected the incomplete watch to be reported, got: %v", err)
	}
}

// TestTypesDiffer covers issue #361: the commit says one kind of change, the
// pull request title says another, and the title is what reaches the
// changelog — through the squash subject, from a line nobody re-reads.
func TestTypesDiffer(t *testing.T) {
	tests := []struct {
		name    string
		message string
		title   string
		want    bool
	}{
		{
			// The case that produced the issue: work started as a feat, turned
			// out to be a fix, and the release would have filed it as Features.
			name:    "a fix committed under a feat title",
			message: "fix(pr): name the step a failing check died on",
			title:   "feat(pr): name the step a failing check died on",
			want:    true,
		},
		{
			name:    "the same type on both sides",
			message: "fix(pr): name the step",
			title:   "fix(pr): name the step a failing check died on",
			want:    false,
		},
		{
			// Only the type decides the changelog section, so a scope that
			// moved is not a disagreement worth interrupting for.
			name:    "a different scope is not a different type",
			message: "fix(cli): name the step",
			title:   "fix(pr): name the step",
			want:    false,
		},
		{
			name:    "a breaking marker on one side only",
			message: "feat(cli)!: remove the deprecated tree",
			title:   "feat(cli): remove the deprecated tree",
			want:    false,
		},
		{
			// A title that is not conventional makes no claim to contradict.
			name:    "a title that says nothing about its type",
			message: "fix(pr): name the step",
			title:   "Name the failing step",
			want:    false,
		},
		{
			name:    "a commit message that says nothing about its type",
			message: "wip",
			title:   "fix(pr): name the step",
			want:    false,
		},
		{
			// The body carries the BREAKING CHANGE footer and much else; only
			// the first line is the header.
			name:    "the type is read from the first line only",
			message: "fix(pr): name the step\n\nfeat: this word in the body is not the type\n",
			title:   "fix(pr): name the step",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commitType, titleType, differ := typesDiffer(tt.message, tt.title)

			if differ != tt.want {
				t.Fatalf("differ = %v, want %v (commit %q, title %q)", differ, tt.want, commitType, titleType)
			}
			if !differ {
				return
			}
			if commitType == "" || titleType == "" || commitType == titleType {
				t.Errorf("a disagreement must name both sides, got %q and %q", commitType, titleType)
			}
		})
	}
}

// TestCPWWatchCI_WarnsWhenTheTitleFilesItElsewhere: the warning reaches the
// user, naming both readings and the command that fixes it (#361).
func TestCPWWatchCI_WarnsWhenTheTitleFilesItElsewhere(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber: 361,
		prTitle:  "feat(pr): name the step a failing check died on",
		waitSHA:  sha,
		waitChecks: &remote.PRChecks{
			TotalCount: 2, WorkflowChecks: 2, Success: 2, Status: "success", HeadSHA: sha,
		},
	}

	action := newCPWAction(provider)
	action.message = "fix(pr): name the step a failing check died on"

	out := captureLogs(t)

	if err := action.watchCI(context.Background(), "fix/naming", sha); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{"'fix'", "'feat'", "cidx pr edit --title"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the warning does not mention %s:\n%s", want, out.String())
		}
	}
}

// TestCPWWatchCI_SaysNothingWhenTheTypesAgree: a warning that fires on the
// ordinary case is a warning people learn to scroll past.
func TestCPWWatchCI_SaysNothingWhenTheTypesAgree(t *testing.T) {
	sha := "abc1234def"
	provider := &cpwFakeProvider{
		prNumber: 361,
		prTitle:  "fix(pr): name the step a failing check died on",
		waitSHA:  sha,
		waitChecks: &remote.PRChecks{
			TotalCount: 2, WorkflowChecks: 2, Success: 2, Status: "success", HeadSHA: sha,
		},
	}

	action := newCPWAction(provider)
	action.message = "fix(pr): name the step"

	out := captureLogs(t)

	if err := action.watchCI(context.Background(), "fix/naming", sha); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(out.String(), "pull request is titled") {
		t.Errorf("warned about types that agree:\n%s", out.String())
	}
}
