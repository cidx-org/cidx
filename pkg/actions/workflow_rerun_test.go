package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
)

// failedRun is the shape #342 is about: one job of a run died on the
// infrastructure while the rest of the run passed.
func failedRun() *remote.Workflow {
	return &remote.Workflow{
		ID:         "724",
		Status:     "completed",
		Conclusion: "failure",
		URL:        "https://github.com/cidx-org/cidx/actions/runs/724",
		Jobs: []remote.Job{
			{Name: "Security", Status: "completed", Conclusion: "success"},
			{Name: "Test", Status: "completed", Conclusion: "failure"},
		},
	}
}

// TestWorkflowRerunRestartsOnlyTheFailedJobs pins what `--failed` asks the
// provider for. Restarting the whole run instead would re-pay for the jobs that
// passed, which is the cost the flag exists to avoid.
func TestWorkflowRerunRestartsOnlyTheFailedJobs(t *testing.T) {
	provider := &fakeProvider{runs: map[string]*remote.Workflow{"724": failedRun()}}

	if err := NewWorkflowRerun(provider, "724", true).Execute(context.Background()); err != nil {
		t.Fatalf("rerun: %v", err)
	}

	if len(provider.rerunCalls) != 1 {
		t.Fatalf("want one rerun request, got %d", len(provider.rerunCalls))
	}
	if call := provider.rerunCalls[0]; call.runID != "724" || !call.failedOnly {
		t.Errorf("want a failed-jobs rerun of 724, got %+v", call)
	}
}

// TestWorkflowRerunRestartsEveryJobWithoutTheFlag: the default follows `gh run
// rerun`, so the flag means the same thing in both tools.
func TestWorkflowRerunRestartsEveryJobWithoutTheFlag(t *testing.T) {
	provider := &fakeProvider{runs: map[string]*remote.Workflow{"724": failedRun()}}

	if err := NewWorkflowRerun(provider, "724", false).Execute(context.Background()); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if call := provider.rerunCalls[0]; call.failedOnly {
		t.Errorf("the default asked for a failed-jobs rerun: %+v", call)
	}
}

// TestWorkflowRerunRefusesARunWithNoFailedJob: GitHub answers that request with
// a bare 403 whose body says the run has no failed jobs. Reading the run first
// turns it into a sentence naming the command that does work.
func TestWorkflowRerunRefusesARunWithNoFailedJob(t *testing.T) {
	green := &remote.Workflow{
		ID: "725", Status: "completed", Conclusion: "success",
		Jobs: []remote.Job{{Name: "Test", Status: "completed", Conclusion: "success"}},
	}
	provider := &fakeProvider{runs: map[string]*remote.Workflow{"725": green}}

	err := NewWorkflowRerun(provider, "725", true).Execute(context.Background())
	if err == nil {
		t.Fatal("a run with no failed job was accepted for --failed")
	}
	if !strings.Contains(err.Error(), "cidx repo workflow rerun 725") {
		t.Errorf("the refusal does not name the command that works: %v", err)
	}
	if len(provider.rerunCalls) != 0 {
		t.Errorf("the provider was called anyway: %+v", provider.rerunCalls)
	}
}

// TestWorkflowRerunSaysWhichIdentifierItTakes: `list` prints a run number next
// to the run ID and handing over the wrong one is the mistake #291 was about.
// The 404 has to say which column to read.
func TestWorkflowRerunSaysWhichIdentifierItTakes(t *testing.T) {
	provider := &fakeProvider{runErr: map[string]error{"640": errors.New("404 Not Found")}}

	err := NewWorkflowRerun(provider, "640", true).Execute(context.Background())
	if err == nil {
		t.Fatal("an unknown run was accepted")
	}
	if !strings.Contains(err.Error(), "cidx repo workflow list") {
		t.Errorf("the error does not say where the identifier comes from: %v", err)
	}
}

// TestWorkflowRerunReportsAProviderRefusal: nothing is swallowed.
func TestWorkflowRerunReportsAProviderRefusal(t *testing.T) {
	provider := &fakeProvider{
		runs:     map[string]*remote.Workflow{"724": failedRun()},
		rerunErr: errors.New("403 Forbidden"),
	}

	err := NewWorkflowRerun(provider, "724", true).Execute(context.Background())
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("want the provider's refusal, got %v", err)
	}
}

// TestWorkflowListAsksForTheBranchAcrossWorkflows covers the other half of
// #342: with no workflow named, the listing is the repository-wide one filtered
// on a branch -- the question you have when a check has just failed and you do
// not yet know which workflow owns it.
func TestWorkflowListAsksForTheBranchAcrossWorkflows(t *testing.T) {
	provider := &fakeProvider{listed: listedRuns()}

	out := captureStdout(t, func() {
		if err := NewWorkflowList(provider, "", "main", 10, false).Execute(context.Background()); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	if len(provider.listCalls) != 1 {
		t.Fatalf("want one listing request, got %d", len(provider.listCalls))
	}
	if call := provider.listCalls[0]; call.workflow != "" || call.branch != "main" {
		t.Errorf("want every workflow on main, got %+v", call)
	}

	// Naming the workflow each run belongs to is the whole reason for this view.
	for _, name := range []string{"CI", "Security Audit"} {
		if !strings.Contains(out, name) {
			t.Errorf("the branch listing does not name the workflow %q:\n%s", name, out)
		}
	}
}

// TestWorkflowListKeepsTheNamedWorkflowView: naming a workflow still lists that
// workflow, across branches, which is what it has always done.
func TestWorkflowListKeepsTheNamedWorkflowView(t *testing.T) {
	provider := &fakeProvider{listed: listedRuns()}

	captureStdout(t, func() {
		if err := NewWorkflowList(provider, "ci", "", 5, false).Execute(context.Background()); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	if call := provider.listCalls[0]; call.workflow != "ci" || call.branch != "" || call.limit != 5 {
		t.Errorf("want the ci workflow across branches, capped at 5, got %+v", call)
	}
}
