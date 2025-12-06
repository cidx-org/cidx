package actions

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// TagDeleteAction deletes a git tag locally and optionally from remote
type TagDeleteAction struct {
	repo      *vcs.Repository
	tagConfig config.TagConfig
	tagName   string
	remote    bool
	force     bool
	dryRun    bool
}

// NewTagDelete creates a new tag delete action
func NewTagDelete(repo *vcs.Repository, tagConfig config.TagConfig, tagName string, remote, force, dryRun bool) *TagDeleteAction {
	return &TagDeleteAction{
		repo:      repo,
		tagConfig: tagConfig,
		tagName:   tagName,
		remote:    remote,
		force:     force,
		dryRun:    dryRun,
	}
}

// Execute deletes the specified tag
func (a *TagDeleteAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// 1. Check if tag exists
	if !a.tagExists() {
		return fmt.Errorf("tag %s does not exist", a.tagName)
	}

	// 2. Check if tag is protected
	if a.isProtected() && !a.force {
		return fmt.Errorf("tag %s is protected and cannot be deleted\n   Use --force to override protection", a.tagName)
	}

	// 3. Show tag info
	log.Infof("🏷️  Tag: %s", a.tagName)
	a.showTagInfo(workDir)

	if a.dryRun {
		log.Info("")
		log.Info("🏁 Dry-run mode: would execute:")
		log.Infof("   git tag -d %s", a.tagName)
		if a.remote {
			log.Infof("   git push origin :refs/tags/%s", a.tagName)
		}
		return nil
	}

	// 4. Delete local tag
	log.Infof("🗑️  Deleting local tag %s...", a.tagName)
	if err := a.deleteLocal(); err != nil {
		return fmt.Errorf("failed to delete local tag: %w", err)
	}
	log.Infof("✓ Local tag %s deleted", a.tagName)

	// 5. Delete remote tag if requested
	if a.remote {
		log.Infof("🗑️  Deleting remote tag %s...", a.tagName)
		if err := a.deleteRemote(); err != nil {
			log.Warnf("⚠️  Could not delete remote tag: %v", err)
			log.Info("   The local tag was deleted, but the remote tag may still exist.")
		} else {
			log.Infof("✓ Remote tag %s deleted", a.tagName)
		}
	}

	log.Info("")
	log.Infof("✅ Tag %s deleted successfully", a.tagName)

	return nil
}

// tagExists checks if the tag exists locally
func (a *TagDeleteAction) tagExists() bool {
	cmd := exec.Command("git", "rev-parse", a.tagName)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	err := cmd.Run()
	return err == nil
}

// isProtected checks if the tag matches any protected pattern
func (a *TagDeleteAction) isProtected() bool {
	for _, pattern := range a.tagConfig.ProtectedTags {
		matched, err := filepath.Match(pattern, a.tagName)
		if err == nil && matched {
			log.Debugf("Tag %s matches protected pattern %s", a.tagName, pattern)
			return true
		}
	}
	return false
}

// showTagInfo displays information about the tag
func (a *TagDeleteAction) showTagInfo(workDir string) {
	// Get tag type (annotated vs lightweight)
	cmd := exec.Command("git", "cat-file", "-t", a.tagName)
	cmd.Dir = workDir
	output, err := cmd.Output()
	if err == nil {
		tagType := strings.TrimSpace(string(output))
		if tagType == "tag" {
			log.Info("   Type: annotated")

			// Show annotation
			cmd = exec.Command("git", "tag", "-n1", a.tagName)
			cmd.Dir = workDir
			if msg, err := cmd.Output(); err == nil {
				log.Infof("   Message: %s", strings.TrimPrefix(strings.TrimSpace(string(msg)), a.tagName+" "))
			}
		} else {
			log.Info("   Type: lightweight")
		}
	}

	// Show commit
	cmd = exec.Command("git", "rev-list", "-1", a.tagName)
	cmd.Dir = workDir
	if hash, err := cmd.Output(); err == nil {
		shortHash := strings.TrimSpace(string(hash))
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}
		log.Infof("   Commit: %s", shortHash)
	}

	// Show date
	cmd = exec.Command("git", "log", "-1", "--format=%ci", a.tagName)
	cmd.Dir = workDir
	if date, err := cmd.Output(); err == nil {
		log.Infof("   Date: %s", strings.TrimSpace(string(date)))
	}
}

// deleteLocal deletes the tag locally
func (a *TagDeleteAction) deleteLocal() error {
	workDir, _ := a.repo.GetWorkDir()

	cmd := exec.Command("git", "tag", "-d", a.tagName)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

// deleteRemote deletes the tag from the remote
func (a *TagDeleteAction) deleteRemote() error {
	workDir, _ := a.repo.GetWorkDir()

	cmd := exec.Command("git", "push", "origin", ":refs/tags/"+a.tagName)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}
