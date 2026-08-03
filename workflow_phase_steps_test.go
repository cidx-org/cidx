package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/validator"
	"github.com/cucumber/godog"
)

// RegisterWorkflowPhaseSteps registers the steps for the phases `check
// workflow` reads out of a GitHub Actions workflow (issue #233).
//
// The steps stage a real workflow file and run the real extraction
// (validator.ParseWorkflow) and the real comparison
// (validator.ValidateWorkflow) over it: a scenario that stayed green while the
// extraction was broken would be worth nothing (issue #265).
func RegisterWorkflowPhaseSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a workflow job "([^"]*)" running "([^"]*)"$`, tc.workflowJobRunning)
	ctx.Given(`^a workflow job "([^"]*)" running:$`, tc.workflowJobRunningScript)

	ctx.When(`^I extract the phases of the workflow$`, tc.extractWorkflowPhases)
	ctx.When(`^I compare the pipeline with the workflow$`, tc.comparePipelineWithWorkflow)

	ctx.Then(`^the extracted phases should be "([^"]*)"$`, tc.extractedPhasesShouldBe)
	ctx.Then(`^no phase should be extracted$`, tc.noPhaseShouldBeExtracted)
	ctx.Then(`^the workflow should be in sync with the pipeline$`, tc.workflowShouldBeInSync)
	ctx.Then(`^phase "([^"]*)" should be reported as missing from the workflow$`, tc.phaseShouldBeMissingFromWorkflow)
}

// workflowJob is a job of the workflow a scenario describes: a name and the
// script its only step runs.
type workflowJob struct {
	name   string
	script string
}

func (tc *TestContext) workflowJobRunning(name, script string) error {
	jobs, _ := tc.Config["phase_jobs"].([]workflowJob)
	tc.Config["phase_jobs"] = append(jobs, workflowJob{name: name, script: script})
	return nil
}

func (tc *TestContext) workflowJobRunningScript(name string, script *godog.DocString) error {
	return tc.workflowJobRunning(name, script.Content)
}

// writeStagedPhaseWorkflow renders the jobs the scenario described as ci.yml,
// in declaration order — the order the phases are expected to come back in.
func (tc *TestContext) writeStagedPhaseWorkflow() (string, error) {
	dir, err := tc.scenarioDir()
	if err != nil {
		return "", err
	}

	jobs, _ := tc.Config["phase_jobs"].([]workflowJob)
	if len(jobs) == 0 {
		return "", fmt.Errorf("no workflow job was described by this scenario")
	}

	var b strings.Builder
	b.WriteString("name: CI\n\non:\n  push:\n    branches: [main]\n\njobs:\n")
	for _, job := range jobs {
		fmt.Fprintf(&b, "  %s:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n", job.name)
		for _, line := range strings.Split(job.script, "\n") {
			fmt.Fprintf(&b, "          %s\n", line)
		}
	}

	workflowDir := filepath.Join(dir, remote.GitHubWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", workflowDir, err)
	}
	path := filepath.Join(workflowDir, "ci.yml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	return path, nil
}

func (tc *TestContext) extractWorkflowPhases() error {
	path, err := tc.writeStagedPhaseWorkflow()
	if err != nil {
		return err
	}

	workflow, err := validator.ParseWorkflow(invocationCommandTree(), path)
	if err != nil {
		return err
	}

	tc.Config["extracted_phases"] = workflow.Phases
	tc.Output = strings.Join(workflow.Phases, ", ")
	return nil
}

func (tc *TestContext) comparePipelineWithWorkflow() error {
	cfg, err := tc.loadStagedConfig()
	if err != nil {
		return err
	}
	path, err := tc.writeStagedPhaseWorkflow()
	if err != nil {
		return err
	}

	result, err := validator.ValidateWorkflow(invocationCommandTree(), cfg, "ci", path)
	if err != nil {
		return err
	}

	tc.Config["workflow_result"] = result
	tc.Output = validator.FormatResult(result)
	if !result.Success {
		tc.ExitCode = 1
	}
	return nil
}

func (tc *TestContext) extractedPhases() ([]string, error) {
	phases, ok := tc.Config["extracted_phases"].([]string)
	if !ok {
		return nil, fmt.Errorf("no phase extraction ran in this scenario")
	}
	return phases, nil
}

func (tc *TestContext) extractedPhasesShouldBe(expected string) error {
	phases, err := tc.extractedPhases()
	if err != nil {
		return err
	}

	var want []string
	for _, phase := range strings.Split(expected, ",") {
		want = append(want, strings.TrimSpace(phase))
	}
	if strings.Join(phases, ", ") != strings.Join(want, ", ") {
		return fmt.Errorf("extracted %v, want %v", phases, want)
	}
	return nil
}

func (tc *TestContext) noPhaseShouldBeExtracted() error {
	phases, err := tc.extractedPhases()
	if err != nil {
		return err
	}
	if len(phases) > 0 {
		return fmt.Errorf("expected no phase, got %v", phases)
	}
	return nil
}

func (tc *TestContext) workflowValidation() (*validator.ValidationResult, error) {
	result, ok := tc.Config["workflow_result"].(*validator.ValidationResult)
	if !ok {
		return nil, fmt.Errorf("no comparison ran in this scenario")
	}
	return result, nil
}

func (tc *TestContext) workflowShouldBeInSync() error {
	result, err := tc.workflowValidation()
	if err != nil {
		return err
	}
	if !result.Success {
		return fmt.Errorf("expected the workflow to be in sync, got:\n%s", validator.FormatResult(result))
	}
	return nil
}

func (tc *TestContext) phaseShouldBeMissingFromWorkflow(phase string) error {
	result, err := tc.workflowValidation()
	if err != nil {
		return err
	}
	for _, missing := range result.MissingInGH {
		if missing == phase {
			return nil
		}
	}
	return fmt.Errorf("expected %q to be reported as missing, got %v", phase, result.MissingInGH)
}
