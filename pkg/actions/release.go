package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/executor"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// releaseBranchPrefix names the short-lived branch that carries the version
// bump commit to the main branch through a pull request (issue #184). Pushing
// the bump straight to main is rejected on any repository whose ruleset
// requires pull requests, so the PR route is the only route.
const releaseBranchPrefix = "chore/release-"

// How long the release waits for GitHub to register the run its tag push
// triggered, and how often it asks. Package-level so tests can shrink them.
var (
	tagWorkflowWaitTimeout  = 90 * time.Second
	tagWorkflowPollInterval = 3 * time.Second
)

// runGit runs a git command in workDir. Package-level so tests can record the
// plumbing the release orchestration issues without a real repository.
var runGit = func(workDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	return cmd.CombinedOutput()
}

// mergeReleasePR waits for the release PR's CI and squash-merges it, reusing
// the exact machinery behind `cidx pr merge` (checks wait, SHA pinning,
// post-merge checkout/pull/branch cleanup). Package-level so tests can drive
// the green and red CI paths.
var mergeReleasePR = func(ctx context.Context, repo *vcs.Repository, provider remote.Provider) error {
	return NewPRMerge(repo, provider, "squash", false, false, false).Execute(ctx)
}

// ReleaseAction orchestrates the release process using dynamic action configuration
type ReleaseAction struct {
	repo          *vcs.Repository
	provider      remote.Provider
	releaseConfig config.ReleaseConfig
	actionName    string
	dryRun        bool
}

// NewRelease creates a new release action
func NewRelease(repo *vcs.Repository, provider remote.Provider, releaseConfig config.ReleaseConfig, actionName string, dryRun bool) *ReleaseAction {
	return &ReleaseAction{
		repo:          repo,
		provider:      provider,
		releaseConfig: releaseConfig,
		actionName:    actionName,
		dryRun:        dryRun,
	}
}

// Execute runs the release workflow using dynamic action configuration
func (a *ReleaseAction) Execute(ctx context.Context) error {
	// 1. Load action configuration
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	action, ok := cfg.Actions[a.actionName]
	if !ok {
		return fmt.Errorf("action '%s' not found in configuration", a.actionName)
	}

	log.Infof("🚀 Running action: %s", a.actionName)
	if action.Description != "" {
		log.Infof("   %s", action.Description)
	}

	// Check for prepared version and release notes
	workDirCheck, _ := a.repo.GetWorkDir()
	hasPreparedVer := HasPreparedVersion(workDirCheck)
	var preparedVersion string
	var hasPreparedNotes bool

	if hasPreparedVer {
		preparedVersion, _ = LoadPreparedVersion(workDirCheck)
		hasPreparedNotes = HasPreparedNotes(workDirCheck, preparedVersion)
	}

	if hasPreparedNotes {
		log.Infof("📋 Using prepared release notes from %s", GetReleaseNotesFile(preparedVersion))
	} else {
		if a.releaseConfig.RequirePrepare {
			return fmt.Errorf("no prepared release notes found\n   Run 'cidx action release prepare' first\n   Or set require_prepare = false in [release] config")
		}
		log.Info("📝 No prepared notes found - GitHub will auto-generate release notes")
		log.Info("   Tip: Run 'cidx action release prepare' before creating releases")
	}

	if hasPreparedVer {
		log.Infof("🏷️  Using prepared version: v%s", preparedVersion)
	}

	// 2. Check for uncommitted changes
	hasChanges, err := a.repo.HasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if hasChanges {
		return fmt.Errorf("cannot create release: you have uncommitted changes. Please commit or stash them first")
	}

	// 3. Get current branch and check against config
	branch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	mainBranch := a.releaseConfig.GetMainBranch()
	isOnMainBranch := branch == mainBranch || (mainBranch == "main" && branch == "master")

	if !isOnMainBranch {
		if !a.releaseConfig.AllowReleaseFromAnyBranch {
			log.Warnf("⚠️  You are on branch '%s', not '%s'", branch, mainBranch)
			log.Info("   💡 Typical workflow for protected branches:")
			log.Info("      1. Prepare & commit on this branch")
			log.Infof("      2. Create PR and merge to %s", mainBranch)
			log.Infof("      3. Run 'cidx action release create' on %s", mainBranch)
			log.Info("")
			log.Info("   To allow releases from any branch, set in cidx.toml:")
			log.Info("   [release]")
			log.Info("   allow_release_from_any_branch = true")
			return fmt.Errorf("releases can only be created from '%s' branch", mainBranch)
		}
		log.Warnf("⚠️  Creating release from branch '%s' (not '%s')", branch, mainBranch)
	}

	// 4. Get working directory
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// 5. Refuse to bump from a version that disagrees with the latest tag: the
	// bump would land below (or duplicate) an already released version (#185).
	state := ResolveVersion(workDir)
	if state.Diverged() {
		return state.DivergenceError()
	}
	warnChangelogTagGap(workDir)

	// 6. Refuse to open a second release PR on top of an unfinished one
	if err := a.checkNoReleaseInFlight(ctx, workDir); err != nil {
		return err
	}

	// 7. Execute action container (version bump)
	log.Info("🔍 Analyzing commits and determining version bump...")

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would execute action container")
		log.Infof("   Image: %s", action.Image)
		if len(action.Entrypoint) > 0 {
			log.Infof("   Entrypoint: %v", action.Entrypoint)
		}
		log.Infof("   Command: %s", action.Command)
		log.Info("   This would:")
		log.Info("   1. Run the bump container: analyze conventional commits since the last")
		log.Info("      tag, pick the semantic bump, update VERSION/.cz.toml/CHANGELOG.md,")
		log.Info("      commit, and create a local tag")
		log.Infof("   2. Move that commit onto '%s<version>' and restore '%s'", releaseBranchPrefix, branch)
		log.Info("   3. Delete the local tag (the squash-merge rewrites the commit)")
		log.Info("   4. Push the branch and open the release pull request")
		log.Info("   5. Wait for the PR checks to pass, then squash-merge it")
		log.Infof("   6. Back on '%s': tag the merged commit and push the tag", branch)
		if action.WatchWorkflow {
			log.Info("   7. Watch the release workflow triggered by the tag")
		}
		return nil
	}

	// The bump commit is created on the current branch; remember where that
	// branch stood so it can be restored once the commit moves to its own branch.
	baseSHA, err := a.repo.GetHeadSHA()
	if err != nil {
		return fmt.Errorf("failed to read current commit: %w", err)
	}

	// Convert Action to ContainerConfig for executor
	// Expand ${WORKSPACE} in volumes
	volumes := make([]string, len(action.Volumes))
	for i, vol := range action.Volumes {
		volumes[i] = strings.ReplaceAll(vol, "${WORKSPACE}", workDir)
	}

	// Modify command to use exact version if prepared
	command := action.Command
	if hasPreparedVer && preparedVersion != "" {
		// Replace increment-based bump with exact version
		// The default command is typically "cz bump --changelog --yes"
		// We change it to "cz bump --exact-version X.X.X --changelog --yes"
		if strings.Contains(command, "cz bump") && !strings.Contains(command, "--exact-version") {
			command = strings.Replace(command, "cz bump", fmt.Sprintf("cz bump --exact-version %s", preparedVersion), 1)
			log.Debugf("Modified command to use exact version: %s", command)
		}
	}

	containerConfig := &config.ContainerConfig{
		Name:       a.actionName,
		Phase:      "action",
		Image:      action.Image,
		Command:    command,
		Entrypoint: action.Entrypoint,
		Workdir:    action.Workdir,
		Volumes:    volumes,
		Env:        action.Env,
		Workspace:  workDir,
	}

	// Execute using Docker executor
	dockerExec, err := executor.NewDockerExecutor(false, false, false)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	if err := dockerExec.Run(ctx, containerConfig); err != nil {
		return fmt.Errorf("action execution failed: %w", err)
	}

	// 8. Read new version from VERSION file
	newVersion, err := readVersionFile(workDir)
	if err != nil {
		return fmt.Errorf("failed to read new version: %w", err)
	}

	log.Infof("✓ Version bumped to v%s", newVersion)

	// 9. Carry the bump to the base branch through a PR, then tag the merged commit
	if err := a.releaseViaPR(ctx, workDir, branch, baseSHA, newVersion); err != nil {
		return err
	}

	// 10. Watch workflow if configured
	if !action.WatchWorkflow {
		log.Info("✅ Release action completed")
		// Cleanup prepared files when not watching workflow
		a.cleanupPreparedFiles(workDir, preparedVersion, hasPreparedNotes, hasPreparedVer)
		return nil
	}

	log.Info("⏳ Waiting for release workflow to start...")

	// Get the run the tag push just triggered
	tagName := fmt.Sprintf("v%s", newVersion)
	workflow, err := a.waitForTagWorkflow(ctx, tagName)
	if err != nil {
		log.Warnf("⚠️  Could not get workflow for tag %s: %v", tagName, err)
		log.Infof("🔗 Check release status at: https://github.com/%s/releases", a.getRepoPath())
		return nil
	}

	log.Infof("👀 Watching release workflow %s...", workflow.ID)
	log.Infof("🔗 %s", workflow.URL)

	// 8. Watch workflow
	updates, err := a.provider.WatchWorkflow(ctx, workflow.ID)
	if err != nil {
		log.Warnf("⚠️  Could not watch workflow: %v", err)
		log.Infof("🔗 Check release status at: %s", workflow.URL)
		return nil
	}

	// 9. Display updates
	for update := range updates {
		if update.Error != nil {
			return update.Error
		}

		DisplayWorkflowStatus(update.Workflow)

		if update.Workflow.Status == "completed" {
			fmt.Println() // New line after progress
			switch update.Workflow.Conclusion {
			case "success", "skipped", "neutral":
				log.Infof("🎉 Release v%s completed successfully!", newVersion)
				log.Infof("🔗 View release at: https://github.com/%s/releases/tag/v%s", a.getRepoPath(), newVersion)
				// Cleanup prepared files after successful release
				a.cleanupPreparedFiles(workDir, preparedVersion, hasPreparedNotes, hasPreparedVer)
			default:
				log.Errorf("❌ Release workflow failed: %s", update.Workflow.Conclusion)
				return fmt.Errorf("release workflow failed with conclusion: %s", update.Workflow.Conclusion)
			}
			break
		}
	}

	return nil
}

// waitForTagWorkflow returns the workflow run the pushed tag triggered.
//
// The run is resolved by tag, not by workflow file name: the release workflow
// lives in its own file, so probing the CI candidates never found it and the
// release ended on a "check the releases page" message (issue #223). GitHub
// also needs a few seconds to register the run after the push returns, hence
// the bounded polling instead of a single lookup.
func (a *ReleaseAction) waitForTagWorkflow(ctx context.Context, tagName string) (*remote.Workflow, error) {
	deadline := time.Now().Add(tagWorkflowWaitTimeout)

	for {
		workflow, err := a.provider.GetLatestRunForTag(ctx, tagName)
		if err == nil {
			return workflow, nil
		}

		if time.Now().After(deadline) {
			return nil, err
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(tagWorkflowPollInterval):
		}
	}
}

// checkNoReleaseInFlight stops the release when a release branch is already
// published on the remote. That means a previous run already opened a bump PR
// (or was interrupted after pushing), and bumping again would compute the same
// version and race a second PR against the first one.
func (a *ReleaseAction) checkNoReleaseInFlight(ctx context.Context, workDir string) error {
	branch := releaseBranchInFlight(workDir)
	if branch == "" {
		return nil
	}

	log.Errorf("❌ A release is already in flight on branch '%s'", branch)
	if _, prURL, err := a.provider.GetPullRequestByBranch(ctx, branch); err == nil {
		log.Infof("🔗 %s", prURL)
	}
	log.Info("📌 Finish it, or drop it and start over:")
	log.Info("   - finish: check out that branch, then cidx pr merge")
	log.Info("             followed by cidx release tag prepare && cidx release tag create")
	log.Info("   - drop:   close the PR and delete the branch, then run cidx release create again")

	return fmt.Errorf("release branch '%s' already exists on the remote", branch)
}

// releaseBranchInFlight returns the name of a release branch already published
// on origin, or "" when there is none.
func releaseBranchInFlight(workDir string) string {
	output, err := runGit(workDir, "ls-remote", "--heads", "origin", releaseBranchPrefix+"*")
	if err != nil {
		log.Debugf("could not list remote release branches: %v", err)
		return ""
	}

	const refPrefix = "refs/heads/"
	for _, line := range strings.Split(string(output), "\n") {
		_, ref, found := strings.Cut(strings.TrimSpace(line), refPrefix)
		if found && ref != "" {
			return ref
		}
	}

	return ""
}

// releaseViaPR moves the version bump commit created by the action container
// onto its own branch, opens a pull request, merges it once CI is green, and
// tags the merged commit.
//
// Tagging happens after the merge on purpose: the squash-merge rewrites the
// bump commit, so the tag cz created locally points at a SHA that never lands
// on the base branch (issue #184).
func (a *ReleaseAction) releaseViaPR(ctx context.Context, workDir, baseBranch, baseSHA, version string) error {
	tagName := fmt.Sprintf("v%s", version)
	branchName := releaseBranchPrefix + tagName

	// Drop the local tag cz created: it points at the pre-merge commit.
	if output, err := runGit(workDir, "tag", "-d", tagName); err != nil {
		log.Debugf("no local tag %s to delete: %v\n%s", tagName, err, output)
	}

	log.Infof("🌿 Moving the bump commit onto '%s'...", branchName)
	if output, err := runGit(workDir, "checkout", "-B", branchName); err != nil {
		return fmt.Errorf("failed to create release branch: %w\n%s", err, output)
	}
	if output, err := runGit(workDir, "branch", "-f", baseBranch, baseSHA); err != nil {
		return fmt.Errorf("failed to restore '%s': %w\n%s", baseBranch, err, output)
	}

	log.Info("📤 Pushing release branch...")
	if output, err := runGit(workDir, "push", "-u", "origin", branchName); err != nil {
		return fmt.Errorf("failed to push release branch: %w\n%s", err, output)
	}

	log.Info("📝 Opening the release pull request...")
	body := fmt.Sprintf("Version bump to %s.\n\n---\n🤖 Created with [CIDX](https://github.com/cidx-org/cidx)", tagName)
	_, prURL, err := a.provider.CreatePullRequest(
		ctx,
		fmt.Sprintf("chore(release): bump version to %s", tagName),
		body,
		branchName,
		baseBranch,
		false, // not a draft: it must run CI and be mergeable right away
	)
	if err != nil {
		return fmt.Errorf("failed to create release PR: %w", err)
	}
	log.Infof("🔗 %s", prURL)

	if err := mergeReleasePR(ctx, a.repo, a.provider); err != nil {
		log.Errorf("❌ Release PR was not merged -- no tag was created")
		log.Infof("🔗 %s", prURL)
		log.Info("📌 The version bump is safe on the PR branch. To finish the release:")
		log.Info("   1. Fix the failure and push: cidx cpw -m \"fix: ...\"")
		log.Info("   2. Merge the release PR:     cidx pr merge")
		log.Info("   3. Tag the merged commit:    cidx release tag prepare && cidx release tag create")
		return fmt.Errorf("release PR %s was not merged: %w", prURL, err)
	}

	// `pr merge` left us back on the base branch with the merge pulled, so
	// HEAD is the squashed bump commit: that is what gets tagged.
	log.Infof("🏷️  Tagging the merged commit as %s...", tagName)
	if output, err := runGit(workDir, "tag", "-a", tagName, "-m", "Release "+tagName); err != nil {
		return fmt.Errorf("failed to create tag %s: %w\n%s", tagName, err, output)
	}
	if output, err := runGit(workDir, "push", "origin", tagName); err != nil {
		return fmt.Errorf("failed to push tag %s: %w\n%s", tagName, err, output)
	}
	log.Infof("✓ Tag %s pushed", tagName)

	return nil
}

// getRepoPath returns owner/repo from the repository
func (a *ReleaseAction) getRepoPath() string {
	owner, repo, err := a.repo.GetRemoteInfo()
	if err != nil {
		return "unknown/unknown"
	}
	return fmt.Sprintf("%s/%s", owner, repo)
}

// cleanupPreparedFiles removes prepared release notes and version files (if auto_cleanup is enabled)
func (a *ReleaseAction) cleanupPreparedFiles(workDir, version string, hasNotes, hasVersion bool) {
	if !a.releaseConfig.AutoCleanup {
		log.Debug("Auto-cleanup disabled, keeping prepared files")
		return
	}

	if hasNotes && version != "" {
		if err := CleanupPreparedNotes(workDir, version); err != nil {
			log.Warnf("⚠️  Could not cleanup release notes: %v", err)
		} else {
			log.Infof("🧹 Cleaned up %s", GetReleaseNotesFile(version))
		}
	}
	if hasVersion {
		if err := CleanupPreparedVersion(workDir); err != nil {
			log.Warnf("⚠️  Could not cleanup version file: %v", err)
		} else {
			log.Info("🧹 Cleaned up prepared version file")
		}
	}
}
