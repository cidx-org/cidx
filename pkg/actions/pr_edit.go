package actions

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// PREditAction updates the title and/or body of the current branch's PR.
type PREditAction struct {
	repo     *vcs.Repository
	provider remote.Provider
	title    string
	body     string
}

// NewPREdit creates a new PR edit action. Empty title/body fields are left
// unchanged on the PR; at least one must be provided.
func NewPREdit(repo *vcs.Repository, provider remote.Provider, title, body string) *PREditAction {
	return &PREditAction{
		repo:     repo,
		provider: provider,
		title:    title,
		body:     body,
	}
}

// Execute resolves the current branch and updates its PR.
func (a *PREditAction) Execute(ctx context.Context) error {
	currentBranch, err := a.repo.GetCurrentBranch()
	if err != nil {
		return fmt.Errorf("failed to get current branch: %w", err)
	}

	if currentBranch == "main" {
		return fmt.Errorf("you are on 'main' branch. Switch to your feature branch first")
	}

	return a.editPR(ctx, currentBranch)
}

// editPR finds the PR for branch (same resolution as `pr ready`/`pr merge`)
// and updates the provided fields.
func (a *PREditAction) editPR(ctx context.Context, branch string) error {
	if a.title == "" && a.body == "" {
		return fmt.Errorf("nothing to update: provide --title and/or --body")
	}

	log.Infof("🔍 Finding PR for branch '%s'...", branch)
	prNumber, prURL, err := a.provider.GetPullRequestByBranch(ctx, branch)
	if err != nil {
		return fmt.Errorf("failed to find PR: %w", err)
	}

	log.Infof("📝 Updating PR #%d...", prNumber)
	if err := a.provider.UpdatePullRequest(ctx, prNumber, a.title, a.body); err != nil {
		return fmt.Errorf("failed to update PR: %w", err)
	}

	log.Infof("✅ PR #%d updated", prNumber)
	if a.title != "" {
		log.Infof("   Title: %s", a.title)
	}
	log.Infof("🔗 %s", prURL)

	return nil
}
