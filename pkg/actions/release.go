package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/arcker/cidx/pkg/config"
	"github.com/arcker/cidx/pkg/executor"
	"github.com/arcker/cidx/pkg/remote"
	"github.com/arcker/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleaseAction orchestrates the release process using dynamic action configuration
type ReleaseAction struct {
	repo       *vcs.Repository
	provider   remote.Provider
	actionName string
	dryRun     bool
}

// NewRelease creates a new release action
func NewRelease(repo *vcs.Repository, provider remote.Provider, actionName string, dryRun bool) *ReleaseAction {
	return &ReleaseAction{
		repo:       repo,
		provider:   provider,
		actionName: actionName,
		dryRun:     dryRun,
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

	// 2. Check for uncommitted changes
	hasChanges, err := a.repo.HasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if hasChanges {
		return fmt.Errorf("cannot create release: you have uncommitted changes. Please commit or stash them first")
	}

	// 3. Get current branch
	branch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if branch != "main" {
		log.Warnf("⚠️  You are on branch '%s', not 'main'. Are you sure you want to create a release?", branch)
	}

	// 4. Get working directory
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// 5. Execute action container (version bump)
	log.Info("🔍 Analyzing commits and determining version bump...")

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would execute action container")
		log.Infof("   Image: %s", action.Image)
		log.Infof("   Command: %s", action.Command)
		log.Info("   This would:")
		log.Info("   - Analyze conventional commits since last tag")
		log.Info("   - Determine semantic version bump (major/minor/patch)")
		log.Info("   - Update VERSION and .cz.toml files")
		log.Info("   - Create version bump commit and tag")
		if action.AutoPush {
			log.Info("   - Push commit to remote")
		}
		if action.PushTags {
			log.Info("   - Push tags to remote")
		}
		if action.WatchWorkflow {
			log.Info("   - Watch release workflow")
		}
		return nil
	}

	// Convert Action to ToolConfig for executor
	// Expand ${WORKSPACE} in volumes
	volumes := make([]string, len(action.Volumes))
	for i, vol := range action.Volumes {
		volumes[i] = strings.ReplaceAll(vol, "${WORKSPACE}", workDir)
	}

	toolConfig := &config.ToolConfig{
		Name:    a.actionName,
		Phase:   "action",
		Image:   action.Image,
		Command: action.Command,
		Workdir: action.Workdir,
		Volumes: volumes,
		Env:     action.Env,
	}

	// Execute using Docker executor
	dockerExec, err := executor.NewDockerExecutor(false, false)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	if err := dockerExec.Run(ctx, toolConfig); err != nil {
		return fmt.Errorf("action execution failed: %w", err)
	}

	// 6. Read new version from VERSION file
	newVersion, err := a.readVersionFile(workDir)
	if err != nil {
		return fmt.Errorf("failed to read new version: %w", err)
	}

	log.Infof("✓ Version bumped to v%s", newVersion)

	// 7. Auto-push commit if configured
	if action.AutoPush {
		log.Info("📤 Pushing version bump commit...")
		if err := a.repo.Push(); err != nil {
			return fmt.Errorf("failed to push commit: %w", err)
		}
		log.Info("✓ Commit pushed")
	}

	// 8. Push tags if configured
	if action.PushTags {
		log.Infof("🏷️  Pushing tag v%s...", newVersion)
		tagName := fmt.Sprintf("v%s", newVersion)
		pushTagCmd := exec.Command("git", "push", "origin", tagName)
		pushTagCmd.Dir = workDir
		if output, err := pushTagCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to push tag: %w\n%s", err, output)
		}
		log.Infof("✓ Tag %s pushed", tagName)
	}

	// 9. Watch workflow if configured
	if !action.WatchWorkflow {
		log.Info("✅ Release action completed")
		return nil
	}

	log.Info("⏳ Waiting for release workflow to start...")

	// Get latest workflow for the tag
	tagName := fmt.Sprintf("v%s", newVersion)
	workflow, err := a.provider.GetLatestWorkflow(ctx, tagName)
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

		a.displayWorkflow(update.Workflow)

		if update.Workflow.Status == "completed" {
			fmt.Println() // New line after progress
			if update.Workflow.Conclusion == "success" {
				log.Infof("🎉 Release v%s completed successfully!", newVersion)
				log.Infof("🔗 View release at: https://github.com/%s/releases/tag/v%s", a.getRepoPath(), newVersion)
			} else {
				log.Errorf("❌ Release workflow failed: %s", update.Workflow.Conclusion)
				return fmt.Errorf("release workflow failed with conclusion: %s", update.Workflow.Conclusion)
			}
			break
		}
	}

	return nil
}

// readVersionFile reads the current version from the VERSION file
func (a *ReleaseAction) readVersionFile(workDir string) (string, error) {
	versionFile := fmt.Sprintf("%s/VERSION", workDir)
	content, err := os.ReadFile(versionFile)
	if err != nil {
		return "", fmt.Errorf("failed to read VERSION file: %w", err)
	}

	version := strings.TrimSpace(string(content))
	return version, nil
}


// displayWorkflow renders the current workflow status
func (a *ReleaseAction) displayWorkflow(w *remote.Workflow) {
	fmt.Printf("\r\033[K") // Clear line

	for i, job := range w.Jobs {
		var icon string
		switch job.Status {
		case "completed":
			switch job.Conclusion {
			case "success":
				icon = "✓"
			case "skipped":
				icon = "○"
			default:
				icon = "✗"
			}
		case "in_progress":
			icon = "⏳"
		case "queued":
			icon = "○"
		default:
			icon = "?"
		}

		fmt.Printf("[%s] %s", icon, job.Name)
		if i < len(w.Jobs)-1 {
			fmt.Printf(" ")
		}
	}
}

// getRepoPath returns owner/repo from the repository
func (a *ReleaseAction) getRepoPath() string {
	owner, repo, err := a.repo.GetRemoteInfo()
	if err != nil {
		return "unknown/unknown"
	}
	return fmt.Sprintf("%s/%s", owner, repo)
}
