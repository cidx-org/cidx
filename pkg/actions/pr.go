package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/arcker/cidx/pkg/remote"
	"github.com/arcker/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// PRAction manages pull request workflow
type PRAction struct {
	repo       *vcs.Repository
	provider   remote.Provider
	title      string
	issueNum   string
	dryRun     bool
	readyMode  bool
}

// NewPR creates a new PR action
func NewPR(repo *vcs.Repository, provider remote.Provider, title, issueNum string, dryRun, readyMode bool) *PRAction {
	return &PRAction{
		repo:      repo,
		provider:  provider,
		title:     title,
		issueNum:  issueNum,
		dryRun:    dryRun,
		readyMode: readyMode,
	}
}

// Execute runs the PR workflow
func (a *PRAction) Execute(ctx context.Context) error {
	// Check if we're marking PR as ready
	if a.readyMode {
		return a.markReady(ctx)
	}

	// Create new PR workflow
	return a.createPR(ctx)
}

// createPR creates a new PR with draft status
func (a *PRAction) createPR(ctx context.Context) error {
	log.Info("🔀 Starting PR workflow...")

	// 1. Check for uncommitted changes
	hasChanges, err := a.repo.HasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}

	if hasChanges {
		return fmt.Errorf("you have uncommitted changes. Please commit or stash them first")
	}

	// 2. Ensure we're on main
	currentBranch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if currentBranch != "main" {
		log.Warnf("⚠️  You are on branch '%s', switching to 'main'...", currentBranch)
		if err := a.repo.Checkout("main"); err != nil {
			return fmt.Errorf("failed to checkout main: %w", err)
		}
	}

	// 3. Pull latest changes
	log.Info("📥 Pulling latest changes from main...")
	if err := a.repo.Pull(); err != nil {
		return fmt.Errorf("failed to pull main: %w", err)
	}

	// 4. Generate branch name from title or issue
	var branchName string
	if a.issueNum != "" {
		branchName = fmt.Sprintf("issue-%s", a.issueNum)
		log.Infof("📋 Creating branch from issue #%s", a.issueNum)
	} else {
		// Convert title to branch name: "Add Auth System" -> "feat/add-auth-system"
		branchName = a.titleToBranchName(a.title)
		log.Infof("🌿 Creating branch: %s", branchName)
	}

	if a.dryRun {
		log.Info("🏁 Dry-run mode:")
		log.Infof("   Would create branch: %s", branchName)
		log.Infof("   Would create draft PR: %s", a.title)
		if a.issueNum != "" {
			log.Infof("   Would link to issue: #%s", a.issueNum)
		}
		return nil
	}

	// 5. Create and checkout branch
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	createBranchCmd := exec.Command("git", "checkout", "-b", branchName)
	createBranchCmd.Dir = workDir
	if output, err := createBranchCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create branch: %w\n%s", err, output)
	}

	log.Infof("✅ Branch '%s' created and checked out", branchName)

	// 6. Push branch to remote (needed for PR creation)
	log.Info("📤 Pushing branch to remote...")
	pushCmd := exec.Command("git", "push", "-u", "origin", branchName)
	pushCmd.Dir = workDir
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push branch: %w\n%s", err, output)
	}

	// 7. Create draft PR using GitHub API
	log.Info("📝 Creating draft pull request...")

	prNumber, prURL, err := a.provider.CreatePullRequest(
		ctx,
		a.title,
		a.generatePRBody(),
		branchName,
		"main",
		true, // draft
	)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	_ = prNumber // May be useful later for tracking

	log.Info("✅ Draft PR created successfully!")
	log.Infof("🔗 %s", prURL)
	log.Info("")
	log.Info("📌 Next steps:")
	log.Info("   1. Make your changes")
	log.Info("   2. git add . && git commit -m 'feat: your changes'")
	log.Info("   3. git push")
	log.Info("   4. When ready: cidx action pr ready")

	return nil
}

// markReady marks the current PR as ready for review
func (a *PRAction) markReady(ctx context.Context) error {
	log.Info("🚀 Marking PR as ready for review...")

	// Get current branch
	currentBranch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if currentBranch == "main" {
		return fmt.Errorf("you are on 'main' branch. Switch to your feature branch first")
	}

	if a.dryRun {
		log.Info("🏁 Dry-run mode:")
		log.Infof("   Would mark PR for branch '%s' as ready", currentBranch)
		return nil
	}

	// Find PR for this branch using GitHub API
	log.Infof("🔍 Finding PR for branch '%s'...", currentBranch)
	prNumber, prURL, err := a.provider.GetPullRequestByBranch(ctx, currentBranch)
	if err != nil {
		return fmt.Errorf("failed to find PR: %w", err)
	}

	// Mark PR as ready using GitHub API
	log.Infof("📝 Marking PR #%d as ready...", prNumber)
	if err := a.provider.MarkPullRequestReady(ctx, prNumber); err != nil {
		return fmt.Errorf("failed to mark PR as ready: %w", err)
	}

	log.Info("✅ PR marked as ready for review!")
	log.Infof("🔗 %s", prURL)

	return nil
}

// titleToBranchName converts a PR title to a branch name
func (a *PRAction) titleToBranchName(title string) string {
	// Convert to lowercase
	name := strings.ToLower(title)

	// Replace spaces and special chars with hyphens
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, name)

	// Remove consecutive hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	// Trim hyphens
	name = strings.Trim(name, "-")

	// Add feat/ prefix (default type)
	// User can manually change branch name if needed
	return fmt.Sprintf("feat/%s", name)
}

// generatePRBody generates the PR body based on conventional commits
func (a *PRAction) generatePRBody() string {
	body := "## Description\n\n"

	if a.issueNum != "" {
		body += fmt.Sprintf("Closes #%s\n\n", a.issueNum)
	}

	body += "<!-- Add your description here -->\n\n"
	body += "## Changes\n\n"
	body += "- [ ] TODO: Describe your changes\n\n"
	body += "## Testing\n\n"
	body += "- [ ] Tests added/updated\n"
	body += "- [ ] Manual testing performed\n\n"
	body += "---\n"
	body += "🤖 Created with [CIDX](https://github.com/arcker/cidx)"

	return body
}
