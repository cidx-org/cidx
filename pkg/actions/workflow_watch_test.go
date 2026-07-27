package actions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
)

// fakeProvider is a minimal remote.Provider implementation for unit-testing the
// workflow watch action. Only the methods exercised by WorkflowWatchAction are
// implemented with real behavior; the rest panic so missing-stub bugs are loud.
type fakeProvider struct {
	latestForBranch map[string]*remote.Workflow
	latestErr       map[string]error
	latestForTag    map[string]*remote.Workflow
	tagErr          map[string]error
	tagLookups      []string
	runs            map[string]*remote.Workflow
	runErr          map[string]error
	watchUpdates    []remote.WorkflowUpdate
	watchErr        error
}

func (f *fakeProvider) GetLatestWorkflow(_ context.Context, branch string) (*remote.Workflow, error) {
	return f.GetLatestRunForBranch(context.Background(), branch)
}

func (f *fakeProvider) GetLatestRunForBranch(_ context.Context, branch string) (*remote.Workflow, error) {
	if err, ok := f.latestErr[branch]; ok {
		return nil, err
	}
	if w, ok := f.latestForBranch[branch]; ok {
		return w, nil
	}
	return nil, errors.New("no workflow runs found")
}

func (f *fakeProvider) GetLatestRunForTag(_ context.Context, tag string) (*remote.Workflow, error) {
	f.tagLookups = append(f.tagLookups, tag)
	if err, ok := f.tagErr[tag]; ok {
		return nil, err
	}
	if w, ok := f.latestForTag[tag]; ok {
		return w, nil
	}
	return nil, errors.New("no workflow runs found for tag " + tag)
}

func (f *fakeProvider) GetWorkflowRun(_ context.Context, runID string) (*remote.Workflow, error) {
	if err, ok := f.runErr[runID]; ok {
		return nil, err
	}
	if w, ok := f.runs[runID]; ok {
		return w, nil
	}
	return nil, errors.New("run not found")
}

func (f *fakeProvider) WatchWorkflow(_ context.Context, _ string) (<-chan remote.WorkflowUpdate, error) {
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	ch := make(chan remote.WorkflowUpdate, len(f.watchUpdates))
	for _, u := range f.watchUpdates {
		ch <- u
	}
	close(ch)
	return ch, nil
}

// Unused methods -- kept minimal to satisfy the remote.Provider interface.
func (f *fakeProvider) CreatePullRequest(_ context.Context, _, _, _, _ string, _ bool) (int, string, error) {
	panic("not implemented in fake")
}
func (f *fakeProvider) MarkPullRequestReady(_ context.Context, _ int) error {
	panic("not implemented in fake")
}
func (f *fakeProvider) GetPullRequestByBranch(_ context.Context, _ string) (int, string, error) {
	panic("not implemented in fake")
}
func (f *fakeProvider) MergePullRequest(_ context.Context, _ int, _ string) error {
	panic("not implemented in fake")
}
func (f *fakeProvider) UpdatePullRequest(_ context.Context, _ int, _, _ string) error {
	panic("not implemented in fake")
}
func (f *fakeProvider) GetPullRequestChecks(_ context.Context, _ int) (*remote.PRChecks, error) {
	panic("not implemented in fake")
}
func (f *fakeProvider) WaitForChecksToStart(_ context.Context, _ int, _ string, _ time.Duration) (string, *remote.PRChecks, error) {
	panic("not implemented in fake")
}
func (f *fakeProvider) WatchPullRequestChecks(_ context.Context, _ int) (<-chan remote.PRChecksUpdate, error) {
	panic("not implemented in fake")
}

// Compile-time assertion that fakeProvider satisfies remote.Provider.
var _ remote.Provider = (*fakeProvider)(nil)

func TestWorkflowWatch_ResolveByBranch_Success(t *testing.T) {
	completed := &remote.Workflow{
		ID:         "111",
		Status:     "completed",
		Conclusion: "success",
		URL:        "https://example.test/runs/111",
	}
	provider := &fakeProvider{
		latestForBranch: map[string]*remote.Workflow{
			"main": completed,
		},
	}

	action := NewWorkflowWatch(provider, "main", "", "", true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowWatch_ResolveByBranch_NoRunsFound(t *testing.T) {
	provider := &fakeProvider{
		latestErr: map[string]error{
			"feature-x": errors.New("no workflow runs found for branch feature-x"),
		},
	}

	action := NewWorkflowWatch(provider, "feature-x", "", "", true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error when no runs are found, got nil")
	}
	if !strings.Contains(err.Error(), "no workflow run found for branch") {
		t.Errorf("expected 'no workflow run found for branch' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "feature-x") {
		t.Errorf("expected branch name in error, got: %v", err)
	}
}

func TestWorkflowWatch_ResolveByRunID(t *testing.T) {
	completed := &remote.Workflow{
		ID:         "9999",
		Status:     "completed",
		Conclusion: "success",
		URL:        "https://example.test/runs/9999",
	}
	provider := &fakeProvider{
		runs: map[string]*remote.Workflow{
			"9999": completed,
		},
	}

	// Branch is not consulted when runID is provided.
	action := NewWorkflowWatch(provider, "ignored-branch", "", "9999", true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowWatch_ResolveByTag_Success(t *testing.T) {
	releaseRun := &remote.Workflow{
		ID:         "30283021151",
		Status:     "completed",
		Conclusion: "success",
		URL:        "https://example.test/runs/30283021151",
	}
	provider := &fakeProvider{
		latestForTag: map[string]*remote.Workflow{
			"v2.1.4": releaseRun,
		},
		// A completed run must be reported without streaming: watching it
		// would surface this error.
		watchErr: errors.New("WatchWorkflow must not be called for a completed run"),
	}

	action := NewWorkflowWatch(provider, "", "v2.1.4", "", true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(provider.tagLookups) != 1 || provider.tagLookups[0] != "v2.1.4" {
		t.Errorf("expected the run to be resolved by tag v2.1.4, got lookups %v", provider.tagLookups)
	}
}

func TestWorkflowWatch_ResolveByTag_NoRunFound(t *testing.T) {
	provider := &fakeProvider{
		tagErr: map[string]error{
			"v9.9.9": errors.New("no workflow runs found for tag v9.9.9"),
		},
	}

	action := NewWorkflowWatch(provider, "", "v9.9.9", "", true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected an error when no run matches the tag")
	}
	if !strings.Contains(err.Error(), "no workflow run found for tag") {
		t.Errorf("expected a tag-specific error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "v9.9.9") {
		t.Errorf("expected the tag name in the error, got: %v", err)
	}
}

func TestWorkflowWatch_TagWinsOverBranch(t *testing.T) {
	// A tag push and the branch push of the same commit produce different runs;
	// --tag must select the tag's one.
	provider := &fakeProvider{
		latestForBranch: map[string]*remote.Workflow{
			"main": {ID: "1", Status: "completed", Conclusion: "failure"},
		},
		latestForTag: map[string]*remote.Workflow{
			"v2.1.4": {ID: "2", Status: "completed", Conclusion: "success"},
		},
	}

	action := NewWorkflowWatch(provider, "main", "v2.1.4", "", true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("expected the tag run to be watched, got error from the branch run: %v", err)
	}
}

func TestWorkflowWatch_RunIDNotFound(t *testing.T) {
	provider := &fakeProvider{
		runErr: map[string]error{
			"42": errors.New("run not found"),
		},
	}

	action := NewWorkflowWatch(provider, "", "", "42", true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error for unknown run ID")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("expected run ID in error, got: %v", err)
	}
}

func TestWorkflowWatch_NoBranchOrRunID(t *testing.T) {
	provider := &fakeProvider{}
	action := NewWorkflowWatch(provider, "", "", "", true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error when neither branch nor run ID is provided")
	}
}

func TestWorkflowWatch_StreamsUpdatesUntilCompletion(t *testing.T) {
	inProgress := &remote.Workflow{
		ID:     "10",
		Status: "in_progress",
		URL:    "https://example.test/runs/10",
	}
	completed := &remote.Workflow{
		ID:         "10",
		Status:     "completed",
		Conclusion: "success",
		URL:        "https://example.test/runs/10",
	}

	provider := &fakeProvider{
		latestForBranch: map[string]*remote.Workflow{
			"main": inProgress,
		},
		watchUpdates: []remote.WorkflowUpdate{
			{Workflow: inProgress},
			{Workflow: completed},
		},
	}

	action := NewWorkflowWatch(provider, "main", "", "", true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowWatch_FailureConclusionReturnsError(t *testing.T) {
	failed := &remote.Workflow{
		ID:         "20",
		Status:     "completed",
		Conclusion: "failure",
		URL:        "https://example.test/runs/20",
	}
	provider := &fakeProvider{
		latestForBranch: map[string]*remote.Workflow{
			"main": failed,
		},
	}

	action := NewWorkflowWatch(provider, "main", "", "", true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error when workflow concluded with failure")
	}
	if !strings.Contains(err.Error(), "failure") {
		t.Errorf("expected 'failure' in error, got: %v", err)
	}
}

func TestWorkflowWatch_StreamUpdateError(t *testing.T) {
	inProgress := &remote.Workflow{
		ID:     "30",
		Status: "in_progress",
	}
	provider := &fakeProvider{
		latestForBranch: map[string]*remote.Workflow{
			"main": inProgress,
		},
		watchUpdates: []remote.WorkflowUpdate{
			{Error: errors.New("API blew up")},
		},
	}

	action := NewWorkflowWatch(provider, "main", "", "", true)
	err := action.Execute(context.Background())
	if err == nil || !strings.Contains(err.Error(), "API blew up") {
		t.Fatalf("expected wrapped stream error, got: %v", err)
	}
}
