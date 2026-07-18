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

	// ciStartTimeout bounds how long we poll for a CI workflow run to appear
	// after the push (issue #167). Overridable in tests.
	ciStartTimeout time.Duration
	// sleepFn overrides time.Sleep in tests. Nil means real sleep.
	sleepFn func(time.Duration)
}

// NewCommitPushWatch creates a new commit-push-watch action
func NewCommitPushWatch(repo *vcs.Repository, provider remote.Provider, message string) *CommitPushWatchAction {
	return &CommitPushWatchAction{
		repo:           repo,
		provider:       provider,
		message:        message,
		ciStartTimeout: defaultCIStartTimeout,
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

	// 5. Wait for the CI workflow run to appear. GitHub Actions typically
	// takes 5-30s to trigger a run after a push, so poll with backoff
	// instead of giving up after a single lookup (issue #167).
	log.Info("⏳ Waiting for workflow to start...")
	workflow, err := a.waitForWorkflow(ctx, branch)
	if err != nil {
		warning, hint := a.noWorkflowHint(ctx, branch)
		log.Warnf("⚠️  %s", warning)
		log.Infof("💡 %s", hint)
		return nil
	}

	log.Infof("👀 Watching workflow %s...\n", workflow.ID)
	log.Infof("🔗 %s\n", workflow.URL)

	// 6. Watch workflow
	updates, err := a.provider.WatchWorkflow(ctx, workflow.ID)
	if err != nil {
		return fmt.Errorf("failed to watch workflow: %w", err)
	}

	// 7. Display updates
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

// waitForWorkflow polls for the latest CI workflow run on branch, retrying
// with backoff until ciStartTimeout of cumulative wait has elapsed. Returns
// the last lookup error if no run appeared in time.
func (a *CommitPushWatchAction) waitForWorkflow(ctx context.Context, branch string) (*remote.Workflow, error) {
	const maxDelay = 10 * time.Second

	delay := 3 * time.Second
	elapsed := time.Duration(0)
	for {
		a.sleep(delay)
		elapsed += delay

		if err := ctx.Err(); err != nil {
			return nil, err
		}

		workflow, err := a.provider.GetLatestWorkflow(ctx, branch)
		if err == nil {
			return workflow, nil
		}
		if elapsed >= a.ciStartTimeout {
			return nil, err
		}

		delay = delay * 3 / 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

// noWorkflowHint returns an honest explanation for a missing workflow run:
// "no PR" and "PR exists but CI has not started" are different situations
// and must not share the same hint (issue #167).
func (a *CommitPushWatchAction) noWorkflowHint(ctx context.Context, branch string) (warning, hint string) {
	if prNumber, _, err := a.provider.GetPullRequestByBranch(ctx, branch); err == nil {
		return fmt.Sprintf("PR #%d exists but no CI workflow started within %s", prNumber, a.ciStartTimeout),
			"Watch it once it starts: cidx pr watch -q"
	}
	return "No CI workflow found for this branch",
		"Create a PR first: cidx pr create \"your title\""
}

// sleep waits for d, honoring the test seam when set.
func (a *CommitPushWatchAction) sleep(d time.Duration) {
	if a.sleepFn != nil {
		a.sleepFn(d)
		return
	}
	time.Sleep(d)
}
