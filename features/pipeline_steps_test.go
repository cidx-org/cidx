package features

import (
	"fmt"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cucumber/godog"
)

// RegisterPipelineSteps registers pipeline-related step definitions
func RegisterPipelineSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Pipeline configuration steps
	ctx.Step(`^the pipeline "([^"]*)" is configured with phases "([^"]*)"$`, tc.pipelineIsConfigured)
	ctx.Given(`^I have a (\w+) pipeline configured:$`, tc.havePipelineConfiguredDocstring)
	ctx.Given(`^I have a pipeline with multiple phases$`, tc.havePipelineWithMultiplePhases)
	ctx.Given(`^the "([^"]*)" phase will fail$`, tc.phaseWillFail)
	ctx.Given(`^all phases will pass$`, tc.allPhasesWillPass)
	ctx.Given(`^I have a pipeline: (.+)$`, tc.havePipelineFromDescription)
	ctx.When(`^I run pipeline "([^"]*)"$`, tc.runPipelineByName)

	// Pipeline execution steps
	ctx.When(`^I run the pipeline$`, tc.runThePipeline)

	// Pipeline assertion steps
	ctx.Step(`^phases should execute in order: "([^"]*)"$`, tc.phasesShouldExecuteInOrder)
	ctx.Step(`^phases should execute in this exact order:$`, tc.phasesShouldExecuteInExactOrder)
	ctx.Step(`^phases should execute in order:$`, tc.phasesShouldExecuteInOrderTable)
	ctx.Step(`^the pipeline should stop$`, tc.pipelineShouldStop)
	ctx.Step(`^the pipeline should execute completely$`, tc.pipelineShouldExecuteCompletely)
	ctx.Step(`^all phases should pass$`, tc.allPhasesShouldPass)
	ctx.Step(`^each phase should complete before the next starts$`, tc.eachPhaseShouldCompleteBeforeNext)
	ctx.Step(`^subsequent phases should NOT execute$`, tc.subsequentPhasesShouldNotExecute)
	ctx.Step(`^all three phases should execute$`, tc.allThreePhasesShouldExecute)
	ctx.Step(`^it should execute phases: (.+)$`, tc.shouldExecutePhasesList)

	// Pipeline inspection
	ctx.Then(`^I should see the release pipeline configuration$`, tc.shouldSeeReleasePipelineConfig)
	ctx.Then(`^I should see which phases it includes$`, tc.shouldSeeWhichPhases)
	ctx.Then(`^I should see the execution order$`, tc.shouldSeeExecutionOrder)
	ctx.Then(`^I should see completion messages for successful tools$`, tc.shouldSeeCompletionMessages)
}

// pipelineIsConfigured configures a pipeline with phases
func (tc *TestContext) pipelineIsConfigured(pipeline, phases string) error {
	tc.Config["phases"] = splitPhases(phases)
	tc.Config["runner_pipeline"] = true
	tc.Pipeline = pipeline
	return nil
}

// havePipelineConfiguredDocstring decodes the pipeline the scenario declares.
func (tc *TestContext) havePipelineConfiguredDocstring(name string, doc *godog.DocString) error {
	var declared struct {
		Pipelines map[string]config.Pipeline `toml:"pipelines"`
	}
	if err := toml.Unmarshal([]byte(doc.Content), &declared); err != nil {
		return fmt.Errorf("decode scenario pipeline: %w", err)
	}
	p, ok := declared.Pipelines[name]
	if !ok || len(p.Phases) == 0 {
		return fmt.Errorf("scenario declares no pipeline %q with phases", name)
	}
	tc.Pipeline = name
	tc.Config["phases"] = p.Phases
	tc.Config["runner_pipeline"] = true
	return nil
}

// havePipelineWithMultiplePhases configures a pipeline with multiple phases
func (tc *TestContext) havePipelineWithMultiplePhases() error {
	tc.Config["runner_pipeline"] = true
	tc.Pipeline = "test"
	tc.Config["pipeline"] = "test"
	tc.Config["phases"] = []string{"security", "code", "test", "build"}
	return nil
}

// phaseWillFail marks a phase as failing
func (tc *TestContext) phaseWillFail(phase string) error {
	tc.Config[fmt.Sprintf("phase_%s_fails", phase)] = true
	return nil
}

// allPhasesWillPass marks all phases as passing
func (tc *TestContext) allPhasesWillPass() error {
	// Default behavior - nothing to do
	return nil
}

// havePipelineFromDescription parses pipeline from description like "security → code → test"
func (tc *TestContext) havePipelineFromDescription(desc string) error {
	phases := strings.Split(desc, "→")
	cleanPhases := []string{}
	for _, p := range phases {
		cleanPhases = append(cleanPhases, strings.TrimSpace(p))
	}
	tc.Config["phases"] = cleanPhases
	tc.Config["runner_pipeline"] = true
	tc.Pipeline = "custom"
	return nil
}

// runPipelineByName runs a named pipeline
func (tc *TestContext) runPipelineByName(pipeline string) error {
	tc.Pipeline = pipeline
	return tc.runCommand("cidx run " + pipeline)
}

// runThePipeline runs the currently configured pipeline
func (tc *TestContext) runThePipeline() error {
	pipeline := tc.Pipeline
	if pipeline == "" {
		pipeline = "ci"
	}

	// If custom phases were configured, use them directly
	if phases, ok := tc.Config["phases"].([]string); ok {
		return tc.runCustomPipeline(phases)
	}

	return tc.runCommand("cidx run " + pipeline)
}

// runCustomPipeline exercises the real orchestration with recorded containers.
func (tc *TestContext) runCustomPipeline(phases []string) error {
	return tc.runDeclaredPipeline(tc.Pipeline, phases, false)
}

func (tc *TestContext) assertPhaseOrder(expected []string) error {
	if !slices.Equal(tc.ExecutedPhases, expected) {
		return fmt.Errorf("executed phases %v, expected exactly %v", tc.ExecutedPhases, expected)
	}
	return nil
}

func (tc *TestContext) phasesShouldExecuteInOrder(expectedOrder string) error {
	return tc.assertPhaseOrder(splitPhases(expectedOrder))
}

func (tc *TestContext) phasesShouldExecuteInExactOrder(table *godog.Table) error {
	var expected []string
	for _, row := range table.Rows[1:] {
		if len(row.Cells) != 2 {
			return fmt.Errorf("expected order and phase columns")
		}
		expected = append(expected, row.Cells[1].Value)
	}
	return tc.assertPhaseOrder(expected)
}

func (tc *TestContext) phasesShouldExecuteInOrderTable(table *godog.Table) error {
	var expected []string
	for _, row := range table.Rows {
		if len(row.Cells) != 1 {
			return fmt.Errorf("expected one phase per row")
		}
		expected = append(expected, row.Cells[0].Value)
	}
	return tc.assertPhaseOrder(expected)
}

// pipelineShouldStop verifies pipeline stopped on failure
func (tc *TestContext) pipelineShouldStop() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("pipeline succeeded instead of stopping")
	}
	return tc.subsequentPhasesShouldNotExecute()
}

// pipelineShouldExecuteCompletely verifies all phases executed
func (tc *TestContext) pipelineShouldExecuteCompletely() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("pipeline failed with exit code %d", tc.ExitCode)
	}
	if len(tc.FailedPhases) > 0 {
		return fmt.Errorf("pipeline had %d failed phases: %v", len(tc.FailedPhases), tc.FailedPhases)
	}
	return nil
}

// allPhasesShouldPass verifies all phases passed
func (tc *TestContext) allPhasesShouldPass() error {
	if len(tc.FailedPhases) > 0 {
		return fmt.Errorf("expected all phases to pass, but %d failed: %v", len(tc.FailedPhases), tc.FailedPhases)
	}
	if tc.ExitCode != 0 {
		return fmt.Errorf("expected exit code 0, got %d", tc.ExitCode)
	}
	return nil
}

// eachPhaseShouldCompleteBeforeNext asserts the run is sequential: every phase
// the pipeline declares was executed, once, in the declared order — which is
// what "completes before the next starts" means for a runner that appends a
// phase only after it returns.
func (tc *TestContext) eachPhaseShouldCompleteBeforeNext() error {
	declared, ok := tc.Config["phases"].([]string)
	if !ok || len(declared) == 0 {
		return fmt.Errorf("the scenario declared no phases to order")
	}
	if len(tc.ExecutedPhases) != len(declared) {
		return fmt.Errorf("declared phases %v, executed %v", declared, tc.ExecutedPhases)
	}
	for i, phase := range declared {
		if tc.ExecutedPhases[i] != phase {
			return fmt.Errorf("phase %d is %q, expected %q (executed: %v)", i+1, tc.ExecutedPhases[i], phase, tc.ExecutedPhases)
		}
	}
	return nil
}

// subsequentPhasesShouldNotExecute asserts fail-fast: the failed phase is the
// last one that ran.
func (tc *TestContext) subsequentPhasesShouldNotExecute() error {
	if len(tc.FailedPhases) == 0 {
		return fmt.Errorf("no phase failed, so nothing was supposed to stop")
	}
	failed := tc.FailedPhases[0]
	for i, phase := range tc.ExecutedPhases {
		if phase != failed {
			continue
		}
		if rest := tc.ExecutedPhases[i+1:]; len(rest) > 0 {
			return fmt.Errorf("%v executed after %q failed", rest, failed)
		}
		return nil
	}
	return fmt.Errorf("phase %q failed but was never executed (executed: %v)", failed, tc.ExecutedPhases)
}

// allThreePhasesShouldExecute checks three phases executed
func (tc *TestContext) allThreePhasesShouldExecute() error {
	if len(tc.ExecutedPhases) != 3 {
		return fmt.Errorf("expected exactly 3 phases, got %d: %v", len(tc.ExecutedPhases), tc.ExecutedPhases)
	}
	return nil
}

// shouldExecutePhasesList checks phases from comma-separated list
func (tc *TestContext) shouldExecutePhasesList(phaseList string) error {
	return tc.assertPhaseOrder(splitPhases(phaseList))
}

// declaredPhases is the phase list the scenario wrote into its pipeline.
func (tc *TestContext) declaredPhases() ([]string, error) {
	phases, ok := tc.Config["phases"].([]string)
	if !ok || len(phases) == 0 {
		return nil, fmt.Errorf("the scenario declared no phases")
	}
	return phases, nil
}

// shouldSeeReleasePipelineConfig asserts the dry-run reported the release
// pipeline the scenario configured, and not some other one.
func (tc *TestContext) shouldSeeReleasePipelineConfig() error {
	if tc.Pipeline != "release" {
		return fmt.Errorf("the run was of pipeline %q, not release", tc.Pipeline)
	}
	return tc.shouldSeeWhichPhases()
}

// shouldSeeWhichPhases asserts every phase the pipeline declares was named by
// the dry-run.
func (tc *TestContext) shouldSeeWhichPhases() error {
	phases, err := tc.declaredPhases()
	if err != nil {
		return err
	}
	for _, phase := range phases {
		if !strings.Contains(tc.Output, "PHASE: "+strings.ToUpper(phase)) {
			return fmt.Errorf("the dry-run does not name the %q phase:\n%s", phase, tc.Output)
		}
	}
	return nil
}

// shouldSeeExecutionOrder asserts the phases were named in the order the
// pipeline declares them, which is the order they would run in.
func (tc *TestContext) shouldSeeExecutionOrder() error {
	phases, err := tc.declaredPhases()
	if err != nil {
		return err
	}
	cursor := 0
	for _, phase := range phases {
		at := strings.Index(tc.Output[cursor:], "PHASE: "+strings.ToUpper(phase))
		if at < 0 {
			return fmt.Errorf("the %q phase is missing or out of order in:\n%s", phase, tc.Output)
		}
		cursor += at + len("PHASE: "+phase)
	}
	return nil
}

// shouldSeeCompletionMessages asserts every tool that passed reported it.
func (tc *TestContext) shouldSeeCompletionMessages() error {
	if len(tc.Tools) == 0 {
		return fmt.Errorf("the scenario staged no tool")
	}
	for _, tool := range tc.Tools {
		if tool.ExitCode != 0 {
			continue
		}
		if line := fmt.Sprintf("✓ %s completed", tool.Name); !strings.Contains(tc.Output, line) {
			return fmt.Errorf("no completion message for %q:\n%s", tool.Name, tc.Output)
		}
	}
	return nil
}
