package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/remote"
)

// stubRefExists replaces the remote-ref probe for the duration of a test, so
// nothing here shells out to git or touches the network.
func stubRefExists(t *testing.T, exists bool, err error) {
	t.Helper()
	original := refExistsOnRemote
	refExistsOnRemote = func(string) (bool, error) { return exists, err }
	t.Cleanup(func() { refExistsOnRemote = original })
}

func TestParseWorkflowInputs(t *testing.T) {
	t.Run("no inputs yields no map", func(t *testing.T) {
		got, err := ParseWorkflowInputs(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("parses repeated pairs", func(t *testing.T) {
		got, err := ParseWorkflowInputs([]string{"dry_run=true", "level=HIGH"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["dry_run"] != "true" || got["level"] != "HIGH" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("passes values through untouched", func(t *testing.T) {
		// Anything after the first '=' is the value, spaces and all: the
		// provider forwards it as given.
		got, err := ParseWorkflowInputs([]string{"query=a=b c", "empty="})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["query"] != "a=b c" {
			t.Errorf("got %q, want %q", got["query"], "a=b c")
		}
		if v, ok := got["empty"]; !ok || v != "" {
			t.Errorf("an explicitly empty value must be kept, got %v", got)
		}
	})

	t.Run("rejects a pair without =", func(t *testing.T) {
		_, err := ParseWorkflowInputs([]string{"dry_run"})
		if err == nil || !strings.Contains(err.Error(), "key=value") {
			t.Fatalf("expected a key=value error, got: %v", err)
		}
	})

	t.Run("rejects an empty key", func(t *testing.T) {
		if _, err := ParseWorkflowInputs([]string{"=true"}); err == nil {
			t.Fatal("expected an error for an empty key")
		}
	})

	t.Run("rejects a key given twice", func(t *testing.T) {
		_, err := ParseWorkflowInputs([]string{"dry_run=true", "dry_run=false"})
		if err == nil || !strings.Contains(err.Error(), "twice") {
			t.Fatalf("expected a duplicate-key error, got: %v", err)
		}
	})
}

func TestWorkflowRun_TriggersThenWatches(t *testing.T) {
	stubRefExists(t, true, nil)

	started := &remote.Workflow{ID: "555", Status: "in_progress", URL: "https://example.test/runs/555"}
	done := &remote.Workflow{ID: "555", Status: "completed", Conclusion: "success", URL: started.URL}

	provider := &fakeProvider{
		triggered:    started,
		runs:         map[string]*remote.Workflow{"555": started},
		watchUpdates: []remote.WorkflowUpdate{{Workflow: started}, {Workflow: done}},
	}

	action := NewWorkflowRun(provider, "go-version-check.yml", "feat/x", map[string]string{"dry_run": "true"}, true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(provider.triggerCalls) != 1 {
		t.Fatalf("expected one dispatch, got %d", len(provider.triggerCalls))
	}
	call := provider.triggerCalls[0]
	if call.workflow != "go-version-check.yml" || call.ref != "feat/x" {
		t.Errorf("dispatched %q on %q, want go-version-check.yml on feat/x", call.workflow, call.ref)
	}
	if call.inputs["dry_run"] != "true" {
		t.Errorf("inputs were not passed through: %v", call.inputs)
	}
}

func TestWorkflowRun_WatchesTheRunTheDispatchCreated(t *testing.T) {
	stubRefExists(t, true, nil)

	// The run is resolved by ID, not by re-reading the branch: a run somebody
	// else started on the same branch must not be watched instead of ours.
	ours := &remote.Workflow{ID: "777", Status: "completed", Conclusion: "success"}
	provider := &fakeProvider{
		triggered:       ours,
		latestForBranch: map[string]*remote.Workflow{"feat/x": {ID: "888", Status: "completed", Conclusion: "failure"}},
		runs:            map[string]*remote.Workflow{"777": ours},
	}

	action := NewWorkflowRun(provider, "ci.yml", "feat/x", nil, true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("expected our run to be watched, got the branch's: %v", err)
	}
}

func TestWorkflowRun_NoWatchReturnsAfterDispatch(t *testing.T) {
	stubRefExists(t, true, nil)

	provider := &fakeProvider{
		triggered: &remote.Workflow{ID: "42", Status: "queued", URL: "https://example.test/runs/42"},
		// Watching would surface this error.
		watchErr: errors.New("WatchWorkflow must not be called with --no-watch"),
	}

	action := NewWorkflowRun(provider, "ci.yml", "main", nil, false)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkflowRun_FailedRunIsReportedAsAnError(t *testing.T) {
	stubRefExists(t, true, nil)

	failed := &remote.Workflow{ID: "13", Status: "completed", Conclusion: "failure"}
	provider := &fakeProvider{triggered: failed, runs: map[string]*remote.Workflow{"13": failed}}

	action := NewWorkflowRun(provider, "ci.yml", "main", nil, true)
	err := action.Execute(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("expected the failed run to be reported, got: %v", err)
	}
}

func TestWorkflowRun_RefNotPushedFailsBeforeDispatching(t *testing.T) {
	stubRefExists(t, false, nil)

	provider := &fakeProvider{
		triggerErr: errors.New("the dispatch must not be attempted"),
	}

	action := NewWorkflowRun(provider, "ci.yml", "feat/local-only", nil, true)
	err := action.Execute(context.Background())
	if err == nil {
		t.Fatal("expected an error for a ref that is not on origin")
	}
	for _, want := range []string{"feat/local-only", "not on origin", "Push it first"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %q", err, want)
		}
	}
	if len(provider.triggerCalls) != 0 {
		t.Errorf("nothing should have been dispatched, got %d call(s)", len(provider.triggerCalls))
	}
}

func TestWorkflowRun_UnverifiableRefStillDispatches(t *testing.T) {
	// No origin, no network: the check cannot answer, so it must not block --
	// the provider reports whatever is actually wrong.
	stubRefExists(t, false, errors.New("git ls-remote failed"))

	succeeded := &remote.Workflow{ID: "1", Status: "completed", Conclusion: "success"}
	provider := &fakeProvider{triggered: succeeded, runs: map[string]*remote.Workflow{"1": succeeded}}

	action := NewWorkflowRun(provider, "ci.yml", "main", nil, true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(provider.triggerCalls) != 1 {
		t.Errorf("expected the dispatch to be attempted, got %d call(s)", len(provider.triggerCalls))
	}
}

func TestWorkflowRun_TriggerErrorIsSurfacedVerbatim(t *testing.T) {
	stubRefExists(t, true, nil)

	// The provider owns the wording of "no workflow_dispatch trigger" and
	// friends; the action must not bury it under a generic message.
	triggerErr := errors.New("workflow ci.yml cannot be triggered: it does not declare the 'workflow_dispatch' trigger")
	provider := &fakeProvider{triggerErr: triggerErr}

	action := NewWorkflowRun(provider, "ci.yml", "main", nil, true)
	err := action.Execute(context.Background())
	if err == nil || !errors.Is(err, triggerErr) {
		t.Fatalf("expected the provider error, got: %v", err)
	}
}

func TestWorkflowRun_RequiresAWorkflowAndARef(t *testing.T) {
	stubRefExists(t, true, nil)
	provider := &fakeProvider{}

	if err := NewWorkflowRun(provider, "", "main", nil, false).Execute(context.Background()); err == nil {
		t.Error("expected an error without a workflow name")
	}
	if err := NewWorkflowRun(provider, "ci.yml", "", nil, false).Execute(context.Background()); err == nil {
		t.Error("expected an error without a ref")
	}
}
