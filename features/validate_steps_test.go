package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cidx-org/cidx/v3/pkg/validator"
	"github.com/cucumber/godog"
)

// RegisterValidateSteps registers the steps for stale cidx invocations in CI
// workflows (issue #239).
func RegisterValidateSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a workflow step running "([^"]*)"$`, tc.workflowStepRunning)
	ctx.Given(`^a workflow step running:$`, tc.workflowStepRunningScript)

	ctx.When(`^I validate the workflow invocations$`, tc.validateWorkflowInvocations)

	ctx.Then(`^the invocation should be reported as stale$`, tc.invocationShouldBeStale)
	ctx.Then(`^no invocation should be reported$`, tc.noInvocationShouldBeReported)
	ctx.Then(`^the report should mention "([^"]*)"$`, tc.reportShouldMention)
}

func (tc *TestContext) workflowStepRunning(script string) error {
	tc.Config["workflow_script"] = script
	return nil
}

func (tc *TestContext) workflowStepRunningScript(script *godog.DocString) error {
	tc.Config["workflow_script"] = script.Content
	return nil
}

func (tc *TestContext) validateWorkflowInvocations() error {
	script, ok := tc.Config["workflow_script"].(string)
	if !ok {
		return fmt.Errorf("no workflow step was given")
	}

	// The tree cidx ships, not a copy of it. A copy had to be updated by hand
	// at every reorganisation, and a copy that is not updated leaves these
	// scenarios green about a CLI that no longer exists (issue #317).
	app := commands.NewApp()
	var report []string
	for _, inv := range validator.ExtractInvocations(script) {
		if reason := validator.Resolve(app, inv.Args); reason != "" {
			report = append(report, fmt.Sprintf("%s: %s", inv, reason))
		}
	}

	tc.Output = strings.Join(report, "\n")
	if len(report) > 0 {
		tc.ExitCode = 1
	}
	return nil
}

func (tc *TestContext) invocationShouldBeStale() error {
	if tc.Output == "" {
		return fmt.Errorf("expected a stale invocation to be reported, got nothing")
	}
	return nil
}

func (tc *TestContext) noInvocationShouldBeReported() error {
	if tc.Output != "" {
		return fmt.Errorf("expected no report, got:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) reportShouldMention(text string) error {
	if !strings.Contains(tc.Output, text) {
		return fmt.Errorf("expected the report to mention %q, got:\n%s", text, tc.Output)
	}
	return nil
}
