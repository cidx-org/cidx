package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/actions"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
	"github.com/cucumber/godog"
	log "github.com/sirupsen/logrus"
)

// RegisterDryRunSteps registers the steps for previews that must not touch
// anything (issue #276).
//
// They run the real action -- actions.NewPR, the one `cidx pr create` builds --
// against a real git repository whose origin is a path that does not exist. A
// command that reaches for the remote fails the scenario instead of quietly
// passing on a machine that happens to be online, and the provider is nil, so
// an API call would not survive the attempt either.
func RegisterDryRunSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a repository on main whose remote does not answer$`, tc.repositoryWithUnreachableRemote)

	ctx.When(`^I preview a pull request titled "([^"]*)"$`, tc.previewPullRequest)

	ctx.Then(`^the preview should succeed$`, tc.previewShouldSucceed)
	ctx.Then(`^it should report that it would pull the latest changes from main$`, tc.shouldReportTheWouldBePull)
	ctx.Then(`^it should report the branch "([^"]*)" it would create$`, tc.shouldReportTheWouldBeBranch)
	ctx.Then(`^the checked-out commit should be the one it started on$`, tc.headShouldNotHaveMoved)
	ctx.Then(`^no branch "([^"]*)" should exist$`, tc.branchShouldNotExist)
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
	repo, err := vcs.OpenRepository(tc.GitRepo)
	if err != nil {
		return fmt.Errorf("failed to open the scenario repository: %w", err)
	}

	var reported bytes.Buffer
	previous := log.StandardLogger().Out
	log.SetOutput(&reported)
	defer log.SetOutput(previous)

	// A nil provider: reaching the remote API is not something this command may
	// do under --dry-run, and a call would not get past the attempt.
	err = actions.NewPR(repo, nil, title, "", true, false).Execute(context.Background())

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
