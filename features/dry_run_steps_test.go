package features

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cucumber/godog"
	log "github.com/sirupsen/logrus"
)

// RegisterDryRunSteps registers the steps for previews that must not touch
// anything (issue #276) and must not need anything (issue #350).
//
// They type the real command line at the real tree -- commands.NewApp(), the
// one cmd/cidx runs (#317) -- from inside a git repository whose origin is a
// path that does not exist. Nothing here can reach a network, and no provider
// can be built from that remote either: a command that resolves one, or
// reaches for the remote, fails the scenario instead of quietly passing on a
// machine that happens to be online.
//
// Driving the command rather than the action is the point of the second issue:
// the preview itself was already clean, and `pr create` failed on
// `unable to parse remote URL` before reaching it -- in the wiring, where a
// scenario calling actions.NewPR directly could not see it.
func RegisterDryRunSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a repository on main whose remote does not answer$`, tc.repositoryWithUnreachableRemote)

	ctx.When(`^I preview a pull request titled "([^"]*)"$`, tc.previewPullRequest)

	ctx.Then(`^the preview should succeed$`, tc.previewShouldSucceed)
	ctx.Then(`^it should report that it would pull the latest changes from main$`, tc.shouldReportTheWouldBePull)
	ctx.Then(`^it should report the branch "([^"]*)" it would create$`, tc.shouldReportTheWouldBeBranch)
	ctx.Then(`^the checked-out commit should be the one it started on$`, tc.headShouldNotHaveMoved)
	ctx.Then(`^no branch "([^"]*)" should exist$`, tc.branchShouldNotExist)

	ctx.Given(`^no container runtime answers$`, tc.noContainerRuntimeAnswers)
	ctx.When(`^I preview the pipeline "([^"]*)"$`, tc.previewPipeline)
	ctx.Then(`^it should report the image it would run$`, tc.shouldReportTheImageItWouldRun)
	ctx.Then(`^no container should have been started$`, tc.noContainerShouldHaveBeenStarted)
}

// repositoryWithUnreachableRemote builds a one-commit repository on main with
// an origin that is a path, and not a path that exists.
func (tc *TestContext) repositoryWithUnreachableRemote() error {
	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}

	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init", "-q", dir},
		{"-C", dir, "config", "user.email", "scenario@example.test"},
		{"-C", dir, "config", "user.name", "cidx scenario"},
		{"-C", dir, "commit", "-q", "--allow-empty", "-m", "chore: root commit"},
		{"-C", dir, "remote", "add", "origin", filepath.Join(dir, "there-is-no-remote-here.git")},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("git %v failed: %w: %s", args, err, out)
		}
	}

	head, err := tc.scenarioHead()
	if err != nil {
		return err
	}
	tc.Config["head_before"] = head
	return nil
}

// scenarioHead reads the commit the scenario's repository has checked out.
func (tc *TestContext) scenarioHead() (string, error) {
	out, err := exec.Command("git", "-C", tc.GitRepo, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("failed to read HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// previewPullRequest runs `cidx pr create --dry-run` against the scenario's
// repository, capturing what it reports.
func (tc *TestContext) previewPullRequest(title string) error {
	var reported bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&reported)
	defer log.SetOutput(previous)

	err := tc.inScenarioDir(func() error {
		return commands.NewApp().Run([]string{"cidx", "pr", "create", "--dry-run", title})
	})

	tc.Output = reported.String()
	tc.ExitCode = 0
	if err != nil {
		tc.Output += err.Error()
		tc.ExitCode = 1
	}
	return nil
}

func (tc *TestContext) previewShouldSucceed() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("the preview failed: %s", tc.Output)
	}
	return nil
}

func (tc *TestContext) shouldReportTheWouldBePull() error {
	if !strings.Contains(tc.Output, "Would pull latest changes from main") {
		return fmt.Errorf("the preview never says the pull it skipped would happen:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) shouldReportTheWouldBeBranch(branch string) error {
	if !strings.Contains(tc.Output, "Would create branch: "+branch) {
		return fmt.Errorf("the preview does not name the branch %q:\n%s", branch, tc.Output)
	}
	return nil
}

func (tc *TestContext) headShouldNotHaveMoved() error {
	before, ok := tc.Config["head_before"].(string)
	if !ok {
		return fmt.Errorf("no starting commit was recorded")
	}

	after, err := tc.scenarioHead()
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("the preview moved the checked-out commit: %s -> %s", before, after)
	}
	return nil
}

func (tc *TestContext) branchShouldNotExist(branch string) error {
	out, err := exec.Command("git", "-C", tc.GitRepo, "branch", "--list", branch).Output()
	if err != nil {
		return fmt.Errorf("failed to list branches: %w", err)
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return fmt.Errorf("the preview created the branch it was only asked to describe: %s", out)
	}
	return nil
}

// The pipeline preview, issue #426. Same rule as the pull request preview above,
// one command over: `cidx run --dry-run` has to answer where nothing can run.
//
// The runtime is neutralised by pointing every backend at a socket that does not
// exist, because the suite runs on machines that do have Docker -- and a
// scenario that only proves something on a machine without one proves it
// nowhere. rememberEnv puts the variables back after the scenario.

func (tc *TestContext) noContainerRuntimeAnswers() error {
	for key, value := range map[string]string{
		"DOCKER_HOST":    "unix:///nonexistent/cidx-dry-run.sock",
		"CONTAINER_HOST": "unix:///nonexistent/cidx-dry-run.sock",
	} {
		tc.rememberEnv(key)
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to neutralise %s: %w", key, err)
		}
	}
	return nil
}

// previewPipeline types the command line the reader types, at the tree the
// binary runs.
func (tc *TestContext) previewPipeline(target string) error {
	dir, err := tc.pipelinePreviewWorkspace()
	if err != nil {
		return err
	}

	restore, err := chdirTo(dir)
	if err != nil {
		return err
	}
	defer restore()

	var out bytes.Buffer
	app := commands.NewApp()
	app.Writer, app.ErrWriter = &out, &out

	// The pipeline logs through a logrus built at construction time, which reads
	// os.Stderr as it is *then* -- so the swap has to happen before app.Run, not
	// on the standard logger the pull-request preview above redirects.
	done, err := captureStderr()
	if err != nil {
		return err
	}

	tc.ExitCode = 0
	runErr := app.Run([]string{"cidx", "run", "--dry-run", target})

	logged := done()
	tc.Output = logged + out.String()
	if runErr != nil {
		tc.Output += "\n" + runErr.Error()
		tc.ExitCode = 1
	}
	return nil
}

// captureStderr redirects os.Stderr into a pipe and returns the function that
// restores it and yields what was written. Read on a goroutine, because a dry
// run that outgrew the pipe buffer would otherwise block for ever.
func captureStderr() (func() string, error) {
	original := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to capture stderr: %w", err)
	}
	os.Stderr = w

	collected := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		collected <- buf.String()
	}()

	return func() string {
		os.Stderr = original
		_ = w.Close()
		text := <-collected
		_ = r.Close()
		return text
	}, nil
}

// pipelinePreviewWorkspace is a project declaring one phase and one container,
// which is all a preview needs to have something to describe.
func (tc *TestContext) pipelinePreviewWorkspace() (string, error) {
	dir, err := os.MkdirTemp("", "cidx-dry-run-*")
	if err != nil {
		return "", fmt.Errorf("failed to create the workspace: %w", err)
	}
	tc.GitRepo = dir

	config := `[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]
`
	if err := os.WriteFile(filepath.Join(dir, "cidx.toml"), []byte(config), 0o600); err != nil {
		return "", fmt.Errorf("failed to write cidx.toml: %w", err)
	}
	return dir, nil
}

func (tc *TestContext) shouldReportTheImageItWouldRun() error {
	if !strings.Contains(tc.Output, "trivy") {
		return fmt.Errorf("the preview named no image, so it described nothing:\n%s", tc.Output)
	}
	return nil
}

// A preview that started something is the one thing this rule forbids. The
// runtime is unreachable, so anything actually attempted would have surfaced as
// a connection failure in the output.
func (tc *TestContext) noContainerShouldHaveBeenStarted() error {
	for _, evidence := range []string{"Cannot connect to the Docker daemon", "connection refused", "no such file or directory"} {
		if strings.Contains(tc.Output, evidence) {
			return fmt.Errorf("the preview reached for the runtime -- %q appears in:\n%s", evidence, tc.Output)
		}
	}
	return nil
}

// chdirTo enters dir and returns the function that goes back.
func chdirTo(dir string) (func(), error) {
	before, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to read the working directory: %w", err)
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("failed to enter %s: %w", dir, err)
	}
	return func() { _ = os.Chdir(before) }, nil
}
