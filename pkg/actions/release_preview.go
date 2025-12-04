package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleasePreviewAction shows what will happen during release
type ReleasePreviewAction struct {
	repo   *vcs.Repository
	dryRun bool
}

// NewReleasePreview creates a new release preview action
func NewReleasePreview(repo *vcs.Repository, dryRun bool) *ReleasePreviewAction {
	return &ReleasePreviewAction{
		repo:   repo,
		dryRun: dryRun,
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
		log.Info("   Run: cidx action release prepare")
	}

	// 2. Show current version
	currentVersion, _ := readVersionFile(workDir)
	log.Infof("📦 Current version: v%s", currentVersion)

	// 3. Get last tag and commits
	lastTag := a.getLastTag()
	commits := a.getCommitCount(lastTag)
	log.Infof("🏷️  Last tag: %s", lastTag)
	log.Infof("📝 Commits since tag: %d", commits)

	// 4. Analyze commits for version suggestion
	commitTypes := a.analyzeCommits(lastTag)
	log.Info("")
	log.Info("📊 Commit analysis:")
	if commitTypes["breaking"] > 0 {
		log.Infof("   🚨 Breaking changes: %d → MAJOR bump", commitTypes["breaking"])
	}
	if commitTypes["feat"] > 0 {
		log.Infof("   ✨ Features: %d → MINOR bump", commitTypes["feat"])
	}
	if commitTypes["fix"] > 0 {
		log.Infof("   🐛 Fixes: %d → PATCH bump", commitTypes["fix"])
	}
	if commitTypes["other"] > 0 {
		log.Infof("   📦 Other: %d", commitTypes["other"])
	}

	// 5. Check for prepared version or suggest one
	var nextVersion string
	if hasPreparedVer {
		nextVersion = preparedVersion
		log.Info("")
		log.Infof("🚀 Prepared version: v%s (editable in %s)", nextVersion, ReleaseVersionFile)
	} else {
		nextVersion = a.suggestVersion(currentVersion, commitTypes)
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
	hasBlockers := false

	// Check uncommitted changes
	hasChanges, _ := a.repo.HasChanges()
	if hasChanges {
		log.Warn("⚠️  You have uncommitted changes")
		hasBlockers = true
	}

	// Check branch
	branch, _ := a.repo.GetCurrentBranch()
	if branch != "main" && branch != "master" {
		log.Warnf("⚠️  You are on branch '%s', not main", branch)
		log.Info("   💡 For protected branches: prepare here → commit → PR → merge → release create on main")
		hasBlockers = true
	}

	if !hasBlockers {
		log.Info("✅ Ready for release!")
		log.Info("")
		log.Info("📌 To create the release, run:")
		log.Info("   cidx action release create")
	} else {
		log.Info("")
		log.Info("📌 Fix the warnings above before releasing")
	}

	return nil
}

// getLastTag returns the most recent tag
func (a *ReleasePreviewAction) getLastTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "(none)"
	}
	return strings.TrimSpace(string(output))
}

// getCommitCount returns number of commits since tag
func (a *ReleasePreviewAction) getCommitCount(tag string) int {
	var args []string
	if tag != "(none)" && tag != "" {
		args = []string{"rev-list", "--count", tag + "..HEAD"}
	} else {
		args = []string{"rev-list", "--count", "HEAD"}
	}

	cmd := exec.Command("git", args...)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	var count int
	_, _ = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &count)
	return count
}

// analyzeCommits categorizes commits by type
func (a *ReleasePreviewAction) analyzeCommits(tag string) map[string]int {
	counts := map[string]int{
		"breaking": 0,
		"feat":     0,
		"fix":      0,
		"other":    0,
	}

	var args []string
	if tag != "(none)" && tag != "" {
		args = []string{"log", tag + "..HEAD", "--pretty=format:%s|%b"}
	} else {
		args = []string{"log", "--pretty=format:%s|%b"}
	}

	cmd := exec.Command("git", args...)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return counts
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		if strings.Contains(line, "BREAKING CHANGE") || strings.Contains(line, "!:") {
			counts["breaking"]++
		} else if strings.HasPrefix(line, "feat") {
			counts["feat"]++
		} else if strings.HasPrefix(line, "fix") {
			counts["fix"]++
		} else {
			counts["other"]++
		}
	}

	return counts
}

// suggestVersion suggests next version based on commit types
func (a *ReleasePreviewAction) suggestVersion(current string, types map[string]int) string {
	var major, minor, patch int
	_, _ = fmt.Sscanf(current, "%d.%d.%d", &major, &minor, &patch)

	if types["breaking"] > 0 {
		major++
		minor = 0
		patch = 0
	} else if types["feat"] > 0 {
		minor++
		patch = 0
	} else {
		patch++
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// readVersionFile reads VERSION file
func readVersionFile(workDir string) (string, error) {
	cmd := exec.Command("cat", "VERSION")
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "0.0.0", err
	}
	return strings.TrimSpace(string(output)), nil
}
