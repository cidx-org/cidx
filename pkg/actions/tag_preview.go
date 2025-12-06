package actions

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// TagPreviewAction shows what will happen during tag creation
type TagPreviewAction struct {
	repo      *vcs.Repository
	tagConfig config.TagConfig
}

// NewTagPreview creates a new tag preview action
func NewTagPreview(repo *vcs.Repository, tagConfig config.TagConfig) *TagPreviewAction {
	return &TagPreviewAction{
		repo:      repo,
		tagConfig: tagConfig,
	}
}

// Execute shows a preview of the tag operation
func (a *TagPreviewAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	log.Info("🔍 Tag Preview")
	log.Info("==============")
	log.Info("")

	// 1. Check for prepared version
	var version string
	hasPrepared := HasPreparedTagVersion(workDir)
	if hasPrepared {
		version, _ = LoadPreparedTagVersion(workDir)
		log.Infof("✓ Prepared version: %s (editable in %s)", version, TagVersionFile)
	} else {
		log.Warn("⚠️  No prepared version found")
		log.Info("   Run: cidx action tag prepare")
		version = a.suggestVersion()
		log.Infof("   Suggested version: %s", version)
	}

	// 2. Format tag name
	tagName := a.tagConfig.FormatTag(version)
	log.Infof("🏷️  Tag name: %s", tagName)

	// 3. Check for prepared message
	hasMessage := HasTagMessage(workDir)
	if hasMessage {
		message, _ := LoadTagMessage(workDir)
		log.Infof("✓ Tag message prepared (%s)", TagMessageFile)
		log.Info("")
		log.Info("📋 Message preview:")
		log.Info("───────────────────")
		lines := strings.Split(message, "\n")
		maxLines := 10
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		for i := 0; i < maxLines; i++ {
			fmt.Println(lines[i])
		}
		if len(lines) > 10 {
			fmt.Printf("... (%d more lines)\n", len(lines)-10)
		}
		log.Info("───────────────────")
	} else if a.tagConfig.RequireAnnotated {
		log.Warn("⚠️  No tag message prepared (required for annotated tags)")
		log.Info("   Run: cidx action tag prepare")
	}

	// 4. Show last tags
	log.Info("")
	log.Info("📜 Recent tags:")
	a.showRecentTags()

	// 5. Check if tag already exists
	if a.tagExists(tagName) {
		log.Warnf("⚠️  Tag %s already exists!", tagName)
		log.Info("   Delete it first: cidx action tag delete " + tagName)
	}

	// 6. Show configuration
	log.Info("")
	log.Info("⚙️  Configuration:")
	log.Infof("   Prefix: %q", a.tagConfig.Prefix)
	log.Infof("   Annotated: %v", a.tagConfig.RequireAnnotated)
	log.Infof("   GPG Sign: %v", a.tagConfig.SignTags)
	log.Infof("   Auto Push: %v", a.tagConfig.AutoPush)
	log.Infof("   Commitizen: %v", a.tagConfig.UseCommitizen)
	log.Infof("   Linked to release: %v", a.tagConfig.LinkedToRelease)

	// 7. Show what will happen
	log.Info("")
	log.Info("🎯 Tag create will:")
	log.Infof("   1. Create tag %s", tagName)
	if a.tagConfig.RequireAnnotated {
		log.Info("   2. Add annotated message")
	}
	if a.tagConfig.SignTags {
		log.Info("   3. Sign tag with GPG")
	}
	if a.tagConfig.AutoPush {
		log.Infof("   4. Push tag to origin")
	}
	if a.tagConfig.LinkedToRelease {
		log.Info("   5. Trigger release workflow (if configured)")
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

	// Check if not prepared
	if !hasPrepared {
		log.Warn("⚠️  No version prepared")
		hasBlockers = true
	}

	if !hasBlockers {
		log.Info("✅ Ready to create tag!")
		log.Info("")
		log.Info("📌 To create the tag, run:")
		log.Info("   cidx action tag create")
	} else {
		log.Info("")
		log.Info("📌 Fix the warnings above before creating tag")
	}

	return nil
}

// suggestVersion returns suggested version based on last tag
func (a *TagPreviewAction) suggestVersion() string {
	lastTag := a.getLastTag()
	if lastTag == "(none)" {
		return "0.1.0"
	}

	version := strings.TrimPrefix(lastTag, a.tagConfig.Prefix)
	var major, minor, patch int
	_, _ = fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch)
	patch++
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// getLastTag returns the most recent tag
func (a *TagPreviewAction) getLastTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "(none)"
	}
	return strings.TrimSpace(string(output))
}

// showRecentTags displays the last few tags
func (a *TagPreviewAction) showRecentTags() {
	cmd := exec.Command("git", "tag", "-l", "--sort=-version:refname")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		log.Info("   (no tags found)")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	maxTags := 5
	if len(lines) < maxTags {
		maxTags = len(lines)
	}

	for i := 0; i < maxTags; i++ {
		if lines[i] != "" {
			log.Infof("   %s", lines[i])
		}
	}

	if len(lines) > 5 {
		log.Infof("   ... and %d more tags", len(lines)-5)
	}
}

// tagExists checks if a tag already exists
func (a *TagPreviewAction) tagExists(tagName string) bool {
	cmd := exec.Command("git", "rev-parse", tagName)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	err := cmd.Run()
	return err == nil
}
