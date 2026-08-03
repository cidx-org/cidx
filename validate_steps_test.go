package main

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/validator"
	"github.com/cucumber/godog"
	"github.com/urfave/cli/v2"
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

// invocationCommandTree mirrors the shape of the real cidx tree: namespaces
// with no action of their own, leaf commands that take arguments, `vuln` living
// under `security`, and the flags `run` accepts before its phase. The real tree
// is covered where it is built, by TestValidate_RealWorkflowsResolve and
// TestCheckWorkflow_RealCIWorkflowKeepsItsPhases in cmd/cidx.
func invocationCommandTree() *cli.App {
	noop := func(*cli.Context) error { return nil }
	return &cli.App{
		Name: "cidx",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}},
			&cli.BoolFlag{Name: "verbose"},
			&cli.BoolFlag{Name: "version", Aliases: []string{"v"}},
		},
		Commands: []*cli.Command{
			{Name: "run", Action: noop, Flags: []cli.Flag{
				&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}},
				&cli.IntFlag{Name: "concurrency", Aliases: []string{"j"}},
			}},
			{Name: "validate", Action: noop},
			{Name: "check", Subcommands: []*cli.Command{
				{Name: "drift", Action: noop},
				{Name: "workflow", Action: noop},
			}},
			{Name: "security", Subcommands: []*cli.Command{
				{Name: "vuln", Subcommands: []*cli.Command{
					{Name: "list", Action: noop},
					{Name: "ignore", Action: noop},
				}},
			}},
			{Name: "preset", Subcommands: []*cli.Command{
				{Name: "scan-targets", Action: noop},
			}},
		},
	}
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

	app := invocationCommandTree()
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
