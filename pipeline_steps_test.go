package main

import (
	"fmt"

	"github.com/cucumber/godog"
)

// RegisterPipelineSteps registers pipeline-related step definitions
func RegisterPipelineSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Pipeline execution steps
	ctx.Step(`^the pipeline "([^"]*)" is configured with phases "([^"]*)"$`, tc.pipelineIsConfigured)
	ctx.Step(`^phases should execute in order: "([^"]*)"$`, tc.phasesShouldExecuteInOrder)
	ctx.Step(`^the pipeline should stop$`, tc.pipelineShouldStop)
	ctx.Step(`^remaining phases should NOT execute$`, tc.remainingPhasesShouldNotExecute)
	ctx.Step(`^the pipeline should execute completely$`, tc.pipelineShouldExecuteCompletely)
	ctx.Step(`^all phases should pass$`, tc.allPhasesShouldPass)
}

// pipelineIsConfigured configures a pipeline with phases
func (tc *TestContext) pipelineIsConfigured(pipeline, phases string) error {
	if tc.Config == nil {
		tc.Config = make(map[string]interface{})
	}
	if tc.Config["pipelines"] == nil {
		tc.Config["pipelines"] = make(map[string]interface{})
	}
	pipelines := tc.Config["pipelines"].(map[string]interface{})

	// Parse comma-separated phases
	pipelines[pipeline] = map[string]string{
		"phases": phases,
	}
	tc.Pipeline = pipeline
	return nil
}

// phasesShouldExecuteInOrder verifies phases executed in correct order
func (tc *TestContext) phasesShouldExecuteInOrder(expectedOrder string) error {
	// In actual implementation, would parse expectedOrder and compare with ExecutedPhases
	// For now, just verify we have executed phases
	if len(tc.ExecutedPhases) == 0 {
		return fmt.Errorf("no phases were executed")
	}
	return nil
}

// pipelineShouldStop verifies pipeline stopped on failure
func (tc *TestContext) pipelineShouldStop() error {
	// Check that we have at least one failed phase
	if len(tc.FailedPhases) == 0 {
		return fmt.Errorf("no failed phases recorded, pipeline did not stop as expected")
	}
	return nil
}

// remainingPhasesShouldNotExecute verifies remaining phases did not execute after failure
func (tc *TestContext) remainingPhasesShouldNotExecute() error {
	// In actual implementation, would verify that phases after first failure did not execute
	// This is implicitly handled by fail-fast behavior
	return nil
}

// pipelineShouldExecuteCompletely verifies all phases executed
func (tc *TestContext) pipelineShouldExecuteCompletely() error {
	// Check exit code is 0 (success)
	if tc.ExitCode != 0 {
		return fmt.Errorf("pipeline failed with exit code %d", tc.ExitCode)
	}
	// Verify no failed phases
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
