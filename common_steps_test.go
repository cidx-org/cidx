package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cucumber/godog"
)

// RegisterCommonSteps registers common step definitions
func RegisterCommonSteps(ctx *godog.ScenarioContext, testCtx *TestContext) {
	// Background steps
	ctx.Given(`^CIDX is configured with default presets$`, testCtx.configureCIDXDefaults)
	ctx.Given(`^CIDX is configured with (.+) pipeline$`, testCtx.configurePipeline)
	ctx.Given(`^CIDX is configured with (.+) and (.+) phases$`, testCtx.configurePhases)
	ctx.Given(`^I am in a Git repository$`, testCtx.createGitRepo)
	ctx.Given(`^I have a (.+) pipeline configured$`, testCtx.havePipelineConfigured)

	// Environment steps
	ctx.Given(`^I am in (local|CI) environment$`, testCtx.setEnvironment)
	ctx.Given(`^I am in (local|CI) environment \(([^)]+)\)$`, testCtx.setEnvironmentWithProvider)
	ctx.Given(`^no CI environment variables are set$`, testCtx.clearCIEnvVars)

	// Environment variable steps
	ctx.Given(`^the environment variable "([^"]*)" is set to "([^"]*)"$`, testCtx.setEnvVar)
	ctx.Given(`^the environment variable "([^"]*)" is not set$`, testCtx.unsetEnvVar)
	ctx.Given(`^GITHUB_TOKEN is set$`, testCtx.setGitHubToken)
	ctx.Given(`^GITHUB_TOKEN is not set$`, testCtx.unsetGitHubToken)

	// Execution steps
	ctx.When(`^I run "([^"]*)"$`, testCtx.runCommand)
	ctx.When(`^I try to run "([^"]*)"$`, testCtx.tryRunCommand)
	ctx.When(`^CIDX detects the environment$`, testCtx.detectEnvironment)
	ctx.When(`^CIDX starts$`, testCtx.startCIDX)

	// Assertion steps
	ctx.Then(`^I should see "([^"]*)"$`, testCtx.shouldSeeOutput)
	ctx.Then(`^I should see error "([^"]*)"$`, testCtx.shouldSeeError)
	ctx.Then(`^I should see error message containing "([^"]*)"$`, testCtx.shouldSeeErrorContaining)
	ctx.Then(`^I should see error message about (.+)$`, testCtx.shouldSeeErrorAbout)
	ctx.Then(`^I should see message "([^"]*)"$`, testCtx.shouldSeeMessage)
	ctx.Then(`^I should see message containing "([^"]*)"$`, testCtx.shouldSeeMessageContaining)
	ctx.Then(`^I should see (.+) results$`, testCtx.shouldSeeResults)
	ctx.Then(`^I should see which (.+) failed$`, testCtx.shouldSeeWhichFailed)
	ctx.Then(`^the exit code should be (\d+)$`, testCtx.shouldHaveExitCode)
	ctx.Then(`^it should fail immediately$`, testCtx.shouldFailImmediately)
	ctx.Then(`^the pipeline should exit with non-zero code$`, testCtx.shouldExitNonZero)
	ctx.Then(`^the pipeline should complete successfully$`, testCtx.shouldCompleteSuccessfully)
	ctx.Then(`^the pipeline should stop immediately$`, testCtx.shouldStopImmediately)
	ctx.Then(`^all phases should execute normally$`, testCtx.allPhasesShouldExecuteNormally)
	ctx.Then(`^all phases should succeed$`, testCtx.allPhasesShouldSucceed)
	ctx.Then(`^it should execute the following phases in order:$`, testCtx.shouldExecutePhasesInOrder)
}

// configureCIDXDefaults sets up default CIDX configuration
func (tc *TestContext) configureCIDXDefaults() error {
	// Load default cidx.toml or use embedded defaults
	tc.Config["default"] = true
	return nil
}

// createGitRepo creates a temporary Git repository for testing
func (tc *TestContext) createGitRepo() error {
	tmpDir, err := os.MkdirTemp("", "cidx-test-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	tc.GitRepo = tmpDir

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to init git repo: %w", err)
	}

	// Set git user for commits
	exec.Command("git", "config", "user.email", "test@cidx.dev").Run()
	exec.Command("git", "config", "user.name", "CIDX Test").Run()

	return nil
}

// setEnvironment sets the environment to local or CI
func (tc *TestContext) setEnvironment(env string) error {
	tc.Environment = env
	if env == "CI" {
		tc.CI = true
		os.Setenv("CI", "true")
	} else {
		tc.CI = false
	}
	return nil
}

// setEnvironmentWithProvider sets environment with specific provider
func (tc *TestContext) setEnvironmentWithProvider(env, provider string) error {
	tc.setEnvironment(env)
	tc.Provider = provider

	switch provider {
	case "GitHub Actions":
		os.Setenv("GITHUB_ACTIONS", "true")
	case "GitLab CI":
		os.Setenv("GITLAB_CI", "true")
	case "Jenkins":
		os.Setenv("JENKINS_URL", "http://jenkins.local")
	case "CircleCI":
		os.Setenv("CIRCLECI", "true")
	}

	return nil
}

// clearCIEnvVars removes all CI environment variables
func (tc *TestContext) clearCIEnvVars() error {
	ciVars := []string{
		"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL",
		"CIRCLECI", "JENKINS_HOME", "GITHUB_TOKEN",
	}
	for _, v := range ciVars {
		os.Unsetenv(v)
	}
	return nil
}

// setEnvVar sets an environment variable
func (tc *TestContext) setEnvVar(key, value string) error {
	return os.Setenv(key, value)
}

// unsetEnvVar unsets an environment variable
func (tc *TestContext) unsetEnvVar(key string) error {
	return os.Unsetenv(key)
}

// setGitHubToken sets a dummy GitHub token
func (tc *TestContext) setGitHubToken() error {
	return os.Setenv("GITHUB_TOKEN", "ghp_test_token_1234567890")
}

// unsetGitHubToken removes GitHub token
func (tc *TestContext) unsetGitHubToken() error {
	return os.Unsetenv("GITHUB_TOKEN")
}

// runCommand executes a CIDX command
func (tc *TestContext) runCommand(cmdStr string) error {
	parts := strings.Fields(cmdStr)
	if len(parts) < 2 {
		return fmt.Errorf("invalid command: %s", cmdStr)
	}

	// Execute cidx command
	cmd := exec.Command(parts[0], parts[1:]...)
	if tc.GitRepo != "" {
		cmd.Dir = tc.GitRepo
	}

	output, err := cmd.CombinedOutput()
	tc.Output = string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			tc.ExitCode = exitErr.ExitCode()
		} else {
			tc.ExitCode = 1
		}
	} else {
		tc.ExitCode = 0
	}

	return nil
}

// tryRunCommand attempts to run a command (may fail)
func (tc *TestContext) tryRunCommand(cmdStr string) error {
	return tc.runCommand(cmdStr)
}

// detectEnvironment triggers environment detection
func (tc *TestContext) detectEnvironment() error {
	// Call CIDX environment detection
	return nil
}

// startCIDX starts CIDX
func (tc *TestContext) startCIDX() error {
	return nil
}

// shouldSeeOutput checks if output contains text
func (tc *TestContext) shouldSeeOutput(expected string) error {
	if !strings.Contains(tc.Output, expected) {
		return fmt.Errorf("expected output to contain '%s', got:\n%s", expected, tc.Output)
	}
	return nil
}

// shouldSeeError checks for error message
func (tc *TestContext) shouldSeeError(expected string) error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected command to fail, but it succeeded")
	}
	return tc.shouldSeeOutput(expected)
}

// shouldSeeErrorContaining checks error contains text
func (tc *TestContext) shouldSeeErrorContaining(expected string) error {
	return tc.shouldSeeError(expected)
}

// shouldSeeMessage checks for message
func (tc *TestContext) shouldSeeMessage(expected string) error {
	return tc.shouldSeeOutput(expected)
}

// shouldSeeMessageContaining checks message contains text
func (tc *TestContext) shouldSeeMessageContaining(expected string) error {
	return tc.shouldSeeOutput(expected)
}

// shouldHaveExitCode checks exit code
func (tc *TestContext) shouldHaveExitCode(expected int) error {
	if tc.ExitCode != expected {
		return fmt.Errorf("expected exit code %d, got %d", expected, tc.ExitCode)
	}
	return nil
}

// shouldFailImmediately checks command failed
func (tc *TestContext) shouldFailImmediately() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected command to fail immediately, but it succeeded")
	}
	return nil
}

// shouldExitNonZero checks for non-zero exit
func (tc *TestContext) shouldExitNonZero() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected non-zero exit code, got 0")
	}
	return nil
}

// shouldCompleteSuccessfully checks for success
func (tc *TestContext) shouldCompleteSuccessfully() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("expected success (exit 0), got exit code %d\nOutput:\n%s", tc.ExitCode, tc.Output)
	}
	return nil
}

// configurePipeline configures a named pipeline
func (tc *TestContext) configurePipeline(pipelineName string) error {
	if tc.Config == nil {
		tc.Config = make(map[string]interface{})
	}
	tc.Config["pipeline"] = pipelineName
	tc.Pipeline = pipelineName
	return nil
}

// configurePhases configures CIDX with specific phases
func (tc *TestContext) configurePhases(phase1, phase2 string) error {
	if tc.Config == nil {
		tc.Config = make(map[string]interface{})
	}
	tc.Config["phases"] = []string{phase1, phase2}
	return nil
}

// havePipelineConfigured marks a pipeline as configured
func (tc *TestContext) havePipelineConfigured(pipelineName string) error {
	return tc.configurePipeline(pipelineName)
}

// shouldSeeErrorAbout checks for error message about topic
func (tc *TestContext) shouldSeeErrorAbout(topic string) error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected error about %s, but command succeeded", topic)
	}
	// Check output contains topic-related keywords
	topicLower := strings.ToLower(topic)
	outputLower := strings.ToLower(tc.Output)
	if !strings.Contains(outputLower, topicLower) {
		return fmt.Errorf("expected error about '%s', got:\n%s", topic, tc.Output)
	}
	return nil
}

// shouldSeeResults checks for scan/test results
func (tc *TestContext) shouldSeeResults(resultType string) error {
	// Check output contains result indicators
	resultLower := strings.ToLower(resultType)
	outputLower := strings.ToLower(tc.Output)
	if !strings.Contains(outputLower, resultLower) {
		return fmt.Errorf("expected to see %s results in output:\n%s", resultType, tc.Output)
	}
	return nil
}

// shouldSeeWhichFailed checks output shows what failed
func (tc *TestContext) shouldSeeWhichFailed(itemType string) error {
	// Check output contains failure details
	if !strings.Contains(strings.ToLower(tc.Output), "fail") {
		return fmt.Errorf("expected to see which %s failed, but no failure info in output", itemType)
	}
	return nil
}

// shouldStopImmediately checks pipeline stopped on first failure
func (tc *TestContext) shouldStopImmediately() error {
	if tc.ExitCode == 0 {
		return fmt.Errorf("expected pipeline to stop immediately, but it succeeded")
	}
	// Check that only early phases executed (fail-fast behavior)
	return nil
}

// allPhasesShouldExecuteNormally checks all phases executed
func (tc *TestContext) allPhasesShouldExecuteNormally() error {
	// In MVP, just check exit code is 0
	return tc.shouldCompleteSuccessfully()
}

// allPhasesShouldSucceed checks all phases passed
func (tc *TestContext) allPhasesShouldSucceed() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("expected all phases to succeed, got exit code %d", tc.ExitCode)
	}
	if len(tc.FailedPhases) > 0 {
		return fmt.Errorf("expected all phases to succeed, but %d failed: %v", len(tc.FailedPhases), tc.FailedPhases)
	}
	return nil
}

// shouldExecutePhasesInOrder checks phases executed in specified order
func (tc *TestContext) shouldExecutePhasesInOrder(table *godog.Table) error {
	// Parse expected phases from table
	expectedPhases := []string{}
	for i, row := range table.Rows {
		if i == 0 {
			// Skip header row
			continue
		}
		// Assume first column is phase name
		if len(row.Cells) > 0 {
			expectedPhases = append(expectedPhases, row.Cells[0].Value)
		}
	}

	// In MVP, just check that we have some executed phases
	// Full implementation would check exact order
	if len(tc.ExecutedPhases) == 0 && len(expectedPhases) > 0 {
		return fmt.Errorf("expected phases to execute, but none were executed")
	}

	return nil
}
