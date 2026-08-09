package features

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"

	"github.com/cidx-org/cidx/v3/pkg/executor"
)

// Test repository configuration
const (
	TestRepoOwner = "cidx-org"
	TestRepoName  = "cidx-test-playground"
	TestRepo      = TestRepoOwner + "/" + TestRepoName
)

// TestFeatures runs BDD scenarios that don't require Docker (strict mode).
// These are the living documentation -- if they fail, the spec is broken.
//
// This is the suite CI gates on: cidx.toml overrides the go-test command to
// `go test -v ./...` so the root package -- this file and the *_steps_test.go
// definitions next to it -- is compiled and run like any other package.
func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   getFormat(),
			Paths:    []string{"."},
			Tags:     "~@docker-required",
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

// TestFeaturesDocker runs scenarios that require Docker daemon control.
// These run in best-effort mode -- pending steps are acceptable when
// the Docker environment cannot be fully controlled (e.g. in CI without DinD).
//
// Best-effort must not mean "silently green". With no runtime reachable every
// scenario short-circuits to pending, Strict:false forgives pending, and the
// suite reports PASS having asserted nothing -- the same decorative signal as
// #239 and #265. Skipping says so out loud instead.
func TestFeaturesDocker(t *testing.T) {
	if !containerRuntimeReachable() {
		t.Skip("no container runtime reachable: @docker-required scenarios need one to assert anything")
	}

	suite := godog.TestSuite{
		ScenarioInitializer: InitializeScenario,
		Options: &godog.Options{
			Format:   getFormat(),
			Paths:    []string{"."},
			Tags:     "@docker-required",
			TestingT: t,
			Output:   colors.Colored(os.Stdout),
			Strict:   false,
			NoColors: false,
		},
	}

	status := suite.Run()
	if status != 0 {
		t.Fatalf("Docker-dependent BDD scenarios failed with status %d", status)
	}
}

// containerRuntimeReachable reports whether Docker or Podman answers, using the
// same selector the executor scenarios use so the gate and the steps agree.
func containerRuntimeReachable() bool {
	selector, err := executor.NewSelector(false, false, false)
	if err != nil {
		return false
	}
	defer func() { _ = selector.Close() }()

	return selector.DockerAvailable() || selector.PodmanAvailable()
}

// InitializeScenario registers all step definitions
func InitializeScenario(ctx *godog.ScenarioContext) {
	// Initialize test context
	testCtx := NewTestContext()

	// Register step definitions
	RegisterCommonSteps(ctx, testCtx)
	RegisterEventSteps(ctx, testCtx)
	RegisterSecuritySteps(ctx, testCtx)
	RegisterBaseEOLSteps(ctx, testCtx)
	RegisterSarifSteps(ctx, testCtx)
	RegisterSummarySteps(ctx, testCtx)
	RegisterPipelineSteps(ctx, testCtx)
	RegisterExecutorSteps(ctx, testCtx)
	RegisterPresetSteps(ctx, testCtx)
	RegisterQuietSteps(ctx, testCtx)
	RegisterDoctorSteps(ctx, testCtx)
	RegisterGenerateSteps(ctx, testCtx)
	RegisterDriftSteps(ctx, testCtx)
	RegisterValidateSteps(ctx, testCtx)
	RegisterFlagPlacementSteps(ctx, testCtx)
	RegisterDryRunSteps(ctx, testCtx)
	RegisterWorkflowPhaseSteps(ctx, testCtx)
	RegisterPullPolicySteps(ctx, testCtx)
	RegisterBranchSteps(ctx, testCtx)
	RegisterArtifactWorkflowSteps(ctx, testCtx)
	RegisterPRChecksSteps(ctx, testCtx)
	RegisterCheckVerdictSteps(ctx, testCtx)
	RegisterWatchTargetSteps(ctx, testCtx)
	RegisterCPWSteps(ctx, testCtx)
	RegisterBranchDeletionSteps(ctx, testCtx)

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
	// The watch-target guard (#414): the two commits it compares, and what it
	// decided. Kept here rather than in the steps file for the same reason
	// every other scenario's state is — one context per scenario, reset by godog.
	watchLocalSHA  string
	watchRemoteSHA string
	watchProceeds  bool
	watchMessage   string

	// What a cpw run decided to do (#416).
	cpwHasChanges  bool
	cpwHasUnpushed bool
	cpwPlan        actions.CPWPlan
	cpwMessage     string

	// How a branch deletion was decided (#417).
	deleteStatus     branch.Status
	deleteLocalHash  string
	deleteRemoteHash string
	deleteForce      bool
	deleteForced     bool

	Environment    string
	CI             bool
	Provider       string
	EventType      string
	Pipeline       string
	ExecutedPhases []string
	FailedPhases   []string
	Output         string
	ExitCode       int
	GitRepo        string
	Config         map[string]any

	// Command tracking
	LastCommand  string
	CommandFlags []string

	// GitHub test artifacts (will be cleaned up after test)
	GitHubToken     string
	CreatedPRs      []int    // PR numbers to clean up
	CreatedIssues   []int    // Issue numbers to clean up
	CreatedTags     []string // Git tags to clean up
	CreatedReleases []string // Release IDs to clean up
	CurrentBranch   string
	CurrentPR       int

	// Executor test state
	Backend  string
	Executor any

	// Tools a quiet-mode scenario staged, and how long the last run took.
	Tools       []stagedTool
	RunDuration time.Duration
}

// stagedTool is a tool a scenario declares: what it writes to stdout and how it
// exits. Nothing else about a tool decides what reaches the terminal.
type stagedTool struct {
	Name     string
	Stdout   string
	ExitCode int
}

// NewTestContext creates a new test context
func NewTestContext() *TestContext {
	return &TestContext{
		ExecutedPhases:  []string{},
		FailedPhases:    []string{},
		Config:          make(map[string]any),
		CommandFlags:    []string{},
		CreatedPRs:      []int{},
		CreatedIssues:   []int{},
		CreatedTags:     []string{},
		CreatedReleases: []string{},
		GitHubToken:     os.Getenv("GITHUB_TOKEN"),
	}
}

// Reset resets the test context between scenarios
func (tc *TestContext) Reset() {
	// Clean up previous scenario artifacts
	tc.Cleanup()

	// Reset state
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
	tc.Config = make(map[string]any)
	tc.LastCommand = ""
	tc.CommandFlags = []string{}
	tc.CreatedPRs = []int{}
	tc.CreatedIssues = []int{}
	tc.CreatedTags = []string{}
	tc.CreatedReleases = []string{}
	tc.CurrentBranch = ""
	tc.CurrentPR = 0
	tc.GitHubToken = os.Getenv("GITHUB_TOKEN")
	tc.Backend = ""
	tc.Executor = nil
	tc.Tools = nil
	tc.RunDuration = 0
}

// Cleanup performs cleanup after scenario
func (tc *TestContext) Cleanup() {
	// Clean up test git repos, temp files, etc.
	if tc.GitRepo != "" {
		_ = os.RemoveAll(tc.GitRepo)
	}

	// Clean up GitHub artifacts if we have a token
	if tc.GitHubToken != "" {
		tc.cleanupGitHubArtifacts()
	}

	// Variables a scenario touched through "the environment sets ..." go back
	// to what they held, which for HOME is not "unset" (#384).
	if touched, ok := tc.Config["touched_env"].(map[string]*string); ok {
		for key, before := range touched {
			if before == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *before)
		}
	}

	// Reset environment variables
	_ = os.Unsetenv("GITHUB_ACTIONS")
	_ = os.Unsetenv("GITLAB_CI")
	_ = os.Unsetenv("JENKINS_URL")
	_ = os.Unsetenv("CIRCLECI")
	_ = os.Unsetenv("GITHUB_TOKEN")
	_ = os.Unsetenv("CI")
	_ = os.Unsetenv("GITHUB_EVENT_NAME")
	_ = os.Unsetenv("GITHUB_REF")
	_ = os.Unsetenv("GITHUB_REF_NAME")
	_ = os.Unsetenv("CI_MERGE_REQUEST_ID")
}

// cleanupGitHubArtifacts removes test artifacts from playground repo
func (tc *TestContext) cleanupGitHubArtifacts() {
	// Future: Use gh CLI or GitHub API to clean up test artifacts
}
