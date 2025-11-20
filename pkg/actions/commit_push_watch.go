package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/arcker/cidx/pkg/remote"
	"github.com/arcker/cidx/pkg/vcs"
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

	// 4. Get current branch
	branch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	// 5. Wait for workflow to start
	log.Info("⏳ Waiting for workflow to start...")
	time.Sleep(5 * time.Second)

	// 6. Get latest workflow
	workflow, err := a.provider.GetLatestWorkflow(ctx, branch)
	if err != nil {
		return fmt.Errorf("failed to get workflow: %w", err)
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

		a.displayWorkflow(update.Workflow)

		if update.Workflow.Status == "completed" {
			fmt.Println() // New line after progress
			if update.Workflow.Conclusion == "success" {
				log.Info("🎉 Workflow completed successfully!")
			} else {
				log.Errorf("❌ Workflow failed: %s", update.Workflow.Conclusion)
				return fmt.Errorf("workflow failed with conclusion: %s", update.Workflow.Conclusion)
			}
			break
		}
	}

	return nil
}

// displayWorkflow renders the current workflow status
func (a *CommitPushWatchAction) displayWorkflow(w *remote.Workflow) {
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
