package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	log "github.com/sirupsen/logrus"
)

// WorkflowRerunAction restarts a run, or only the jobs of it that failed.
//
// It closes the recovery half of the workflow loop: a job that dies pulling an
// image (`read: connection reset by peer`, run #724 in issue #342) failed on the
// infrastructure, not on the change, and the only way back was `gh run rerun
// --failed` -- the one step that still had to leave cidx, reached for precisely
// when something has just gone wrong.
type WorkflowRerunAction struct {
	provider   remote.Provider
	runID      string
	failedOnly bool
}

// NewWorkflowRerun creates a workflow rerun action. runID must already be
// resolved by the caller (the CLI defaults it to the latest run on the current
// branch).
func NewWorkflowRerun(provider remote.Provider, runID string, failedOnly bool) *WorkflowRerunAction {
	return &WorkflowRerunAction{
		provider:   provider,
		runID:      runID,
		failedOnly: failedOnly,
	}
}

// Execute restarts the run and names the command that follows it.
//
// The run is read first, for two reasons: a wrong identifier is reported as such
// -- `list` prints a run number next to the ID and handing over the wrong one is
// the mistake #291 was about -- and `--failed` on a run with no failed job is
// refused here rather than as the provider's bare 403.
func (a *WorkflowRerunAction) Execute(ctx context.Context) error {
	if a.runID == "" {
		return fmt.Errorf("a run ID is required")
	}

	run, err := a.provider.GetWorkflowRun(ctx, a.runID)
	if err != nil {
		return fmt.Errorf("run %s could not be read -- the identifier is the `id` column of `cidx repo workflow list`, not the `#` run number beside it: %w", a.runID, err)
	}

	if a.failedOnly {
		failed := failedJobs(run)
		if len(failed) == 0 {
			return fmt.Errorf("run %s has no failed job to rerun (status %s, conclusion %q); rerun all of it with: cidx repo workflow rerun %s",
				a.runID, run.Status, run.Conclusion, a.runID)
		}
		log.Infof("🔁 Rerunning the failed job(s) of run %s: %s", a.runID, strings.Join(failed, ", "))
	} else {
		log.Infof("🔁 Rerunning every job of run %s", a.runID)
	}

	if err := a.provider.RerunWorkflow(ctx, a.runID, a.failedOnly); err != nil {
		return err
	}

	log.Info("✅ Rerun requested")
	if run.URL != "" {
		log.Infof("🔗 %s", run.URL)
	}
	// Not watched from here: a rerun starts a new attempt of the same run and
	// the API goes on reporting the previous one as completed for a few seconds,
	// so a watch chained onto this would report the very failure it was asked to
	// clear. Naming the command leaves that second to the person typing it.
	log.Infof("💡 Follow it with: cidx repo workflow watch %s", a.runID)
	return nil
}

// failedJobs names the jobs a rerun --failed would restart. Cancelled counts:
// GitHub restarts those too, and a job the runner lost is the same accident as
// one whose pull was reset.
func failedJobs(run *remote.Workflow) []string {
	var failed []string
	for _, job := range run.Jobs {
		switch job.Conclusion {
		case "failure", "cancelled", "timed_out":
			failed = append(failed, job.Name)
		}
	}
	return failed
}
