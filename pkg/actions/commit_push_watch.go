package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// CommitPushWatchAction orchestrates commit, push, and workflow watching
type CommitPushWatchAction struct {
	repo     *vcs.Repository
	provider remote.Provider
	message  string
}

// NewCommitPushWatch creates a new commit-push-watch action
func NewCommitPushWatch(repo *vcs.Repository, provider remote.Provider, message string) *CommitPushWatchAction {
	return &CommitPushWatchAction{
		repo:     repo,
		provider: provider,
		message:  message,
	}
}

// Execute runs the action: commit → push → watch
func (a *CommitPushWatchAction) Execute(ctx context.Context) error {
	// 0. Block direct push to main/master
	branch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}
	if branch == "main" || branch == "master" {
		return fmt.Errorf("refusing to push directly to %s -- create a feature branch first: cidx pr create \"your title\"", branch)
	}

	// 1. Check for changes
	hasChanges, err := a.repo.HasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if !hasChanges {
		log.Info("No changes to commit")
		return nil
	}

	// 2. Commit
	log.Info("📝 Creating commit...")
	if err := a.repo.Commit(a.message); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	log.Info("✓ Commit created")

	// 3. Push
	log.Info("📤 Pushing to remote...")
	if err := a.repo.Push(); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	log.Info("✓ Pushed to remote")

	// 4. Branch already known from step 0

	// 5. Wait for workflow to start
	log.Info("⏳ Waiting for workflow to start...")
	time.Sleep(5 * time.Second)

	// 6. Get latest workflow (may not exist if no PR/CI trigger)
	workflow, err := a.provider.GetLatestWorkflow(ctx, branch)
	if err != nil {
		log.Warn("⚠️  No CI workflow found for this branch")
		log.Info("💡 Create a PR first: cidx pr create \"your title\"")
		return nil
	}

	log.Infof("👀 Watching workflow %s...\n", workflow.ID)
	log.Infof("🔗 %s\n", workflow.URL)

	// 7. Watch workflow
	updates, err := a.provider.WatchWorkflow(ctx, workflow.ID)
	if err != nil {
		return fmt.Errorf("failed to watch workflow: %w", err)
	}

	// 8. Display updates
	for update := range updates {
		if update.Error != nil {
			return update.Error
		}

		DisplayWorkflowStatus(update.Workflow)

		if update.Workflow.Status == "completed" {
			fmt.Println() // New line after progress
			switch update.Workflow.Conclusion {
			case "success", "skipped", "neutral":
				log.Info("🎉 Workflow completed successfully!")
			default:
				log.Errorf("❌ Workflow failed: %s", update.Workflow.Conclusion)
				return fmt.Errorf("workflow failed with conclusion: %s", update.Workflow.Conclusion)
			}
			break
		}
	}

	return nil
}
