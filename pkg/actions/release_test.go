package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// releaseFakeProvider extends fakeProvider with the PR calls the release
// orchestration makes: opening the bump PR and resolving an in-flight one.
type releaseFakeProvider struct {
	fakeProvider

	createCalls []createPRCall
	createErr   error

	lookupBranch string
	lookupErr    error
}

type createPRCall struct {
	title string
	body  string
	head  string
	base  string
	draft bool
}

func (f *releaseFakeProvider) CreatePullRequest(_ context.Context, title, body, head, base string, draft bool) (int, string, error) {
	f.createCalls = append(f.createCalls, createPRCall{title, body, head, base, draft})
	if f.createErr != nil {
		return 0, "", f.createErr
	}
	return 220, "https://example.test/pr/220", nil
}

func (f *releaseFakeProvider) GetPullRequestByBranch(_ context.Context, branch string) (int, string, error) {
	f.lookupBranch = branch
	if f.lookupErr != nil {
		return 0, "", f.lookupErr
	}
	return 220, "https://example.test/pr/220", nil
}

// gitRecorder replaces the runGit seam and records the plumbing issued.
type gitRecorder struct {
	calls  [][]string
	output map[string]string // first arg -> canned stdout
	fail   map[string]error  // first arg -> canned failure
}

func (g *gitRecorder) install(t *testing.T) {
	t.Helper()
	original := runGit
	runGit = func(_ string, args ...string) ([]byte, error) {
		g.calls = append(g.calls, args)
		if err, ok := g.fail[args[0]]; ok {
			return nil, err
		}
		return []byte(g.output[args[0]]), nil
	}
	t.Cleanup(func() { runGit = original })
}

// ran reports whether a git command starting with the given args was issued.
func (g *gitRecorder) ran(prefix ...string) bool {
	for _, call := range g.calls {
		if len(call) < len(prefix) {
			continue
		}
		match := true
		for i, want := range prefix {
			if call[i] != want {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// stubMerge replaces the mergeReleasePR seam with a canned outcome.
func stubMerge(t *testing.T, err error) *bool {
	t.Helper()
	called := false
	original := mergeReleasePR
	mergeReleasePR = func(_ context.Context, _ *vcs.Repository, _ remote.Provider) error {
		called = true
		return err
	}
	t.Cleanup(func() { mergeReleasePR = original })
	return &called
}

func TestReleaseViaPR_HappyPath(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)
	merged := stubMerge(t, nil)

	provider := &releaseFakeProvider{}
	action := &ReleaseAction{provider: provider}

	if err := action.releaseViaPR(context.Background(), "/repo", "main", "base000sha", "2.1.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The tag cz created locally must be dropped: the squash-merge rewrites it.
	if !git.ran("tag", "-d", "v2.1.4") {
		t.Error("expected the local bump tag to be deleted before the merge")
	}
	if !git.ran("checkout", "-B", "chore/release-v2.1.4") {
		t.Error("expected the bump commit to move onto a release branch")
	}
	if !git.ran("branch", "-f", "main", "base000sha") {
		t.Error("expected main to be restored to its pre-bump commit")
	}
	if !git.ran("push", "-u", "origin", "chore/release-v2.1.4") {
		t.Error("expected the release branch to be pushed")
	}
	if !*merged {
		t.Error("expected the release PR to be merged")
	}

	// The tag must be created after the merge, on the merged commit.
	if !git.ran("tag", "-a", "v2.1.4") {
		t.Fatal("expected an annotated tag on the merged commit")
	}
	if !git.ran("push", "origin", "v2.1.4") {
		t.Error("expected the tag to be pushed")
	}

	tagIdx, mergeIdx := -1, -1
	for i, call := range git.calls {
		if call[0] == "tag" && call[1] == "-a" {
			tagIdx = i
		}
		if call[0] == "push" && len(call) > 2 && call[1] == "-u" {
			mergeIdx = i // the push precedes the merge
		}
	}
	if tagIdx < mergeIdx || mergeIdx == -1 {
		t.Errorf("expected the tag to be created after the branch push/merge, got calls %v", git.calls)
	}

	if len(provider.createCalls) != 1 {
		t.Fatalf("expected exactly one PR to be created, got %d", len(provider.createCalls))
	}
	pr := provider.createCalls[0]
	if pr.head != "chore/release-v2.1.4" || pr.base != "main" {
		t.Errorf("expected PR chore/release-v2.1.4 -> main, got %s -> %s", pr.head, pr.base)
	}
	if pr.draft {
		t.Error("release PR must not be a draft: it has to run CI and be mergeable")
	}
	if !strings.HasPrefix(pr.title, "chore(release): bump version to v2.1.4") {
		t.Errorf("expected a conventional release PR title, got %q", pr.title)
	}
}

func TestReleaseViaPR_FailedCIDoesNotTag(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)
	stubMerge(t, errors.New("PR checks failed: 1/4 checks failed"))

	action := &ReleaseAction{provider: &releaseFakeProvider{}}

	err := action.releaseViaPR(context.Background(), "/repo", "main", "base000sha", "2.1.4")
	if err == nil {
		t.Fatal("expected an error when the release PR cannot be merged")
	}
	if !strings.Contains(err.Error(), "was not merged") {
		t.Errorf("expected an actionable 'not merged' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "https://example.test/pr/220") {
		t.Errorf("expected the PR URL in the error, got: %v", err)
	}
	if git.ran("tag", "-a", "v2.1.4") {
		t.Error("no tag must be created when the release PR was not merged")
	}
	if git.ran("push", "origin", "v2.1.4") {
		t.Error("no tag must be pushed when the release PR was not merged")
	}
}

func TestReleaseViaPR_PRCreationFailureStopsBeforeMerge(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)
	merged := stubMerge(t, nil)

	action := &ReleaseAction{provider: &releaseFakeProvider{createErr: errors.New("API blew up")}}

	err := action.releaseViaPR(context.Background(), "/repo", "main", "base000sha", "2.1.4")
	if err == nil || !strings.Contains(err.Error(), "API blew up") {
		t.Fatalf("expected the PR creation error, got: %v", err)
	}
	if *merged {
		t.Error("nothing should be merged when the PR could not be created")
	}
	if git.ran("tag", "-a", "v2.1.4") {
		t.Error("no tag must be created when the PR could not be created")
	}
}

func TestCheckNoReleaseInFlight_NoBranchIsClear(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)

	action := &ReleaseAction{provider: &releaseFakeProvider{}}

	if err := action.checkNoReleaseInFlight(context.Background(), "/repo"); err != nil {
		t.Fatalf("expected no in-flight release, got: %v", err)
	}
}

func TestCheckNoReleaseInFlight_ExistingBranchStopsWithoutDuplicating(t *testing.T) {
	git := &gitRecorder{
		output: map[string]string{
			"ls-remote": "d34db33f\trefs/heads/chore/release-v2.1.4\n",
		},
	}
	git.install(t)

	provider := &releaseFakeProvider{}
	action := &ReleaseAction{provider: provider}

	err := action.checkNoReleaseInFlight(context.Background(), "/repo")
	if err == nil {
		t.Fatal("expected the release to stop when one is already in flight")
	}
	if !strings.Contains(err.Error(), "chore/release-v2.1.4") {
		t.Errorf("expected the in-flight branch in the error, got: %v", err)
	}
	if provider.lookupBranch != "chore/release-v2.1.4" {
		t.Errorf("expected the existing PR to be looked up, got %q", provider.lookupBranch)
	}
	if len(provider.createCalls) != 0 {
		t.Error("must not open a second release PR")
	}
}

func TestCheckNoReleaseInFlight_UnresolvablePRStillStops(t *testing.T) {
	// The branch is published but its PR was closed: still refuse to start a
	// second release rather than racing two bump branches.
	git := &gitRecorder{
		output: map[string]string{
			"ls-remote": "d34db33f\trefs/heads/chore/release-v2.1.4\n",
		},
	}
	git.install(t)

	action := &ReleaseAction{provider: &releaseFakeProvider{lookupErr: errors.New("no PR found")}}

	if err := action.checkNoReleaseInFlight(context.Background(), "/repo"); err == nil {
		t.Fatal("expected the release to stop on a published release branch")
	}
}

// tagRunProvider answers the tag lookup with a canned sequence, so the wait for
// the run a tag push triggered can be driven through "not registered yet" and
// then "found".
type tagRunProvider struct {
	fakeProvider

	failures int // number of lookups that report no run before one appears
	calls    int
	run      *remote.Workflow
}

func (p *tagRunProvider) GetLatestRunForTag(_ context.Context, tag string) (*remote.Workflow, error) {
	p.calls++
	p.tagLookups = append(p.tagLookups, tag)
	if p.calls <= p.failures {
		return nil, errors.New("no workflow runs found for tag " + tag)
	}
	return p.run, nil
}

// shrinkTagWorkflowWait keeps the polling test fast.
func shrinkTagWorkflowWait(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	origTimeout, origInterval := tagWorkflowWaitTimeout, tagWorkflowPollInterval
	tagWorkflowWaitTimeout, tagWorkflowPollInterval = timeout, interval
	t.Cleanup(func() {
		tagWorkflowWaitTimeout, tagWorkflowPollInterval = origTimeout, origInterval
	})
}

func TestWaitForTagWorkflow_ResolvesTheRunTheTagTriggered(t *testing.T) {
	shrinkTagWorkflowWait(t, 50*time.Millisecond, time.Millisecond)

	releaseRun := &remote.Workflow{ID: "30283021151", Status: "in_progress"}
	provider := &tagRunProvider{run: releaseRun}
	action := &ReleaseAction{provider: provider}

	workflow, err := action.waitForTagWorkflow(context.Background(), "v2.1.4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if workflow.ID != releaseRun.ID {
		t.Errorf("got run %s, want the release run %s", workflow.ID, releaseRun.ID)
	}
	// Resolved by tag, not by the CI workflow candidates on a branch (#223).
	if len(provider.tagLookups) != 1 || provider.tagLookups[0] != "v2.1.4" {
		t.Errorf("expected a single lookup for tag v2.1.4, got %v", provider.tagLookups)
	}
}

func TestWaitForTagWorkflow_RetriesUntilTheRunIsRegistered(t *testing.T) {
	shrinkTagWorkflowWait(t, time.Second, time.Millisecond)

	// GitHub registers the run a moment after the push returns.
	provider := &tagRunProvider{failures: 2, run: &remote.Workflow{ID: "77"}}
	action := &ReleaseAction{provider: provider}

	workflow, err := action.waitForTagWorkflow(context.Background(), "v2.1.4")
	if err != nil {
		t.Fatalf("expected the wait to outlast the registration delay, got: %v", err)
	}
	if workflow.ID != "77" {
		t.Errorf("got run %s, want 77", workflow.ID)
	}
	if provider.calls != 3 {
		t.Errorf("expected 3 lookups (2 empty + 1 hit), got %d", provider.calls)
	}
}

func TestWaitForTagWorkflow_TimesOutWithTheLookupError(t *testing.T) {
	shrinkTagWorkflowWait(t, 5*time.Millisecond, time.Millisecond)

	provider := &tagRunProvider{failures: 1000}
	action := &ReleaseAction{provider: provider}

	_, err := action.waitForTagWorkflow(context.Background(), "v2.1.4")
	if err == nil {
		t.Fatal("expected an error when no run ever appears for the tag")
	}
	if !strings.Contains(err.Error(), "v2.1.4") {
		t.Errorf("expected the tag in the error, got: %v", err)
	}
}

func TestWaitForTagWorkflow_StopsOnCancelledContext(t *testing.T) {
	shrinkTagWorkflowWait(t, time.Hour, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	action := &ReleaseAction{provider: &tagRunProvider{failures: 1000}}
	if _, err := action.waitForTagWorkflow(ctx, "v2.1.4"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the wait to honour cancellation, got: %v", err)
	}
}

func TestReleaseBranchInFlight_IgnoresUnrelatedOutput(t *testing.T) {
	git := &gitRecorder{
		fail: map[string]error{"ls-remote": errors.New("network down")},
	}
	git.install(t)

	if branch := releaseBranchInFlight("/repo"); branch != "" {
		t.Errorf("a failed remote lookup must not block the release, got %q", branch)
	}
}

// TestRelease_ProviderIsNotResolvedUntilNeeded covers #227: creating the
// remote provider reads `origin`, so doing it up front made every release step
// -- including the ones that only read the local repository -- fail on a repo
// without a remote. Nothing must resolve it while no release is in flight.
func TestRelease_ProviderIsNotResolvedUntilNeeded(t *testing.T) {
	git := &gitRecorder{}
	git.install(t)

	resolved := 0
	action := NewRelease(nil, func() (remote.Provider, error) {
		resolved++
		return nil, errors.New("failed to get remote URL: remote not found")
	}, config.ReleaseConfig{}, "release-create", true)

	if err := action.checkNoReleaseInFlight(context.Background(), "/repo"); err != nil {
		t.Fatalf("expected no in-flight release, got: %v", err)
	}
	if resolved != 0 {
		t.Errorf("the remote must not be resolved when no step needs it, resolved %d time(s)", resolved)
	}
}

// TestRelease_ProviderResolvedOnceWhenNeeded checks the other half: a step that
// does need the remote gets it, and repeated steps reuse the same instance.
func TestRelease_ProviderResolvedOnceWhenNeeded(t *testing.T) {
	resolved := 0
	provider := &tagRunProvider{run: &remote.Workflow{ID: "42"}}
	action := NewRelease(nil, func() (remote.Provider, error) {
		resolved++
		return provider, nil
	}, config.ReleaseConfig{}, "release-create", false)

	for i := range 2 {
		if _, err := action.waitForTagWorkflow(context.Background(), "v2.1.4"); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}

	if resolved != 1 {
		t.Errorf("expected the provider to be resolved once and cached, got %d", resolved)
	}
}

// TestRelease_ProviderErrorSurfacesAtTheStepThatNeedsIt keeps the failure
// honest: deferring resolution must not swallow it.
func TestRelease_ProviderErrorSurfacesAtTheStepThatNeedsIt(t *testing.T) {
	action := NewRelease(nil, func() (remote.Provider, error) {
		return nil, errors.New("failed to get remote URL: remote not found")
	}, config.ReleaseConfig{}, "release-create", false)

	_, err := action.waitForTagWorkflow(context.Background(), "v2.1.4")
	if err == nil || !strings.Contains(err.Error(), "remote not found") {
		t.Fatalf("expected the resolver error, got: %v", err)
	}
}
