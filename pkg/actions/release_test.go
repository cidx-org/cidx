package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
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

func TestReleaseBranchInFlight_IgnoresUnrelatedOutput(t *testing.T) {
	git := &gitRecorder{
		fail: map[string]error{"ls-remote": errors.New("network down")},
	}
	git.install(t)

	if branch := releaseBranchInFlight("/repo"); branch != "" {
		t.Errorf("a failed remote lookup must not block the release, got %q", branch)
	}
}
