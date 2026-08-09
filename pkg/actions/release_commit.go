package actions

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/vcs"
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

	// First check for prepared version file - we need this to find the notes file
	if !HasPreparedVersion(workDir) {
		return fmt.Errorf("no prepared version found at %s\nRun 'cidx release prepare' first", ReleaseVersionFile)
	}

	version, err := LoadPreparedVersion(workDir)
	if err != nil {
		return fmt.Errorf("failed to load prepared version: %w", err)
	}

	// Check if release notes exist for this version
	notesFile := GetReleaseNotesFile(version)
	if !HasPreparedNotes(workDir, version) {
		return fmt.Errorf("no release notes found at %s\nRun 'cidx release prepare' first", notesFile)
	}

	log.Infof("📝 Committing release notes for v%s...", version)

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would execute:")
		log.Infof("   git add %s", notesFile)
		log.Infof("   git commit -m \"chore: prepare release v%s\"", version)
		return nil
	}

	// The notes, and only the notes. ReleaseVersionFile is scratch state between
	// prepare and create -- .gitignore lists it beside .cidx/tag-version and
	// .cidx/tag-message -- so staging it made `git add` refuse an ignored path
	// and no release could be cut through cidx at all (#418). It stays on disk,
	// because `release create` reads it back.
	addCmd := vcs.Git(workDir, "add", notesFile)
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to stage release notes: %w\n%s", err, output)
	}

	// Commit the release notes with version in message
	commitMsg := fmt.Sprintf("chore: prepare release v%s", version)
	commitCmd := vcs.Git(workDir, "commit", "-m", commitMsg)
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit release notes: %w\n%s", err, output)
	}

	log.Info("✓ Release notes committed")
	log.Info("")
	log.Info("📌 Next steps:")
	log.Info("   1. Run: cidx release preview")
	log.Info("   2. Run: cidx release create")

	return nil
}
