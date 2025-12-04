package actions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleasePrepareAction prepares release notes for human review
type ReleasePrepareAction struct {
	repo     *vcs.Repository
	provider remote.Provider
	dryRun   bool
}

// ReleaseNotesFilePattern is the pattern for release notes files
const ReleaseNotesFilePattern = ".cidx/release-notes-v%s.md"

// ReleaseVersionFile is the path where the target version is stored
const ReleaseVersionFile = ".cidx/release-version"

// GetReleaseNotesFile returns the path for release notes with the given version
func GetReleaseNotesFile(version string) string {
	return fmt.Sprintf(ReleaseNotesFilePattern, version)
}

// CommitInfo holds parsed commit information
type CommitInfo struct {
	Hash    string
	Type    string
	Scope   string
	Subject string
	Body    string
	PR      int
}

// NewReleasePrepare creates a new release prepare action
func NewReleasePrepare(repo *vcs.Repository, provider remote.Provider, dryRun bool) *ReleasePrepareAction {
	return &ReleasePrepareAction{
		repo:     repo,
		provider: provider,
		dryRun:   dryRun,
	}
}

// Execute prepares release notes and opens editor for review
func (a *ReleasePrepareAction) Execute(ctx context.Context) error {
	log.Info("📋 Preparing release notes...")

	// 1. Get last tag
	lastTag, err := a.getLastTag()
	if err != nil {
		log.Warnf("No previous tag found, will include all commits")
		lastTag = ""
	} else {
		log.Infof("   Last release: %s", lastTag)
	}

	// 2. Get commits since last tag
	commits, err := a.getCommitsSince(lastTag)
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}
	log.Infof("   Found %d commits since %s", len(commits), lastTag)

	// 3. Get merged PRs since last tag
	prs, err := a.getMergedPRsSince(ctx, lastTag)
	if err != nil {
		log.Warnf("Could not fetch PRs: %v", err)
	} else {
		log.Infof("   Found %d merged PRs", len(prs))
	}

	// 4. Determine next version
	currentVersion, _ := a.readVersionFile()
	nextVersion := a.suggestNextVersion(currentVersion, commits)
	log.Infof("   Suggested version: %s → %s", currentVersion, nextVersion)

	// 5. Generate release notes
	notes := a.generateReleaseNotes(nextVersion, commits, prs)

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would generate these release notes:")
		fmt.Println("\n" + notes)
		return nil
	}

	// 6. Save to files
	workDir, _ := a.repo.GetWorkDir()
	notesFile := GetReleaseNotesFile(nextVersion)
	if err := a.saveReleaseNotes(notes, nextVersion); err != nil {
		return fmt.Errorf("failed to save release notes: %w", err)
	}
	log.Infof("✓ Release notes saved to %s", notesFile)

	if err := SavePreparedVersion(workDir, nextVersion); err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}
	log.Infof("✓ Target version saved to %s", ReleaseVersionFile)

	// 7. Open editor
	if err := a.openEditor(nextVersion); err != nil {
		log.Warnf("Could not open editor: %v", err)
		log.Infof("📝 Please edit %s manually", notesFile)
	}

	log.Info("")
	log.Info("📌 Next steps:")
	log.Infof("   1. Review and edit %s and %s", notesFile, ReleaseVersionFile)
	log.Info("   2. Run: cidx action release commit")
	log.Info("   3. Run: cidx action release preview")
	log.Info("   4. Run: cidx action release create")

	return nil
}

// getLastTag returns the most recent semver tag
func (a *ReleasePrepareAction) getLastTag() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

// getCommitsSince returns commits since a tag (or all commits if tag is empty)
func (a *ReleasePrepareAction) getCommitsSince(tag string) ([]CommitInfo, error) {
	var args []string
	if tag != "" {
		args = []string{"log", tag + "..HEAD", "--pretty=format:%H|%s|%b<<<END>>>"}
	} else {
		args = []string{"log", "--pretty=format:%H|%s|%b<<<END>>>"}
	}

	cmd := exec.Command("git", args...)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return a.parseCommits(string(output)), nil
}

// parseCommits parses git log output into CommitInfo structs
func (a *ReleasePrepareAction) parseCommits(output string) []CommitInfo {
	var commits []CommitInfo

	// Split by our delimiter
	entries := strings.Split(output, "<<<END>>>")

	// Regex for conventional commit format: type(scope): subject
	conventionalRe := regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?:\s*(.+)$`)
	// Regex for PR number in commit: (#123)
	prRe := regexp.MustCompile(`\(#(\d+)\)`)

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, "|", 3)
		if len(parts) < 2 {
			continue
		}

		hash := parts[0]
		subject := parts[1]
		body := ""
		if len(parts) > 2 {
			body = parts[2]
		}

		commit := CommitInfo{
			Hash:    hash[:8], // Short hash
			Subject: subject,
			Body:    body,
			Type:    "other",
		}

		// Parse conventional commit
		if matches := conventionalRe.FindStringSubmatch(subject); matches != nil {
			commit.Type = matches[1]
			commit.Scope = matches[2]
			commit.Subject = matches[3]
		}

		// Extract PR number
		if matches := prRe.FindStringSubmatch(subject); matches != nil {
			_, _ = fmt.Sscanf(matches[1], "%d", &commit.PR)
		}

		commits = append(commits, commit)
	}

	return commits
}

// getMergedPRsSince returns PRs merged since a tag
func (a *ReleasePrepareAction) getMergedPRsSince(ctx context.Context, tag string) ([]PRInfo, error) {
	// Get tag date
	var since time.Time
	if tag != "" {
		cmd := exec.Command("git", "log", "-1", "--format=%ci", tag)
		workDir, _ := a.repo.GetWorkDir()
		cmd.Dir = workDir

		output, err := cmd.Output()
		if err == nil {
			since, _ = time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(output)))
		}
	}

	// Use gh CLI to get merged PRs (more reliable than API for this)
	args := []string{"pr", "list", "--state", "merged", "--json", "number,title,author,mergedAt", "--limit", "100"}

	cmd := exec.Command("gh", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return a.parsePRList(string(output), since)
}

// PRInfo holds PR information
type PRInfo struct {
	Number int
	Title  string
	Author string
}

// parsePRList parses gh pr list JSON output
func (a *ReleasePrepareAction) parsePRList(output string, since time.Time) ([]PRInfo, error) {
	// Simple JSON parsing for our specific format
	var prs []PRInfo

	// Use gh CLI output format parsing
	// Format: [{"number":123,"title":"...","author":{"login":"..."},"mergedAt":"..."}]

	// For simplicity, use a regex-based approach
	numberRe := regexp.MustCompile(`"number":(\d+)`)
	titleRe := regexp.MustCompile(`"title":"([^"]*)"`)
	authorRe := regexp.MustCompile(`"login":"([^"]*)"`)
	mergedRe := regexp.MustCompile(`"mergedAt":"([^"]*)"`)

	numbers := numberRe.FindAllStringSubmatch(output, -1)
	titles := titleRe.FindAllStringSubmatch(output, -1)
	authors := authorRe.FindAllStringSubmatch(output, -1)
	merged := mergedRe.FindAllStringSubmatch(output, -1)

	for i := 0; i < len(numbers) && i < len(titles); i++ {
		var num int
		_, _ = fmt.Sscanf(numbers[i][1], "%d", &num)

		// Check if merged after since date
		if i < len(merged) && !since.IsZero() {
			mergedAt, err := time.Parse(time.RFC3339, merged[i][1])
			if err == nil && mergedAt.Before(since) {
				continue
			}
		}

		author := ""
		if i < len(authors) {
			author = authors[i][1]
		}

		prs = append(prs, PRInfo{
			Number: num,
			Title:  titles[i][1],
			Author: author,
		})
	}

	return prs, nil
}

// suggestNextVersion analyzes commits to suggest version bump
func (a *ReleasePrepareAction) suggestNextVersion(current string, commits []CommitInfo) string {
	// Parse current version
	var major, minor, patch int
	_, _ = fmt.Sscanf(current, "%d.%d.%d", &major, &minor, &patch)

	// Determine bump type
	hasBreaking := false
	hasFeature := false

	for _, c := range commits {
		if strings.Contains(c.Body, "BREAKING CHANGE") || strings.HasSuffix(c.Type, "!") {
			hasBreaking = true
		}
		if c.Type == "feat" {
			hasFeature = true
		}
	}

	if hasBreaking {
		major++
		minor = 0
		patch = 0
	} else if hasFeature {
		minor++
		patch = 0
	} else {
		patch++
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// generateReleaseNotes creates markdown release notes
func (a *ReleasePrepareAction) generateReleaseNotes(version string, commits []CommitInfo, prs []PRInfo) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Release v%s\n\n", version))
	sb.WriteString(fmt.Sprintf("<!-- Generated by cidx on %s -->\n", time.Now().Format("2006-01-02 15:04")))
	sb.WriteString("<!-- Edit this file to customize your release notes -->\n\n")

	// Group commits by type
	groups := map[string][]CommitInfo{
		"feat":     {},
		"fix":      {},
		"refactor": {},
		"docs":     {},
		"test":     {},
		"chore":    {},
		"other":    {},
	}

	for _, c := range commits {
		if _, ok := groups[c.Type]; ok {
			groups[c.Type] = append(groups[c.Type], c)
		} else {
			groups["other"] = append(groups["other"], c)
		}
	}

	// Section headers
	headers := map[string]string{
		"feat":     "✨ Features",
		"fix":      "🐛 Bug Fixes",
		"refactor": "♻️ Refactoring",
		"docs":     "📚 Documentation",
		"test":     "🧪 Tests",
		"chore":    "🔧 Maintenance",
		"other":    "📦 Other Changes",
	}

	// Write sections in order
	order := []string{"feat", "fix", "refactor", "docs", "test", "chore", "other"}

	for _, typ := range order {
		if len(groups[typ]) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("## %s\n\n", headers[typ]))

		for _, c := range groups[typ] {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s:** ", c.Scope)
			}

			prLink := ""
			if c.PR > 0 {
				prLink = fmt.Sprintf(" (#%d)", c.PR)
			}

			sb.WriteString(fmt.Sprintf("- %s%s%s\n", scope, c.Subject, prLink))
		}

		sb.WriteString("\n")
	}

	// Add PR contributors section if we have PR data
	if len(prs) > 0 {
		// Collect unique authors
		authors := make(map[string]bool)
		for _, pr := range prs {
			if pr.Author != "" && pr.Author != "github-actions[bot]" {
				authors[pr.Author] = true
			}
		}

		if len(authors) > 0 {
			sb.WriteString("## 👥 Contributors\n\n")
			var authorList []string
			for author := range authors {
				authorList = append(authorList, author)
			}
			sort.Strings(authorList)

			for _, author := range authorList {
				sb.WriteString(fmt.Sprintf("- @%s\n", author))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// readVersionFile reads current version from VERSION file
func (a *ReleasePrepareAction) readVersionFile() (string, error) {
	workDir, _ := a.repo.GetWorkDir()
	content, err := os.ReadFile(filepath.Join(workDir, "VERSION"))
	if err != nil {
		return "0.0.0", err
	}
	return strings.TrimSpace(string(content)), nil
}

// saveReleaseNotes saves notes to the release notes file with version in filename
func (a *ReleasePrepareAction) saveReleaseNotes(notes, version string) error {
	workDir, _ := a.repo.GetWorkDir()
	dir := filepath.Join(workDir, ".cidx")

	// Create .cidx directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	path := filepath.Join(workDir, GetReleaseNotesFile(version))
	return os.WriteFile(path, []byte(notes), 0644)
}

// openEditor opens the release notes in the user's editor
func (a *ReleasePrepareAction) openEditor(version string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		// Try common editors
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

	workDir, _ := a.repo.GetWorkDir()
	path := filepath.Join(workDir, GetReleaseNotesFile(version))

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// LoadPreparedNotes loads previously prepared release notes for the given version
func LoadPreparedNotes(workDir, version string) (string, error) {
	path := filepath.Join(workDir, GetReleaseNotesFile(version))
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// HasPreparedNotes checks if release notes have been prepared for the given version
func HasPreparedNotes(workDir, version string) bool {
	path := filepath.Join(workDir, GetReleaseNotesFile(version))
	_, err := os.Stat(path)
	return err == nil
}

// CleanupPreparedNotes removes the release notes file after successful release
func CleanupPreparedNotes(workDir, version string) error {
	path := filepath.Join(workDir, GetReleaseNotesFile(version))
	return os.Remove(path)
}

// SavePreparedVersion saves the target version to a file
func SavePreparedVersion(workDir, version string) error {
	dir := filepath.Join(workDir, ".cidx")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(workDir, ReleaseVersionFile)
	return os.WriteFile(path, []byte(version+"\n"), 0644)
}

// LoadPreparedVersion loads the target version from file
func LoadPreparedVersion(workDir string) (string, error) {
	path := filepath.Join(workDir, ReleaseVersionFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(content)), nil
}

// HasPreparedVersion checks if a version has been prepared
func HasPreparedVersion(workDir string) bool {
	path := filepath.Join(workDir, ReleaseVersionFile)
	_, err := os.Stat(path)
	return err == nil
}

// CleanupPreparedVersion removes the version file after successful release
func CleanupPreparedVersion(workDir string) error {
	path := filepath.Join(workDir, ReleaseVersionFile)
	return os.Remove(path)
}
