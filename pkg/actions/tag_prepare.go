package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// TagPrepareAction prepares a tag for human review before creation
type TagPrepareAction struct {
	repo      *vcs.Repository
	tagConfig config.TagConfig
	dryRun    bool
}

// TagVersionFile is the path where the target tag version is stored
const TagVersionFile = ".cidx/tag-version"

// TagMessageFile is the path where the tag message is stored
const TagMessageFile = ".cidx/tag-message"

// NewTagPrepare creates a new tag prepare action
func NewTagPrepare(repo *vcs.Repository, tagConfig config.TagConfig, dryRun bool) *TagPrepareAction {
	return &TagPrepareAction{
		repo:      repo,
		tagConfig: tagConfig,
		dryRun:    dryRun,
	}
}

// Execute prepares a tag version and message for review
func (a *TagPrepareAction) Execute(ctx context.Context) error {
	log.Info("🏷️  Preparing tag...")

	workDir, err := a.repo.GetWorkDir()
	if err != nil {
		return fmt.Errorf("failed to get work directory: %w", err)
	}

	// 1. Get current version and last tag
	currentVersion, _ := readVersionFile(workDir)
	lastTag := a.getLastTag()
	log.Infof("   Current VERSION: %s", currentVersion)
	log.Infof("   Last tag: %s", lastTag)

	// 2. Determine next version
	var nextVersion string
	if a.tagConfig.UseCommitizen {
		nextVersion, err = a.getCommitizenVersion()
		if err != nil {
			log.Warnf("   Commitizen failed, using VERSION file: %v", err)
			nextVersion = currentVersion
		} else {
			log.Infof("   Commitizen suggests: %s", nextVersion)
		}
	} else {
		nextVersion = a.suggestNextVersion(lastTag)
		log.Infof("   Auto-increment suggests: %s", nextVersion)
	}

	// 3. Generate tag message
	message := a.generateTagMessage(nextVersion)

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would prepare:")
		fmt.Printf("\nVersion: %s\n", nextVersion)
		fmt.Printf("Tag: %s\n", a.tagConfig.FormatTag(nextVersion))
		fmt.Printf("\nMessage:\n%s\n", message)
		return nil
	}

	// 4. Save to files
	if err := SavePreparedTagVersion(workDir, nextVersion); err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}
	log.Infof("✓ Target version saved to %s", TagVersionFile)

	if err := SaveTagMessage(workDir, message); err != nil {
		return fmt.Errorf("failed to save message: %w", err)
	}
	log.Infof("✓ Tag message saved to %s", TagMessageFile)

	// 5. Open editor for message
	if err := a.openEditor(workDir); err != nil {
		log.Warnf("Could not open editor: %v", err)
		log.Infof("📝 Please edit %s and %s manually", TagVersionFile, TagMessageFile)
	}

	log.Info("")
	log.Info("📌 Next steps:")
	log.Infof("   1. Review and edit %s (version) and %s (message)", TagVersionFile, TagMessageFile)
	log.Info("   2. Run: cidx action tag preview")
	log.Info("   3. Run: cidx action tag create")

	return nil
}

// getLastTag returns the most recent tag
func (a *TagPrepareAction) getLastTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "(none)"
	}
	return strings.TrimSpace(string(output))
}

// getCommitizenVersion uses commitizen to determine the next version
func (a *TagPrepareAction) getCommitizenVersion() (string, error) {
	// Check if cz is available
	if _, err := exec.LookPath("cz"); err != nil {
		return "", fmt.Errorf("commitizen (cz) not found in PATH")
	}

	cmd := exec.Command("cz", "bump", "--dry-run", "--yes")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cz bump failed: %w\n%s", err, output)
	}

	// Parse output for version
	// Output format: "bump: version X.Y.Z -> X.Y.Z+1"
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "→") || strings.Contains(line, "->") {
			parts := strings.Split(line, "→")
			if len(parts) < 2 {
				parts = strings.Split(line, "->")
			}
			if len(parts) >= 2 {
				version := strings.TrimSpace(parts[1])
				// Remove any trailing text
				version = strings.Fields(version)[0]
				return version, nil
			}
		}
	}

	return "", fmt.Errorf("could not parse cz output: %s", output)
}

// suggestNextVersion increments patch version from last tag
func (a *TagPrepareAction) suggestNextVersion(lastTag string) string {
	// Strip prefix
	version := strings.TrimPrefix(lastTag, a.tagConfig.Prefix)
	if version == "" || version == "(none)" {
		return "0.1.0"
	}

	var major, minor, patch int
	_, _ = fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch)

	// Default to patch bump
	patch++
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// generateTagMessage creates a default tag message
func (a *TagPrepareAction) generateTagMessage(version string) string {
	tag := a.tagConfig.FormatTag(version)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Release %s\n\n", tag))

	// Get commit summary since last tag
	lastTag := a.getLastTag()
	if lastTag != "(none)" {
		commits := a.getCommitSummary(lastTag)
		if commits != "" {
			sb.WriteString("Changes:\n")
			sb.WriteString(commits)
		}
	}

	return sb.String()
}

// getCommitSummary returns a brief summary of commits since tag
func (a *TagPrepareAction) getCommitSummary(tag string) string {
	cmd := exec.Command("git", "log", tag+"..HEAD", "--oneline", "--no-merges")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 10 {
		lines = lines[:10]
		lines = append(lines, fmt.Sprintf("... and %d more commits", len(lines)-10))
	}

	var sb strings.Builder
	for _, line := range lines {
		if line != "" {
			sb.WriteString(fmt.Sprintf("- %s\n", line))
		}
	}
	return sb.String()
}

// openEditor opens the tag message file in editor
func (a *TagPrepareAction) openEditor(workDir string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"vim", "nano", "vi", "code", "notepad"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}

	if editor == "" {
		return fmt.Errorf("no editor found")
	}

	// Open both files if editor supports it
	versionPath := filepath.Join(workDir, TagVersionFile)
	messagePath := filepath.Join(workDir, TagMessageFile)

	cmd := exec.Command(editor, versionPath, messagePath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// SavePreparedTagVersion saves the target version to a file
func SavePreparedTagVersion(workDir, version string) error {
	dir := filepath.Join(workDir, ".cidx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(workDir, TagVersionFile)
	return os.WriteFile(path, []byte(version+"\n"), 0644)
}

// LoadPreparedTagVersion loads the target version from file
func LoadPreparedTagVersion(workDir string) (string, error) {
	path := filepath.Join(workDir, TagVersionFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// HasPreparedTagVersion checks if a tag version has been prepared
func HasPreparedTagVersion(workDir string) bool {
	path := filepath.Join(workDir, TagVersionFile)
	_, err := os.Stat(path)
	return err == nil
}

// CleanupPreparedTagVersion removes the version file after successful tag creation
func CleanupPreparedTagVersion(workDir string) error {
	path := filepath.Join(workDir, TagVersionFile)
	return os.Remove(path)
}

// SaveTagMessage saves the tag message to a file
func SaveTagMessage(workDir, message string) error {
	dir := filepath.Join(workDir, ".cidx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(workDir, TagMessageFile)
	return os.WriteFile(path, []byte(message), 0644)
}

// LoadTagMessage loads the tag message from file
func LoadTagMessage(workDir string) (string, error) {
	path := filepath.Join(workDir, TagMessageFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// HasTagMessage checks if a tag message has been prepared
func HasTagMessage(workDir string) bool {
	path := filepath.Join(workDir, TagMessageFile)
	_, err := os.Stat(path)
	return err == nil
}

// CleanupTagMessage removes the message file after successful tag creation
func CleanupTagMessage(workDir string) error {
	path := filepath.Join(workDir, TagMessageFile)
	return os.Remove(path)
}
