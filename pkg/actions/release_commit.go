package actions

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleaseCommitAction commits prepared release notes
type ReleaseCommitAction struct {
	repo   *vcs.Repository
	dryRun bool
}

// NewReleaseCommit creates a new release commit action
func NewReleaseCommit(repo *vcs.Repository, dryRun bool) *ReleaseCommitAction {
	return &ReleaseCommitAction{
		repo:   repo,
		dryRun: dryRun,
	}
}

// Execute commits the prepared release notes
func (a *ReleaseCommitAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// Check if release notes exist
	if !HasPreparedNotes(workDir) {
		return fmt.Errorf("no release notes found at %s\nRun 'cidx action release prepare' first", ReleaseNotesFile)
	}

	log.Info("📝 Committing release notes...")

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would execute:")
		log.Infof("   git add %s", ReleaseNotesFile)
		log.Info("   git commit -m \"chore: prepare release notes\"")
		return nil
	}

	// Stage the release notes file
	addCmd := exec.Command("git", "add", ReleaseNotesFile)
	addCmd.Dir = workDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage release notes: %w\n%s", err, output)
	}

	// Commit the release notes
	commitCmd := exec.Command("git", "commit", "-m", "chore: prepare release notes")
	commitCmd.Dir = workDir
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit release notes: %w\n%s", err, output)
	}

	log.Info("✓ Release notes committed")
	log.Info("")
	log.Info("📌 Next steps:")
	log.Info("   1. Run: cidx action release preview")
	log.Info("   2. Run: cidx action release create")

	return nil
}
