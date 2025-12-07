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

// TagListAction lists git tags with filtering options
type TagListAction struct {
	repo      *vcs.Repository
	tagConfig config.TagConfig
	limit     int
	pattern   string
	verbose   bool
}

// NewTagList creates a new tag list action
func NewTagList(repo *vcs.Repository, tagConfig config.TagConfig, limit int, pattern string, verbose bool) *TagListAction {
	return &TagListAction{
		repo:      repo,
		tagConfig: tagConfig,
		limit:     limit,
		pattern:   pattern,
		verbose:   verbose,
	}
}

// Execute lists tags based on configuration
func (a *TagListAction) Execute(ctx context.Context) error {
	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// Build git tag command
	args := []string{"tag", "-l", "--sort=-version:refname"}

	// Add pattern filter if specified
	if a.pattern != "" {
		args = append(args, a.pattern)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list tags: %w", err)
	}

	tags := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(tags) == 1 && tags[0] == "" {
		log.Info("No tags found")
		return nil
	}

	// Apply limit
	if a.limit > 0 && len(tags) > a.limit {
		tags = tags[:a.limit]
	}

	// Display tags
	if a.verbose {
		a.displayVerbose(workDir, tags)
	} else {
		a.displaySimple(tags)
	}

	return nil
}

// displaySimple shows tags in a simple list format
func (a *TagListAction) displaySimple(tags []string) {
	log.Infof("🏷️  Tags (%d):", len(tags))
	log.Info("")

	for _, tag := range tags {
		protected := ""
		if a.isProtected(tag) {
			protected = " 🔒"
		}
		fmt.Printf("  %s%s\n", tag, protected)
	}
}

// displayVerbose shows tags with additional information
func (a *TagListAction) displayVerbose(workDir string, tags []string) {
	log.Infof("🏷️  Tags (%d):", len(tags))
	log.Info("")

	// Header
	fmt.Printf("  %-20s %-10s %-20s %s\n", "TAG", "TYPE", "DATE", "COMMIT")
	fmt.Printf("  %-20s %-10s %-20s %s\n", "---", "----", "----", "------")

	for _, tag := range tags {
		tagType := a.getTagType(workDir, tag)
		date := a.getTagDate(workDir, tag)
		commit := a.getTagCommit(workDir, tag)

		protected := ""
		if a.isProtected(tag) {
			protected = " 🔒"
		}

		fmt.Printf("  %-20s %-10s %-20s %s%s\n", tag, tagType, date, commit, protected)
	}

	// Legend
	log.Info("")
	log.Info("  🔒 = protected tag")
}

// getTagType returns whether a tag is annotated or lightweight
func (a *TagListAction) getTagType(workDir, tag string) string {
	cmd := exec.Command("git", "cat-file", "-t", tag)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	tagType := strings.TrimSpace(string(output))
	if tagType == "tag" {
		return "annotated"
	}
	return "lightweight"
}

// getTagDate returns the date of the tag
func (a *TagListAction) getTagDate(workDir, tag string) string {
	cmd := exec.Command("git", "log", "-1", "--format=%ci", tag)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	date := strings.TrimSpace(string(output))
	// Return just the date part
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

// getTagCommit returns the short commit hash for the tag
func (a *TagListAction) getTagCommit(workDir, tag string) string {
	cmd := exec.Command("git", "rev-list", "-1", "--abbrev-commit", tag)
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	return strings.TrimSpace(string(output))
}

// isProtected checks if the tag matches any protected pattern
func (a *TagListAction) isProtected(tag string) bool {
	for _, pattern := range a.tagConfig.ProtectedTags {
		matched, err := filepath.Match(pattern, tag)
		if err == nil && matched {
			return true
		}
	}
	return false
}
