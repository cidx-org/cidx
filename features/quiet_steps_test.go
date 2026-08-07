package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
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

// haveToolWithExitCode stages a tool whose standard output the scenario does
// not spell out. It still has some -- a container that prints nothing cannot
// show whether quiet suppressed anything -- so it gets a line of its own.
func (tc *TestContext) haveToolWithExitCode(tool string, exitCode int) error {
	return tc.stageTool(tool, tool+": standard output", exitCode)
}

// haveToolWithOutputAndExitCode stages a tool whose standard output the
// scenario names, so the assertions can look for that exact text.
func (tc *TestContext) haveToolWithOutputAndExitCode(tool, output string, exitCode int) error {
	return tc.stageTool(tool, output, exitCode)
}

// haveMultipleToolsParallel stages the mixed run the parallel scenario is
// about: tools that pass and one that does not, so "only the failed logs" has
// something to be false about.
func (tc *TestContext) haveMultipleToolsParallel() error {
	if err := tc.stageTool("lint", "lint: standard output", 0); err != nil {
		return err
	}
	if err := tc.stageTool("unit", "unit: standard output", 0); err != nil {
		return err
	}
	return tc.stageTool("scan", "scan: standard output", 1)
}

func (tc *TestContext) stageTool(name, stdout string, exitCode int) error {
	tc.Tools = append(tc.Tools, stagedTool{Name: name, Stdout: stdout, ExitCode: exitCode})
	return nil
}

// runStagedTools plays the staged tools through the decision cidx really makes
// about their output: commands.ResolveQuiet says whether a container's stdout
// is buffered or streamed, and pkg/executor's Run flushes the buffer only when
// the container exits non-zero. The two lines it emits are the executor's own
// -- the ✓ it logs on success, and the "container exited with code N" its
// error carries (pkg/executor/docker.go).
//
// What is stood in for is the container: a real one would need a daemon and an
// image pull for a decision that needs neither.
func (tc *TestContext) runStagedTools(parts []string) error {
	var quiet, stream, verbose, parallel bool
	for _, part := range parts {
		switch part {
		case "--quiet", "-q":
			quiet = true
		case "--stream":
			stream = true
		case "--verbose":
			verbose = true
		case "--parallel":
			parallel = true
		}
	}

	buffered := commands.ResolveQuiet(quiet, stream, verbose, tc.CI)
	tc.Config["quiet"] = buffered

	for _, tool := range tc.Tools {
		if !buffered {
			tc.Output += tool.Stdout + "\n"
		}

		if tool.ExitCode != 0 {
			if buffered {
				tc.Output += tool.Stdout + "\n"
			}
			tc.Output += fmt.Sprintf("container exited with code %d\n", tool.ExitCode)
			tc.ExitCode = tool.ExitCode
			tc.FailedPhases = append(tc.FailedPhases, tool.Name)
			if !parallel {
				return nil // fail-fast, like a sequential phase
			}
			continue
		}

		tc.Output += fmt.Sprintf("  ✓ %s completed\n", tool.Name)
		tc.ExecutedPhases = append(tc.ExecutedPhases, tool.Name)
	}
	return nil
}

// executionShouldBeQuiet asserts the run resolved to buffering, through
// commands.ResolveQuiet rather than a restatement of its rules.
func (tc *TestContext) executionShouldBeQuiet() error {
	quiet, ok := tc.Config["quiet"].(bool)
	if !ok {
		return fmt.Errorf("no run was invoked")
	}
	if !quiet {
		return fmt.Errorf("the run resolved to streamed output, expected it to be quiet")
	}
	return nil
}
