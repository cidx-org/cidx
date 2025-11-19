package main

import (
	"context"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

// TestFeatures runs all BDD scenarios
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   getFormat(),
			Paths:    []string{"features"},
			TestingT: t,
			Output:   colors.Colored(os.Stdout),
			Strict:   true,
			NoColors: false,
		},
	}

	status := suite.Run()
	if status != 0 {
		t.Fatalf("BDD scenarios failed with status %d", status)
	}
}

// InitializeScenario registers all step definitions
func InitializeScenario(ctx *godog.ScenarioContext) {
	// Initialize test context
	testCtx := NewTestContext()

	// Register step definitions
	RegisterCommonSteps(ctx, testCtx)
	RegisterEventSteps(ctx, testCtx)
	RegisterSecuritySteps(ctx, testCtx)
	RegisterPipelineSteps(ctx, testCtx)

	// Hooks
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		testCtx.Reset()
		return ctx, nil
	})

	ctx.After(func(ctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		testCtx.Cleanup()
		return ctx, nil
	})
}

// getFormat returns the output format from environment or default
func getFormat() string {
	format := os.Getenv("GODOG_FORMAT")
	if format == "" {
		format = "pretty"
	}
	return format
}

// TestContext holds shared test state
type TestContext struct {
	Environment     string
	CI              bool
	Provider        string
	EventType       string
	Pipeline        string
	ExecutedPhases  []string
	FailedPhases    []string
	Output          string
	ExitCode        int
	GitRepo         string
	Config          map[string]interface{}
}

// NewTestContext creates a new test context
func NewTestContext() *TestContext {
	return &TestContext{
		ExecutedPhases: []string{},
		FailedPhases:   []string{},
		Config:         make(map[string]interface{}),
	}
}

// Reset resets the test context between scenarios
func (tc *TestContext) Reset() {
	tc.Environment = ""
	tc.CI = false
	tc.Provider = ""
	tc.EventType = ""
	tc.Pipeline = ""
	tc.ExecutedPhases = []string{}
	tc.FailedPhases = []string{}
	tc.Output = ""
	tc.ExitCode = 0
	tc.GitRepo = ""
	tc.Config = make(map[string]interface{})
}

// Cleanup performs cleanup after scenario
func (tc *TestContext) Cleanup() {
	// Clean up test git repos, temp files, etc.
	if tc.GitRepo != "" {
		os.RemoveAll(tc.GitRepo)
	}

	// Reset environment variables
	os.Unsetenv("GITHUB_ACTIONS")
	os.Unsetenv("GITLAB_CI")
	os.Unsetenv("JENKINS_URL")
	os.Unsetenv("CIRCLECI")
	os.Unsetenv("GITHUB_TOKEN")
}
