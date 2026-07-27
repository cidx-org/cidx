package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleasePreviewAction shows what will happen during release
type ReleasePreviewAction struct {
	repo          *vcs.Repository
	releaseConfig config.ReleaseConfig
	dryRun        bool
}

// NewReleasePreview creates a new release preview action
func NewReleasePreview(repo *vcs.Repository, releaseConfig config.ReleaseConfig, dryRun bool) *ReleasePreviewAction {
	return &ReleasePreviewAction{
		repo:          repo,
		releaseConfig: releaseConfig,
		dryRun:        dryRun,
	}
}

// Execute shows a preview of the release
func (a *ReleasePreviewAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	log.Info("🔍 Release Preview")
	log.Info("==================")
	log.Info("")

	// 1. Check for prepared version first
	var preparedVersion string
	hasPreparedVer := HasPreparedVersion(workDir)
	if hasPreparedVer {
		preparedVersion, _ = LoadPreparedVersion(workDir)
	}

	// Check for prepared notes (need version to find the file)
	hasPrepared := false
	if hasPreparedVer {
		hasPrepared = HasPreparedNotes(workDir, preparedVersion)
	}

	if hasPrepared {
		log.Infof("✓ Release notes prepared (%s)", GetReleaseNotesFile(preparedVersion))
	} else {
		log.Warn("⚠️  No release notes prepared")
		log.Info("   Run: cidx release prepare")
	}

	// 2. Show the current version, reconciled from the latest tag
	state := ResolveVersion(workDir)
	log.Infof("📦 Current version: v%s (latest tag)", state.Current())
	log.Infof("🏷️  Last tag: %s", state.LastTagDisplay())
	logDivergence(state.DivergenceError())

	// 3. Analyze the commits in the range
	counts := countCommits(workDir, state.LastTag)
	log.Infof("📝 Commits since tag: %d", counts.Total())

	log.Info("")
	log.Info("📊 Commit analysis:")
	if counts.Breaking > 0 {
		log.Infof("   🚨 Breaking changes: %d → MAJOR bump", counts.Breaking)
	}
	if counts.Feat > 0 {
		log.Infof("   ✨ Features: %d → MINOR bump", counts.Feat)
	}
	if counts.Fix > 0 {
		log.Infof("   🐛 Fixes: %d → PATCH bump", counts.Fix)
	}
	if counts.Other > 0 {
		log.Infof("   📦 Other: %d", counts.Other)
	}

	// 4. Check for prepared version or suggest one
	var nextVersion string
	if hasPreparedVer {
		nextVersion = preparedVersion
		log.Info("")
		log.Infof("🚀 Prepared version: v%s (editable in %s)", nextVersion, ReleaseVersionFile)
	} else {
		nextVersion = NextVersion(state.Current(), counts)
		log.Info("")
		log.Infof("🚀 Suggested next version: v%s", nextVersion)
	}

	// 6. Show prepared release notes preview
	if hasPrepared {
		log.Info("")
		log.Info("📋 Release notes preview:")
		log.Info("─────────────────────────")

		notes, err := LoadPreparedNotes(workDir, preparedVersion)
		if err == nil {
			// Show first 20 lines
			lines := strings.Split(notes, "\n")
			maxLines := 20
			if len(lines) < maxLines {
				maxLines = len(lines)
			}
			for i := 0; i < maxLines; i++ {
				fmt.Println(lines[i])
			}
			if len(lines) > 20 {
				fmt.Printf("\n... (%d more lines)\n", len(lines)-20)
			}
		}
		log.Info("─────────────────────────")
	}

	// 7. Show what will happen
	log.Info("")
	log.Info("🎯 Release create will:")
	log.Infof("   1. Bump version to v%s", nextVersion)
	log.Info("   2. Update VERSION and .cz.toml files")
	log.Info("   3. Create version bump commit")
	log.Infof("   4. Create and push tag v%s", nextVersion)
	log.Info("   5. Trigger GitHub release workflow")
	if hasPrepared {
		log.Info("   6. Use prepared release notes")
	} else {
		log.Info("   6. Generate release notes automatically")
	}

	// 8. Check for blockers
	log.Info("")
	// A version computed from the wrong source produces a broken tag sequence,
	// and cz bump hard-fails when the changelog and the tags disagree: stop
	// before either, not after (issue #185).
	hasBlockers := state.Diverged()
	if warnChangelogTagGap(workDir) {
		hasBlockers = true
	}

	// Check uncommitted changes
	hasChanges, _ := a.repo.HasChanges()
	if hasChanges {
		log.Warn("⚠️  You have uncommitted changes")
		hasBlockers = true
	}

	// Check branch
	branch, _ := a.repo.GetCurrentBranch()
	mainBranch := a.releaseConfig.GetMainBranch()
	isOnMainBranch := branch == mainBranch || (mainBranch == "main" && branch == "master")

	if !isOnMainBranch {
		if a.releaseConfig.AllowReleaseFromAnyBranch {
			log.Infof("ℹ️  You are on branch '%s' (releases allowed from any branch)", branch)
		} else {
			log.Warnf("⚠️  You are on branch '%s', not '%s'", branch, mainBranch)
			log.Infof("   💡 For protected branches: prepare here → commit → PR → merge → release create on %s", mainBranch)
			hasBlockers = true
		}
	}

	if !hasBlockers {
		log.Info("✅ Ready for release!")
		log.Info("")
		log.Info("📌 To create the release, run:")
		log.Info("   cidx release create")
	} else {
		log.Info("")
		log.Info("📌 Fix the warnings above before releasing")
	}

	return nil
}
