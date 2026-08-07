package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/validator"
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
	ctx.Given(`^workflow "([^"]*)" has a job "([^"]*)" running "([^"]*)"$`, tc.namedWorkflowJobRunning)
	ctx.Given(`^pipeline "([^"]*)" declares that no workflow implements it$`, tc.pipelineDeclaresNoWorkflow)
	ctx.Given(`^pipeline "([^"]*)" declares workflow "([^"]*)"$`, tc.pipelineDeclaresWorkflow)

	ctx.When(`^I extract the phases of the workflow$`, tc.extractWorkflowPhases)
	ctx.When(`^I compare the pipeline with the workflow$`, tc.comparePipelineWithWorkflow)
	ctx.When(`^I check every workflow$`, tc.checkEveryWorkflow)

	ctx.Then(`^the extracted phases should be "([^"]*)"$`, tc.extractedPhasesShouldBe)
	ctx.Then(`^no phase should be extracted$`, tc.noPhaseShouldBeExtracted)
	ctx.Then(`^the workflow should be in sync with the pipeline$`, tc.workflowShouldBeInSync)
	ctx.Then(`^phase "([^"]*)" should be reported as missing from the workflow$`, tc.phaseShouldBeMissingFromWorkflow)
	ctx.Then(`^pipeline "([^"]*)" should be in sync with its workflow$`, tc.pipelineShouldBeInSync)
	ctx.Then(`^pipeline "([^"]*)" should be reported out of sync$`, tc.pipelineShouldBeOutOfSync)
	ctx.Then(`^pipeline "([^"]*)" should not be compared with any workflow$`, tc.pipelineShouldNotBeCompared)
}

// workflowJob is a job of a workflow a scenario describes: the file it belongs
// to, a name, and the script its only step runs.
type workflowJob struct {
	file   string
	name   string
	script string
}

// defaultWorkflowFile is the workflow a scenario writes into when it does not
// name one. Most scenarios describe a single CI workflow.
const defaultWorkflowFile = "ci.yml"

func (tc *TestContext) workflowJobRunning(name, script string) error {
	return tc.namedWorkflowJobRunning(defaultWorkflowFile, name, script)
}

func (tc *TestContext) workflowJobRunningScript(name string, script *godog.DocString) error {
	return tc.workflowJobRunning(name, script.Content)
}

func (tc *TestContext) namedWorkflowJobRunning(file, name, script string) error {
	jobs, _ := tc.Config["phase_jobs"].([]workflowJob)
	tc.Config["phase_jobs"] = append(jobs, workflowJob{file: file, name: name, script: script})
	return nil
}

func (tc *TestContext) pipelineDeclaresWorkflow(pipeline, workflow string) error {
	workflows, ok := tc.Config["pipeline_workflows"].(map[string]string)
	if !ok {
		workflows = make(map[string]string)
		tc.Config["pipeline_workflows"] = workflows
	}
	workflows[pipeline] = workflow
	return nil
}

func (tc *TestContext) pipelineDeclaresNoWorkflow(pipeline string) error {
	return tc.pipelineDeclaresWorkflow(pipeline, config.NoWorkflow)
}

// writeStagedPhaseWorkflows renders the jobs the scenario described into the
// workflow files they belong to, each in declaration order — the order the
// phases are expected to come back in. It returns the workflow directory.
func (tc *TestContext) writeStagedPhaseWorkflows() (string, error) {
	dir, err := tc.scenarioDir()
	if err != nil {
		return "", err
	}

	jobs, _ := tc.Config["phase_jobs"].([]workflowJob)
	if len(jobs) == 0 {
		return "", fmt.Errorf("no workflow job was described by this scenario")
	}

	files := make(map[string]*strings.Builder)
	var order []string
	for _, job := range jobs {
		b, seen := files[job.file]
		if !seen {
			b = &strings.Builder{}
			b.WriteString("name: CI\n\non:\n  push:\n    branches: [main]\n\njobs:\n")
			files[job.file] = b
			order = append(order, job.file)
		}
		fmt.Fprintf(b, "  %s:\n    runs-on: ubuntu-latest\n    steps:\n      - run: |\n", job.name)
		for _, line := range strings.Split(job.script, "\n") {
			fmt.Fprintf(b, "          %s\n", line)
		}
	}

	workflowDir := filepath.Join(dir, remote.GitHubWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", workflowDir, err)
	}
	for _, file := range order {
		path := filepath.Join(workflowDir, file)
		if err := os.WriteFile(path, []byte(files[file].String()), 0o644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	return workflowDir, nil
}

// writeStagedPhaseWorkflow stages the described workflows and returns the path
// of the default one, which the single-workflow scenarios talk about.
func (tc *TestContext) writeStagedPhaseWorkflow() (string, error) {
	workflowDir, err := tc.writeStagedPhaseWorkflows()
	if err != nil {
		return "", err
	}
	return filepath.Join(workflowDir, defaultWorkflowFile), nil
}

func (tc *TestContext) extractWorkflowPhases() error {
	path, err := tc.writeStagedPhaseWorkflow()
	if err != nil {
		return err
	}

	workflow, err := validator.ParseWorkflow(commands.NewApp(), path)
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

	result, err := validator.ValidateWorkflow(commands.NewApp(), cfg, "ci", path)
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

// checkEveryWorkflow is `cidx check workflow` without an argument: the real
// pairing of every declared pipeline with the workflow that implements it.
func (tc *TestContext) checkEveryWorkflow() error {
	cfg, err := tc.loadStagedConfig()
	if err != nil {
		return err
	}
	workflowDir, err := tc.writeStagedPhaseWorkflows()
	if err != nil {
		return err
	}

	results, err := validator.ValidateAllWorkflows(commands.NewApp(), cfg, workflowDir)
	if err != nil {
		return err
	}

	tc.Config["workflow_results"] = results
	if len(results) == 1 {
		// A scenario with a single pipeline can talk about "the comparison",
		// the way the single-workflow steps do.
		tc.Config["workflow_result"] = results[0]
	}
	var output strings.Builder
	for _, result := range results {
		output.WriteString(validator.FormatResult(result))
		if !result.Success {
			tc.ExitCode = 1
		}
	}
	tc.Output = output.String()
	return nil
}

// comparedPipeline returns the result reported for a pipeline, or nil when no
// workflow was compared with it.
func (tc *TestContext) comparedPipeline(pipeline string) (*validator.ValidationResult, error) {
	results, ok := tc.Config["workflow_results"].([]*validator.ValidationResult)
	if !ok {
		return nil, fmt.Errorf("no workflow check ran in this scenario")
	}
	for _, result := range results {
		if result.Pipeline == pipeline {
			return result, nil
		}
	}
	return nil, nil
}

func (tc *TestContext) pipelineShouldBeInSync(pipeline string) error {
	result, err := tc.comparedPipeline(pipeline)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("pipeline %q was not compared with any workflow", pipeline)
	}
	if !result.Success {
		return fmt.Errorf("expected %q to be in sync, got:\n%s", pipeline, validator.FormatResult(result))
	}
	return nil
}

func (tc *TestContext) pipelineShouldBeOutOfSync(pipeline string) error {
	result, err := tc.comparedPipeline(pipeline)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("pipeline %q was not compared with any workflow", pipeline)
	}
	if result.Success {
		return fmt.Errorf("expected %q to be reported out of sync, got:\n%s", pipeline, validator.FormatResult(result))
	}
	return nil
}

func (tc *TestContext) pipelineShouldNotBeCompared(pipeline string) error {
	result, err := tc.comparedPipeline(pipeline)
	if err != nil {
		return err
	}
	if result != nil {
		return fmt.Errorf("expected %q to be left alone, it was compared with %s", pipeline, result.WorkflowFile)
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
