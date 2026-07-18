package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// CommitPushWatchAction orchestrates commit, push, and CI watching
type CommitPushWatchAction struct {
	repo     *vcs.Repository
	provider remote.Provider
	message  string

	// ciStartTimeout bounds how long we wait for CI checks to appear after
	// the push (issue #167). Overridable in tests.
	ciStartTimeout time.Duration
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

	// 4. Watch CI for the commit we just pushed. The local HEAD SHA is the
	// source of truth: resolving the head from the provider API right after
	// the push can return the previous commit (replication lag).
	pushedSHA, err := a.repo.GetHeadSHA()
	if err != nil {
		return fmt.Errorf("failed to get pushed commit SHA: %w", err)
	}

	return a.watchCI(ctx, branch, pushedSHA)
}

// watchCI finds the PR for branch, waits for CI checks to start on the
// pushed commit, then streams check updates until completion.
//
// It reuses the same provider wait logic as `pr merge`
// (WaitForChecksToStart), pinned to the pushed commit SHA. This replaces
// the old single workflow lookup after a fixed 5s sleep, which gave up
// before GitHub Actions had created the run and then suggested creating a
// PR that already existed (issue #167).
func (a *CommitPushWatchAction) watchCI(ctx context.Context, branch, pushedSHA string) error {
	prNumber, prURL, err := a.provider.GetPullRequestByBranch(ctx, branch)
	if err != nil {
		log.Warn("⚠️  No PR found for this branch")
		log.Info("💡 Create a PR first: cidx pr create \"your title\"")
		return nil
	}

	log.Infof("⏳ Waiting for CI to start on PR #%d...", prNumber)
	headSHA, checks, err := a.provider.WaitForChecksToStart(ctx, prNumber, pushedSHA, a.ciStartTimeout)
	if err != nil {
		if checks != nil && checks.TotalCount == 0 {
			log.Warnf("⚠️  PR #%d exists but no CI checks started within %s", prNumber, a.ciStartTimeout)
			log.Info("💡 Watch them once they start: cidx pr watch -q")
			return nil
		}
		return fmt.Errorf("failed waiting for CI to start: %w", err)
	}

	shortSHA := headSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	log.Infof("📍 Watching CI for commit %s", shortSHA)
	log.Infof("🔗 %s", prURL)

	displayChecksStatus(checks)

	// If checks are still running, stream updates until they complete
	if checks.Pending > 0 {
		updates, err := a.provider.WatchPullRequestChecks(ctx, prNumber)
		if err != nil {
			return fmt.Errorf("failed to watch PR checks: %w", err)
		}

		for update := range updates {
			if update.Error != nil {
				return update.Error
			}

			// Verify we're still watching the same commit
			if update.Checks.HeadSHA != headSHA {
				log.Warn("⚠️  HEAD SHA changed during check - new commits were pushed")
				return fmt.Errorf("HEAD SHA changed during CI check - please retry")
			}

			displayChecksStatus(update.Checks)

			checks = update.Checks
			if checks.Pending == 0 {
				break
			}
		}

		if checks.Pending > 0 {
			return fmt.Errorf("stopped watching before checks completed")
		}
	}

	if checks.Failure > 0 {
		log.Errorf("❌ %d/%d checks failed", checks.Failure, checks.TotalCount)
		return fmt.Errorf("PR checks failed: %d/%d checks failed", checks.Failure, checks.TotalCount)
	}

	log.Info("🎉 All checks passed!")
	return nil
}
