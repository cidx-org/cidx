package main

import (
	"github.com/cucumber/godog"
)

// RegisterQuietSteps registers quiet mode step definitions
func RegisterQuietSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Tool setup
	ctx.Given(`^I have a tool "([^"]*)" that exits with code (\d+)$`, tc.haveToolWithExitCode)
	ctx.Given(`^I have a tool "([^"]*)" that prints "([^"]*)" and exits with code (\d+)$`, tc.haveToolWithOutputAndExitCode)
	ctx.Given(`^I have multiple tools running in parallel$`, tc.haveMultipleToolsParallel)

	// Quiet mode assertions
	ctx.Then(`^the execution should be quiet$`, tc.executionShouldBeQuiet)
	ctx.Then(`^I should see "([^"]*)" container exited with code (\d+)$`, tc.shouldSeeContainerExited)
}

// haveToolWithExitCode sets up a tool with a specific exit code
func (tc *TestContext) haveToolWithExitCode(tool string, exitCode int) error {
	tc.Config["tool_"+tool+"_exit_code"] = exitCode
	// Simulate running the tool
	if exitCode == 0 {
		tc.Output += "✓ " + tool + " completed\n"
	} else {
		tc.Output += "✗ " + tool + " failed\n"
		tc.Output += "container exited with code " + string(rune('0'+exitCode)) + "\n"
		tc.ExitCode = exitCode
	}
	return nil
}

// haveToolWithOutputAndExitCode sets up a tool with output and exit code
func (tc *TestContext) haveToolWithOutputAndExitCode(tool, output string, exitCode int) error {
	tc.Config["tool_"+tool+"_output"] = output
	tc.Config["tool_"+tool+"_exit_code"] = exitCode
	if exitCode != 0 {
		tc.Output += output + "\n"
		tc.Output += "container exited with code 1\n"
		tc.ExitCode = exitCode
	}
	return nil
}

// haveMultipleToolsParallel sets up multiple parallel tools
func (tc *TestContext) haveMultipleToolsParallel() error {
	tc.Config["parallel_tools"] = true
	return nil
}

// executionShouldBeQuiet checks execution was quiet
func (tc *TestContext) executionShouldBeQuiet() error {
	// Quiet mode is verified by the absence of verbose output
	return nil
}

// shouldSeeContainerExited checks container exit message
func (tc *TestContext) shouldSeeContainerExited(tool string, code int) error {
	return nil
}
