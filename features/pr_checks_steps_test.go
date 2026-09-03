package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cucumber/godog"
)

// RegisterPRChecksSteps registers the steps for what `cidx pr status` and
// `cidx pr watch` say about a check that failed (issue #347).
//
// The staged checks go through branch.ChecksInfo -- the real reduction of a
// provider's answer, the one GetPRInfo applies -- and the report is the real
// renderer the commands print. Nothing about which check counts as a failure,
// or how a failure reads, is decided here. That the two commands do print these
// reports is covered by the unit tests of internal/commands, which drive the
// watch loops through a package-level seam.
func RegisterPRChecksSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the pull request ended with checks:$`, tc.pullRequestEndedWithChecks)
	ctx.Given(`^the workflow run ended with jobs:$`, tc.workflowRunEndedWithJobs)

	ctx.When(`^I read the PR status$`, tc.readThePRStatus)
	ctx.When(`^a watch closes on those checks$`, tc.aWatchClosesOnThoseChecks)
	ctx.When(`^a workflow watch reports that completed run$`, tc.aWorkflowWatchReportsThatCompletedRun)

	ctx.Then(`^the report still counts "([^"]*)"$`, tc.theReportContains)
	ctx.Then(`^the report names the failing check "([^"]*)"$`, tc.theReportNamesTheFailingCheck)
	ctx.Then(`^the report names "([^"]*)" as the failing step of "([^"]*)"$`, tc.theReportNamesTheFailingStep)
	ctx.Then(`^the report shows "([^"]*)"$`, tc.theReportContains)
	ctx.Then(`^the report does not name "([^"]*)"$`, tc.theReportDoesNotName)
	ctx.Then(`^the report lists no failing check$`, tc.theReportListsNoFailingCheck)
}

func (tc *TestContext) workflowRunEndedWithJobs(table *godog.Table) error {
	jobs := make([]remote.Job, 0, len(table.Rows)-1)
	for _, row := range table.Rows[1:] {
		jobs = append(jobs, remote.Job{
			Name:       row.Cells[0].Value,
			Status:     "completed",
			Conclusion: row.Cells[1].Value,
			FailedStep: row.Cells[2].Value,
		})
	}
	tc.Config["workflow_jobs"] = jobs
	return nil
}

func (tc *TestContext) aWorkflowWatchReportsThatCompletedRun() error {
	jobs, ok := tc.Config["workflow_jobs"].([]remote.Job)
	if !ok {
		return fmt.Errorf("no workflow jobs were staged")
	}
	tc.Output = actions.FormatFailedWorkflowJobs(jobs, "   ")
	return nil
}

// pullRequestEndedWithChecks stages the answer the provider gives for a pull
// request whose checks have all finished, and reduces it the way GetPRInfo
// does.
func (tc *TestContext) pullRequestEndedWithChecks(table *godog.Table) error {
	checks := &remote.PRChecks{}

	for _, row := range table.Rows[1:] {
		checks.Checks = append(checks.Checks, remote.CheckRun{
			Name:       row.Cells[0].Value,
			Status:     "completed",
			Conclusion: row.Cells[1].Value,
			FailedStep: row.Cells[2].Value,
			ErrorLog:   row.Cells[3].Value,
		})

		checks.TotalCount++
		switch row.Cells[1].Value {
		case "success", "skipped", "neutral":
			checks.Success++
		default:
			checks.Failure++
		}
	}

	checks.Status = "success"
	if checks.Failure > 0 {
		checks.Status = "failure"
	}

	tc.Config["pr_checks"] = branch.ChecksInfo(checks)
	return nil
}

func (tc *TestContext) stagedChecks() (*branch.PRChecksInfo, error) {
	checks, ok := tc.Config["pr_checks"].(*branch.PRChecksInfo)
	if !ok {
		return nil, fmt.Errorf("no checks were staged")
	}
	return checks, nil
}

// readThePRStatus renders the block `cidx pr status` prints, in full.
func (tc *TestContext) readThePRStatus() error {
	checks, err := tc.stagedChecks()
	if err != nil {
		return err
	}

	tc.Output = branch.FormatPRInfo(&branch.PRInfo{
		Number:     347,
		Title:      "the pull request under watch",
		Status:     branch.PRStatusOpen,
		BranchName: "fix/naming",
		BaseBranch: "main",
		Mergeable:  true,
		Checks:     checks,
	})
	return nil
}

// aWatchClosesOnThoseChecks renders what `cidx pr watch` appends to its verdict
// when the run ends red -- the same lines in both the animated and the quiet
// form.
func (tc *TestContext) aWatchClosesOnThoseChecks() error {
	checks, err := tc.stagedChecks()
	if err != nil {
		return err
	}

	tc.Output = branch.FormatFailedChecks(checks.Failed, "  ")
	return nil
}

func (tc *TestContext) theReportContains(text string) error {
	if !strings.Contains(tc.Output, text) {
		return fmt.Errorf("expected %q in the report:\n%s", text, tc.Output)
	}
	return nil
}

func (tc *TestContext) theReportNamesTheFailingCheck(name string) error {
	return tc.theReportContains("✗ " + name)
}

func (tc *TestContext) theReportNamesTheFailingStep(step, check string) error {
	return tc.theReportContains(fmt.Sprintf("✗ %s — failed step: %s", check, step))
}

func (tc *TestContext) theReportDoesNotName(name string) error {
	if strings.Contains(tc.Output, "✗ "+name) {
		return fmt.Errorf("%q is named as failing in:\n%s", name, tc.Output)
	}
	return nil
}

func (tc *TestContext) theReportListsNoFailingCheck() error {
	checks, err := tc.stagedChecks()
	if err != nil {
		return err
	}
	if len(checks.Failed) != 0 {
		return fmt.Errorf("expected no failing check, got %+v", checks.Failed)
	}
	if strings.Contains(tc.Output, "✗") {
		return fmt.Errorf("expected no failure marker in:\n%s", tc.Output)
	}
	return nil
}
