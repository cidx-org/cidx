package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
	log "github.com/sirupsen/logrus"
)

// ReleasePrepareAction prepares release notes for human review.
//
// It carries no remote provider: every step reads the local repository (git
// log, tags, version files) and the merged-PR list comes from the gh CLI, so
// preparing release notes must work in a repository without a remote --
// building the provider up front made it fail before any of that ran
// (issue #227).
type ReleasePrepareAction struct {
	repo          *vcs.Repository
	releaseConfig config.ReleaseConfig
	dryRun        bool
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
	Hash     string
	Type     string
	Scope    string
	Subject  string
	Body     string
	PR       int
	Breaking bool
}

// CommitLogFormat is the git log format ParseCommitLog reads.
const CommitLogFormat = "--pretty=format:%H|%s|%b<<<END>>>"

// conventionalRe matches a conventional-commit subject "type(scope)!: text".
// The optional "!" is what issue #175 was about: the regex used to stop at the
// colon, so "feat!: drop X" matched nothing, was typed "other" and never
// counted as breaking.
var conventionalRe = regexp.MustCompile(`^(\w+)(?:\(([^)]+)\))?(!)?:\s*(.+)$`)

// prRe matches a "(#123)" marker. The one a squash merge appends is the last
// of the subject, not the first: a title is free to cite an issue or another
// PR in its text, and GitHub adds its number after it (issue #376).
var prRe = regexp.MustCompile(`\(#(\d+)\)`)

// ParseCommit is the single conventional-commit parser of the release flow:
// prepare, preview, tag and the TUI all classify commits through it, so they
// cannot disagree on a version bump. Subjects that are not conventional keep
// type "other" and their raw text.
func ParseCommit(subject, body string) CommitInfo {
	commit := CommitInfo{
		Type:     "other",
		Subject:  subject,
		Body:     body,
		Breaking: strings.Contains(body, "BREAKING CHANGE"),
	}

	if m := conventionalRe.FindStringSubmatch(subject); m != nil {
		commit.Type, commit.Scope, commit.Subject = m[1], m[2], m[4]
		commit.Breaking = commit.Breaking || m[3] == "!"
	}

	if ms := prRe.FindAllStringSubmatch(subject, -1); ms != nil {
		_, _ = fmt.Sscanf(ms[len(ms)-1][1], "%d", &commit.PR)
		// The marker is the squash's only when it is the tail, and then it is
		// taken out of the subject: the notes print the number themselves, so
		// leaving it in named every PR twice — "(#375) (#375)" on all 39 lines
		// of the v3.0.0 dry-run (issue #376). A number cited mid-sentence is
		// part of the text and stays.
		commit.Subject = strings.TrimSpace(strings.TrimSuffix(commit.Subject, fmt.Sprintf("(#%d)", commit.PR)))
	}

	return commit
}

// ParseCommitLog parses `git log CommitLogFormat` output into commits.
func ParseCommitLog(output string) []CommitInfo {
	var commits []CommitInfo

	for _, entry := range strings.Split(output, "<<<END>>>") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.SplitN(entry, "|", 3)
		if len(parts) < 2 {
			continue
		}

		body := ""
		if len(parts) > 2 {
			body = parts[2]
		}

		commit := ParseCommit(parts[1], body)
		commit.Hash = parts[0][:min(8, len(parts[0]))]
		commits = append(commits, commit)
	}

	return commits
}

// NewReleasePrepare creates a new release prepare action
func NewReleasePrepare(repo *vcs.Repository, releaseConfig config.ReleaseConfig, dryRun bool) *ReleasePrepareAction {
	return &ReleasePrepareAction{
		repo:          repo,
		releaseConfig: releaseConfig,
		dryRun:        dryRun,
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

	// 4. Determine next version from the latest tag, not the version files
	workDir, _ := a.repo.GetWorkDir()
	state := ResolveVersion(workDir)
	logDivergence(state.DivergenceError())
	currentVersion := state.Current()
	nextVersion := NextVersion(currentVersion, CountCommitInfos(commits))
	log.Infof("   Suggested version: %s → %s", currentVersion, nextVersion)

	// 5. Generate release notes
	notes := a.generateReleaseNotes(nextVersion, commits, prs)

	if a.dryRun {
		log.Info("🏁 Dry-run mode: would generate these release notes:")
		fmt.Println("\n" + notes)
		return nil
	}

	// 6. Save to files
	notesFile := GetReleaseNotesFile(nextVersion)
	if err := a.saveReleaseNotes(notes, nextVersion); err != nil {
		return fmt.Errorf("failed to save release notes: %w", err)
	}
	log.Infof("✓ Release notes saved to %s", notesFile)

	if err := SavePreparedVersion(workDir, nextVersion); err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}
	log.Infof("✓ Target version saved to %s", ReleaseVersionFile)

	// 7. Open editor for review (skipped when there is no terminal)
	if err := openInEditor(a.releaseConfig.GetEditor(), filepath.Join(workDir, notesFile)); err != nil {
		log.Warnf("Could not open editor: %v", err)
		log.Infof("📝 Please edit %s manually", notesFile)
	}

	log.Info("")
	log.Info("📌 Next steps:")
	log.Infof("   1. Review and edit %s and %s", notesFile, ReleaseVersionFile)
	log.Info("   2. Run: cidx release commit")
	log.Info("   3. Run: cidx release preview")
	log.Info("   4. Run: cidx release create")

	return nil
}

// getLastTag returns the most recent semver tag
func (a *ReleasePrepareAction) getLastTag() (string, error) {
	cmd := vcs.Git("", "describe", "--tags", "--abbrev=0")
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
	args := []string{"log", CommitLogFormat}
	if tag != "" {
		args = []string{"log", tag + "..HEAD", CommitLogFormat}
	}

	cmd := vcs.Git("", args...)
	workDir, _ := a.repo.GetWorkDir()
	cmd.Dir = workDir

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return ParseCommitLog(string(output)), nil
}

// getMergedPRsSince returns PRs merged since a tag
func (a *ReleasePrepareAction) getMergedPRsSince(ctx context.Context, tag string) ([]PRInfo, error) {
	workDir, _ := a.repo.GetWorkDir()

	// Get tag date
	var since time.Time
	if tag != "" {
		cmd := vcs.Git(workDir, "log", "-1", "--format=%ci", tag)

		output, err := cmd.Output()
		if err == nil {
			since, _ = time.Parse("2006-01-02 15:04:05 -0700", strings.TrimSpace(string(output)))
		}
	}

	// Use gh CLI to get merged PRs (more reliable than API for this)
	args := []string{"pr", "list", "--state", "merged", "--json", "number,title,author,mergedAt", "--limit", "100"}

	output, err := ghPRList(workDir, args).Output()
	if err != nil {
		return nil, ghCommandError(args, err)
	}

	return a.parsePRList(string(output), since)
}

// ghPRList builds the `gh pr list` invocation, rooted in the repository being
// released. gh resolves which repository to query from its working directory,
// so without Dir it listed the PRs of wherever the cidx process happened to be
// started -- invisible when releasing the current directory, wrong the moment
// the repository is elsewhere. Every other command here already sets Dir.
func ghPRList(workDir string, args []string) *exec.Cmd {
	cmd := exec.Command("gh", args...)
	cmd.Dir = workDir
	return cmd
}

// ghCommandError names the command that failed and quotes what it printed on
// stderr. Bare "exit status 1" said neither which tool broke nor why -- not
// logged in, no remote, rate limited all looked identical (issue #227).
func ghCommandError(args []string, err error) error {
	invocation := "gh " + strings.Join(args, " ")

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if stderr := strings.TrimSpace(string(exitErr.Stderr)); stderr != "" {
			return fmt.Errorf("%s: %w: %s", invocation, err, stderr)
		}
	}

	return fmt.Errorf("%s: %w", invocation, err)
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

// generateReleaseNotes creates markdown release notes
func (a *ReleasePrepareAction) generateReleaseNotes(version string, commits []CommitInfo, prs []PRInfo) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Release v%s\n\n", version)
	fmt.Fprintf(&sb, "<!-- Generated by cidx on %s -->\n", time.Now().Format("2006-01-02 15:04"))
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

		fmt.Fprintf(&sb, "## %s\n\n", headers[typ])

		for _, c := range groups[typ] {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s:** ", c.Scope)
			}

			prLink := ""
			if c.PR > 0 {
				prLink = fmt.Sprintf(" (#%d)", c.PR)
			}

			fmt.Fprintf(&sb, "- %s%s%s\n", scope, c.Subject, prLink)
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
				fmt.Fprintf(&sb, "- @%s\n", author)
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
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
