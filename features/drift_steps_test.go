package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/drift"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cucumber/godog"
)

// RegisterDriftSteps registers step definitions for drift detection scenarios.
//
// The steps stage a real cidx.toml and a real workflow file, then run the
// comparison `cidx check drift` runs (drift.Compare) over them: a scenario that
// stays green while pkg/drift is broken would be worth nothing (issue #265).
func RegisterDriftSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the GitHub Actions workflow has jobs "([^"]*)"$`, tc.ciWorkflowHasJobs)
	ctx.Given(`^the GitHub Actions workflow triggers on "([^"]*)"$`, tc.ciWorkflowTriggersOn)
	ctx.Given(`^the GitHub Actions workflow does NOT trigger on "([^"]*)"$`, tc.ciWorkflowDoesNotTriggerOn)
	ctx.Given(`^cidx\.toml and CI workflow are in sync$`, tc.cidxAndCIInSync)
	ctx.Given(`^cidx\.toml and CI workflow have differences$`, tc.cidxAndCIHaveDifferences)

	ctx.Then(`^I should see a phases table$`, tc.shouldSeePhasesTable)
	ctx.Then(`^all phases should show "([^"]*)"$`, tc.allPhasesShouldShow)
	ctx.Then(`^phase "([^"]*)" should show "([^"]*)"$`, tc.phaseShouldShow)
	ctx.Then(`^job "([^"]*)" should show "([^"]*)"$`, tc.jobShouldShow)
	ctx.Then(`^trigger "([^"]*)" should show "([^"]*)"$`, tc.triggerShouldShow)
	ctx.Then(`^I should see the number of differences$`, tc.shouldSeeNumberOfDifferences)
}

func (tc *TestContext) ciWorkflowHasJobs(jobsStr string) error {
	var jobs []string
	for _, job := range strings.Split(jobsStr, ",") {
		jobs = append(jobs, strings.TrimSpace(job))
	}
	tc.Config["ci_jobs"] = jobs
	// A workflow that runs jobs runs them on something: push, unless the
	// scenario states otherwise — the trigger `cidx generate` writes for the
	// ci pipeline.
	if tc.Config["ci_triggers"] == nil {
		tc.Config["ci_triggers"] = []string{"push"}
	}
	return nil
}

func (tc *TestContext) ciWorkflowTriggersOn(event string) error {
	triggers, _ := tc.Config["ci_triggers"].([]string)
	tc.Config["ci_triggers"] = append(triggers, event)
	return nil
}

func (tc *TestContext) ciWorkflowDoesNotTriggerOn(event string) error {
	tc.Config["ci_no_trigger_"+event] = true
	return nil
}

func (tc *TestContext) cidxAndCIInSync() error {
	tc.Config["pipelines"] = map[string][]string{
		"ci": {"security", "code", "test"},
	}
	tc.Config["ci_jobs"] = []string{"security", "code", "test"}
	tc.Config["ci_triggers"] = []string{"push"}
	return nil
}

func (tc *TestContext) cidxAndCIHaveDifferences() error {
	tc.Config["pipelines"] = map[string][]string{
		"ci": {"security", "code", "test", "build"},
	}
	tc.Config["ci_jobs"] = []string{"security", "code", "test"}
	tc.Config["ci_triggers"] = []string{"push"}
	return nil
}

// runDrift compares the staged cidx.toml with the staged workflow exactly as
// checkDriftAction does: the same workflow resolution (#170), the same
// comparison, the same rendering.
func (tc *TestContext) runDrift() error {
	cfg, err := tc.loadStagedConfig()
	if err != nil {
		return err
	}
	if err := tc.writeStagedWorkflow(); err != nil {
		return err
	}

	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}
	workflowFile, err := remote.ResolveWorkflowFile(filepath.Join(dir, remote.GitHubWorkflowDir))
	if err != nil {
		return err
	}

	result, err := drift.Compare(cfg, workflowFile)
	if err != nil {
		return err
	}

	tc.Config["drift_result"] = result
	tc.Output = drift.Format(result) + "\n" + drift.Summary(result) + "\n"
	if result.HasDrift() {
		tc.ExitCode = 1
	}
	return nil
}

// writeStagedWorkflow renders the GitHub Actions workflow the scenario
// described — one job per stated job name, next to the bootstrap job every
// generated workflow carries — as ci.yml, the name that maps this workflow to
// the [pipelines.ci] it implements.
func (tc *TestContext) writeStagedWorkflow() error {
	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString("name: CIDX CI\n\non:\n")
	triggers := tc.stagedWorkflowTriggers()
	for _, trigger := range triggers {
		switch trigger {
		case "push":
			b.WriteString("  push:\n    branches: [main]\n")
		case "pull_request":
			b.WriteString("  pull_request:\n    branches: [main]\n")
		default:
			fmt.Fprintf(&b, "  %s:\n", trigger)
		}
	}
	if len(triggers) == 0 {
		// A workflow nothing triggers automatically is still a workflow.
		b.WriteString("  workflow_dispatch:\n")
	}

	b.WriteString("\njobs:\n")
	b.WriteString("  bootstrap:\n    name: Bootstrap\n    runs-on: ubuntu-latest\n")
	jobs, _ := tc.Config["ci_jobs"].([]string)
	for _, job := range jobs {
		fmt.Fprintf(&b, "  %s:\n    name: %s\n    runs-on: ubuntu-latest\n    needs: [bootstrap]\n", job, job)
	}

	workflowDir := filepath.Join(dir, remote.GitHubWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", workflowDir, err)
	}
	path := filepath.Join(workflowDir, "ci.yml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// stagedWorkflowTriggers is what the scenario left the workflow triggering on.
func (tc *TestContext) stagedWorkflowTriggers() []string {
	stated, _ := tc.Config["ci_triggers"].([]string)
	var triggers []string
	for _, trigger := range stated {
		if tc.Config["ci_no_trigger_"+trigger] == true {
			continue
		}
		triggers = append(triggers, trigger)
	}
	return triggers
}

func (tc *TestContext) driftResult() (*drift.Result, error) {
	result, ok := tc.Config["drift_result"].(*drift.Result)
	if !ok {
		return nil, fmt.Errorf("no drift comparison ran in this scenario")
	}
	return result, nil
}

func (tc *TestContext) shouldSeePhasesTable() error {
	result, err := tc.driftResult()
	if err != nil {
		return err
	}
	if len(result.Phases) == 0 {
		return fmt.Errorf("the comparison reported no phase at all")
	}
	if !strings.Contains(tc.Output, "Phases:") {
		return fmt.Errorf("expected a phases table in the output:\n%s", tc.Output)
	}
	for _, phase := range result.Phases {
		if !strings.Contains(tc.Output, phase.Name) {
			return fmt.Errorf("phase %q is missing from the table:\n%s", phase.Name, tc.Output)
		}
	}
	return nil
}

func (tc *TestContext) allPhasesShouldShow(status string) error {
	result, err := tc.driftResult()
	if err != nil {
		return err
	}
	if len(result.Phases) == 0 {
		return fmt.Errorf("expected every phase to show %q, but no phase was compared", status)
	}
	for _, phase := range result.Phases {
		if string(phase.Status) != status {
			return fmt.Errorf("phase %q shows %q, want %q", phase.Name, phase.Status, status)
		}
	}
	return nil
}

// phaseShouldShow asserts on the comparison itself, then on the table the user
// reads — a status the user cannot see is not reported.
func (tc *TestContext) phaseShouldShow(phase, status string) error {
	result, err := tc.driftResult()
	if err != nil {
		return err
	}
	for _, diff := range result.Phases {
		if diff.Name != phase {
			continue
		}
		if string(diff.Status) != status {
			return fmt.Errorf("phase %q shows %q, want %q", phase, diff.Status, status)
		}
		if !strings.Contains(tc.Output, phase) || !strings.Contains(tc.Output, status) {
			return fmt.Errorf("phase %q with status %q is missing from the table:\n%s", phase, status, tc.Output)
		}
		return nil
	}
	return fmt.Errorf("no phase %q in the comparison (compared: %s)", phase, phaseNames(result))
}

// jobShouldShow reads the same table from the CI side: a workflow job with no
// phase behind it lands in the phases comparison as extra.
func (tc *TestContext) jobShouldShow(job, status string) error {
	return tc.phaseShouldShow(job, status)
}

func (tc *TestContext) triggerShouldShow(trigger, status string) error {
	result, err := tc.driftResult()
	if err != nil {
		return err
	}
	for _, diff := range result.Triggers {
		if diff.Event != trigger {
			continue
		}
		if string(diff.Status) != status {
			return fmt.Errorf("trigger %q shows %q, want %q", trigger, diff.Status, status)
		}
		if !strings.Contains(tc.Output, trigger) || !strings.Contains(tc.Output, status) {
			return fmt.Errorf("trigger %q with status %q is missing from the table:\n%s", trigger, status, tc.Output)
		}
		return nil
	}
	return fmt.Errorf("no trigger %q in the comparison (compared: %s)", trigger, triggerEvents(result))
}

func (tc *TestContext) shouldSeeNumberOfDifferences() error {
	result, err := tc.driftResult()
	if err != nil {
		return err
	}
	if result.DiffCount() == 0 {
		return fmt.Errorf("expected differences to be counted, the comparison found none")
	}
	expected := fmt.Sprintf("%d difference(s) found", result.DiffCount())
	if !strings.Contains(tc.Output, expected) {
		return fmt.Errorf("expected %q in the output:\n%s", expected, tc.Output)
	}
	return nil
}

func phaseNames(result *drift.Result) string {
	names := make([]string, 0, len(result.Phases))
	for _, phase := range result.Phases {
		names = append(names, phase.Name)
	}
	return strings.Join(names, ", ")
}

func triggerEvents(result *drift.Result) string {
	events := make([]string, 0, len(result.Triggers))
	for _, trigger := range result.Triggers {
		events = append(events, trigger.Event)
	}
	return strings.Join(events, ", ")
}
