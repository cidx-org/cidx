package actions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// CommitPushWatchAction orchestrates commit, push, and CI watching
type CommitPushWatchAction struct {
	repo     *vcs.Repository
	provider remote.Provider
	message  string

	// verify runs the `code` phase before the commit (issue #307). On by
	// default, cleared by --no-verify -- the contract `git commit` already
	// has with its hooks.
	verify bool

	// ciStartTimeout bounds how long we wait for CI checks to appear after
	// the push (issue #167). Overridable in tests.
	ciStartTimeout time.Duration
}

// NewCommitPushWatch creates a new commit-push-watch action
func NewCommitPushWatch(repo *vcs.Repository, provider remote.Provider, message string, verify bool) *CommitPushWatchAction {
	return &CommitPushWatchAction{
		repo:           repo,
		provider:       provider,
		message:        message,
		verify:         verify,
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

	// 1. Check for changes. Untracked files count: Commit() runs `git add .`,
	// so a brand-new file is committable — reporting "No changes to commit"
	// left the user believing work was pushed when it was not (issue #180).
	hasChanges, err := a.repo.HasChangesIncludingUntracked()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if !hasChanges {
		log.Info("No changes to commit")
		return nil
	}

	// 2. Run the checks CI is about to run. Before the commit, which is where
	// git puts its own pre-commit gate: a failure then leaves the tree exactly
	// as it was, with nothing to amend or reset (issue #307).
	if err := a.runVerification(ctx); err != nil {
		return err
	}

	// 3. Commit
	log.Info("📝 Creating commit...")
	if err := a.repo.Commit(a.message); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	log.Info("✓ Commit created")

	// 4. Push
	log.Info("📤 Pushing to remote...")
	if err := a.repo.Push(); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}
	log.Info("✓ Pushed to remote")

	// 5. Watch CI for the commit we just pushed. The local HEAD SHA is the
	// source of truth: resolving the head from the provider API right after
	// the push can return the previous commit (replication lag).
	pushedSHA, err := a.repo.GetHeadSHA()
	if err != nil {
		return fmt.Errorf("failed to get pushed commit SHA: %w", err)
	}

	return a.watchCI(ctx, branch, pushedSHA)
}

// planVerification decides whether cpw runs the `code` phase itself, and says
// why when it does not (issue #307).
//
// It is on by default: a formatting slip costs a full CI cycle plus a second
// commit, and the phase costs ~20s locally once the images are pulled. That is
// the same trade `git commit` makes with its hooks, so it gets the same escape
// hatch -- --no-verify, spelled the way git spells it.
//
// A repository with its own pre-commit hook already has this gate, and
// Commit() shells out to git precisely so the hook fires. Running the phase
// here as well would run it twice on the same tree.
func planVerification(verify, preCommitHook bool) (run bool, reason string) {
	switch {
	case !verify:
		return false, "--no-verify was given"
	case preCommitHook:
		return false, "a pre-commit hook runs the checks on the commit"
	default:
		return true, ""
	}
}

// verificationOutcome turns what the code phase returned into what cpw does
// about it.
//
// Only a real failure stops the push. The two ways the check cannot run --
// nothing configured to run, nothing to run it in -- are reported and stepped
// over: cpw is how work reaches a remote, and a stopped Docker daemon is not a
// reason to make that impossible. CI remains the authority either way.
func verificationOutcome(err error) error {
	switch {
	case err == nil:
		log.Info("✓ Code phase passed")
		return nil

	case errors.Is(err, errNoCodePhase):
		log.Warn("⚠️  No code phase configured -- pushing without checking")
		return nil

	case errors.Is(err, errNoContainerRuntime):
		log.Warn("⚠️  No container runtime available -- pushing without running the code phase")
		log.Info("💡 See what is missing: cidx doctor")
		return nil

	default:
		return fmt.Errorf("code phase failed -- fix it, or push anyway with cidx cpw --no-verify: %w", err)
	}
}

// runVerification runs the checks CI is about to run, before anything is
// committed.
func (a *CommitPushWatchAction) runVerification(ctx context.Context) error {
	// Probed only when it can change the answer: --no-verify must not need a
	// repository, a hooks directory or a git binary to do nothing.
	preCommitHook := a.verify && a.repo.HasActivePreCommitHook()

	if run, reason := planVerification(a.verify, preCommitHook); !run {
		log.Infof("⏭️  Code phase not run here: %s", reason)
		return nil
	}

	log.Info("🔍 Running the code phase before pushing (skip with --no-verify)...")
	return verificationOutcome(verifyBeforePush(ctx))
}

// warnOnTypeMismatch says so when the commit just pushed and the pull request
// it lands in disagree about what kind of change this is.
//
// The type is a guess made at the moment you know the least about a change --
// before writing it -- so getting it wrong is ordinary. What is not ordinary is
// how it surfaces: `pr merge` squashes under the *title*, commitizen files the
// changelog from the squash subject, and nobody re-reads a title. A change
// started as `feat`, discovered to be a `fix`, and committed as one would still
// have been released under Features (issue #361).
//
// A warning rather than a refusal: the commit is already pushed, both readings
// are legitimate mid-branch, and the fix is one `cidx pr edit --title` away.
// Best-effort too — a title that cannot be read is not a reason to fail a watch
// that is otherwise fine.
func (a *CommitPushWatchAction) warnOnTypeMismatch(ctx context.Context, prNumber int) {
	title, err := a.provider.GetPullRequestTitle(ctx, prNumber)
	if err != nil {
		return
	}

	commitType, titleType, differ := typesDiffer(a.message, title)
	if !differ {
		return
	}

	log.Warnf("⚠️  This commit is a '%s', the pull request is titled '%s'", commitType, titleType)
	log.Warnf("   The squash subject comes from the title, and the changelog from the squash subject,")
	log.Warnf("   so the release would file this under '%s'. Retitle it with: cidx pr edit --title \"...\"", titleType)
}

// typesDiffer reports the conventional-commit types of a commit message and a
// pull request title when they disagree.
//
// Both have to be recognisable for the comparison to mean anything: a title
// that is not conventional carries no claim to contradict, and neither does a
// commit message. Scope and the breaking marker are deliberately not compared —
// only the type decides the changelog section.
func typesDiffer(message, title string) (commitType, titleType string, differ bool) {
	commitType = conventionalType(message)
	titleType = conventionalType(title)

	if commitType == "" || titleType == "" || commitType == titleType {
		return "", "", false
	}

	return commitType, titleType, true
}

// conventionalType returns the type of a conventional-commit header, or "" when
// the first line is not one.
func conventionalType(text string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if m := conventionalTypeRe.FindStringSubmatch(strings.TrimSpace(first)); m != nil {
		return m[1]
	}

	return ""
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

	// Checks can exist without CI having started: GitHub attaches its own
	// dependabot config check to any PR touching .github/dependabot.yml.
	// Calling that green would greenlight an unverified commit (issue #257).
	if checks.WorkflowChecks == 0 {
		log.Warnf("⚠️  No workflow started on this commit within %s", a.ciStartTimeout)
		displayChecksStatus(checks)
		log.Info("💡 Watch them once they start: cidx pr watch -q")
		return nil
	}

	shortSHA := headSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	log.Infof("📍 Watching CI for commit %s", shortSHA)
	log.Infof("🔗 %s", prURL)

	a.warnOnTypeMismatch(ctx, prNumber)

	displayChecksStatus(checks)

	// If anything is still running, stream updates until it is not. Entering
	// on `Pending > 0` skipped the watch entirely when the first read landed
	// between two stages -- the exact shape of issue #367.
	if !checks.Complete() {
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
			// Not `Pending == 0`: with a `needs:` between jobs that is also
			// true in the gap where the earlier job is green and the later
			// ones have no check yet, which is how this announced a green CI
			// on one job out of five (issue #367).
			if checks.Complete() {
				break
			}
		}

		if !checks.Complete() {
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
