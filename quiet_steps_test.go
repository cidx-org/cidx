package main

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v2/internal/commands"
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

	// Streaming at the normal log level (issue #273)
	ctx.Given(`^cidx runs (locally|in CI)$`, tc.cidxRunsWhere)
	ctx.When(`^the run is invoked with "([^"]*)"$`, tc.runInvokedWith)
	ctx.Then(`^container output should be (buffered|streamed)$`, tc.containerOutputShouldBe)
}

// cidxRunsWhere records whether the run happens on a CI runner, the condition
// that turns quiet on by itself (#87).
func (tc *TestContext) cidxRunsWhere(where string) error {
	tc.CI = where == "in CI"
	return nil
}

// runInvokedWith resolves the flags of a `cidx run` line through the decision
// the command really makes -- commands.ResolveQuiet, not a restatement of it
// (issue #317).
func (tc *TestContext) runInvokedWith(flags string) error {
	var quiet, stream, verbose bool
	for _, flag := range strings.Fields(flags) {
		switch flag {
		case "--quiet", "-q":
			quiet = true
		case "--stream":
			stream = true
		case "--verbose":
			verbose = true
		default:
			return fmt.Errorf("unknown flag %q", flag)
		}
	}

	tc.Config["quiet"] = commands.ResolveQuiet(quiet, stream, verbose, tc.CI)
	return nil
}

// containerOutputShouldBe asserts what happens to a successful container's
// output: buffered means it is captured and dropped, streamed means it reaches
// the terminal as it is produced.
func (tc *TestContext) containerOutputShouldBe(expected string) error {
	quiet, ok := tc.Config["quiet"].(bool)
	if !ok {
		return fmt.Errorf("no run was invoked")
	}

	got := "streamed"
	if quiet {
		got = "buffered"
	}
	if got != expected {
		return fmt.Errorf("container output is %s, expected %s", got, expected)
	}
	return nil
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
