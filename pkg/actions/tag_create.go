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

// TagCreateAction creates and optionally pushes a git tag
type TagCreateAction struct {
	repo      *vcs.Repository
	tagConfig config.TagConfig
	dryRun    bool
}

// NewTagCreate creates a new tag create action
func NewTagCreate(repo *vcs.Repository, tagConfig config.TagConfig, dryRun bool) *TagCreateAction {
	return &TagCreateAction{
		repo:      repo,
		tagConfig: tagConfig,
		dryRun:    dryRun,
	}
}

// Execute creates the tag and optionally pushes it
func (a *TagCreateAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// 1. Check for uncommitted changes
	hasChanges, err := a.repo.HasChanges()
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("cannot create tag: you have uncommitted changes. Please commit or stash them first")
	}

	// 2. Get prepared version or fail
	if !HasPreparedTagVersion(workDir) {
		return fmt.Errorf("no prepared version found\n   Run 'cidx action tag prepare' first")
	}

	version, err := LoadPreparedTagVersion(workDir)
	if err != nil {
		return fmt.Errorf("failed to load prepared version: %w", err)
	}

	tagName := a.tagConfig.FormatTag(version)
	log.Infof("🏷️  Creating tag %s...", tagName)

	// 3. Check if tag already exists
	if a.tagExists(tagName) {
		return fmt.Errorf("tag %s already exists\n   Delete it first: cidx action tag delete %s", tagName, tagName)
	}

	// 4. Get message if annotated
	var message string
	if a.tagConfig.RequireAnnotated {
		if HasTagMessage(workDir) {
			message, err = LoadTagMessage(workDir)
			if err != nil {
				return fmt.Errorf("failed to load tag message: %w", err)
			}
		} else {
			message = fmt.Sprintf("Release %s", tagName)
		}
	}

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would execute:")
		a.showDryRun(tagName, message)
		return nil
	}

	// 5. Create the tag
	if err := a.createTag(tagName, message); err != nil {
		return fmt.Errorf("failed to create tag: %w", err)
	}
	log.Infof("✓ Tag %s created", tagName)

	// 6. Push if configured
	if a.tagConfig.AutoPush {
		log.Infof("📤 Pushing tag %s...", tagName)
		if err := a.pushTag(tagName); err != nil {
			return fmt.Errorf("failed to push tag: %w", err)
		}
		log.Infof("✓ Tag %s pushed", tagName)
	}

	// 7. Cleanup prepared files
	a.cleanup(workDir)

	log.Info("")
	log.Infof("🎉 Tag %s created successfully!", tagName)

	if a.tagConfig.LinkedToRelease {
		log.Info("")
		log.Info("📌 Next steps:")
		log.Info("   The tag push should trigger your release workflow.")
		log.Info("   Run 'cidx action release create' if you want to manually trigger it.")
	}

	return nil
}

// tagExists checks if a tag already exists
func (a *TagCreateAction) tagExists(tagName string) bool {
	cmd := exec.Command("git", "rev-parse", tagName)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	err := cmd.Run()
	return err == nil
}

// createTag creates the git tag
func (a *TagCreateAction) createTag(tagName, message string) error {
	workDir, _ := a.repo.GetWorkDir()

	var args []string

	if a.tagConfig.RequireAnnotated && message != "" {
		// Annotated tag
		args = []string{"tag", "-a", tagName, "-m", message}
	} else {
		// Lightweight tag
		args = []string{"tag", tagName}
	}

	// Add signing if configured
	if a.tagConfig.SignTags {
		args = append(args[:2], append([]string{"-s"}, args[2:]...)...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

// pushTag pushes the tag to origin
func (a *TagCreateAction) pushTag(tagName string) error {
	workDir, _ := a.repo.GetWorkDir()

	cmd := exec.Command("git", "push", "origin", tagName)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

// showDryRun displays what would be executed
func (a *TagCreateAction) showDryRun(tagName, message string) {
	var args []string

	if a.tagConfig.RequireAnnotated && message != "" {
		args = []string{"git", "tag", "-a", tagName, "-m", "<message>"}
	} else {
		args = []string{"git", "tag", tagName}
	}

	if a.tagConfig.SignTags {
		args = append(args[:3], append([]string{"-s"}, args[3:]...)...)
	}

	log.Infof("   %s", strings.Join(args, " "))

	if a.tagConfig.AutoPush {
		log.Infof("   git push origin %s", tagName)
	}

	if message != "" {
		log.Info("")
		log.Info("   Message:")
		lines := strings.Split(message, "\n")
		maxLines := 5
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		for i := 0; i < maxLines; i++ {
			log.Infof("   | %s", lines[i])
		}
	}
}

// cleanup removes prepared files
func (a *TagCreateAction) cleanup(workDir string) {
	if err := CleanupPreparedTagVersion(workDir); err != nil {
		log.Debugf("Could not cleanup version file: %v", err)
	} else {
		log.Debug("Cleaned up prepared version file")
	}

	if err := CleanupTagMessage(workDir); err != nil {
		log.Debugf("Could not cleanup message file: %v", err)
	} else {
		log.Debug("Cleaned up tag message file")
	}
}
