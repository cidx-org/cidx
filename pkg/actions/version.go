package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/semver"
	log "github.com/sirupsen/logrus"
)

// VersionState reconciles the two places the release flow can read a "current
// version" from: the VERSION file (what commitizen bumps) and the latest git
// tag (what was actually released). They can disagree -- a tag cut outside the
// flow, or a bump commit that never got tagged -- and computing the next
// version from the wrong one yields a version below the latest tag (issue
// #185). The tag is the source of truth; a disagreement is reported, never
// silently resolved.
type VersionState struct {
	LastTag     string // "v2.1.3", empty when the repo has no tag
	TagVersion  string // "2.1.3", empty when there is no tag or it is not semver
	FileVersion string // VERSION file contents, empty when unreadable
}

// ResolveVersion reads both sources and returns their reconciliation.
func ResolveVersion(workDir string) VersionState {
	var state VersionState

	if version, err := readVersionFile(workDir); err == nil {
		state.FileVersion = version
	}

	if output, err := runGit(workDir, "describe", "--tags", "--abbrev=0"); err == nil {
		state.LastTag = strings.TrimSpace(string(output))
		if version := semver.Trim(state.LastTag); semver.IsValid(version) {
			state.TagVersion = version
		}
	}

	return state
}

// Current returns the version the release flow must compute from: the latest
// tag, falling back to the version files only when nothing was ever tagged.
func (s VersionState) Current() string {
	switch {
	case s.TagVersion != "":
		return s.TagVersion
	case s.FileVersion != "":
		return s.FileVersion
	default:
		return "0.0.0"
	}
}

// LastTagDisplay returns the latest tag, or "(none)" when the repo has none.
func (s VersionState) LastTagDisplay() string {
	if s.LastTag == "" {
		return "(none)"
	}
	return s.LastTag
}

// Diverged reports whether the version files disagree with the latest tag.
func (s VersionState) Diverged() bool { return s.compare() != 0 }

// FilesAhead reports a version bump that landed in the files but was never
// tagged -- the state `cidx release tag create` exists to close.
func (s VersionState) FilesAhead() bool { return s.compare() > 0 }

// compare returns -1 when the version files trail the tag, +1 when they run
// ahead of it, and 0 when they agree or cannot be compared.
func (s VersionState) compare() int {
	if s.TagVersion == "" || s.FileVersion == "" {
		return 0
	}
	return semver.Compare(s.FileVersion, s.TagVersion)
}

// DivergenceError names both values and the way out, or nil when they agree.
func (s VersionState) DivergenceError() error {
	switch s.compare() {
	case -1:
		return fmt.Errorf("version files trail the latest tag: VERSION says %s but the latest tag is %s\n"+
			"   The tag is the released version, so bumping from %s would produce a version below %s\n"+
			"   Fix: set VERSION and .cz.toml to %s, commit, then run the release again",
			s.FileVersion, s.LastTag, s.FileVersion, s.LastTag, s.TagVersion)
	case 1:
		return fmt.Errorf("version files run ahead of the latest tag: VERSION says %s but the latest tag is %s\n"+
			"   A version bump already landed and was never tagged\n"+
			"   Fix: tag it with 'cidx release tag prepare && cidx release tag create', then run the release again",
			s.FileVersion, s.LastTag)
	}
	return nil
}

// logDivergence reports the disagreement one line per entry: logrus escapes
// embedded newlines into a single unreadable field.
func logDivergence(err error) {
	if err == nil {
		return
	}
	lines := strings.Split(err.Error(), "\n")
	log.Warnf("⚠️  %s", lines[0])
	for _, line := range lines[1:] {
		log.Info(line)
	}
}

// CommitCounts is the conventional-commit breakdown of a commit range.
type CommitCounts struct {
	Breaking int
	Feat     int
	Fix      int
	Other    int
}

// Total returns the number of commits in the range.
func (c CommitCounts) Total() int { return c.Breaking + c.Feat + c.Fix + c.Other }

// countCommits classifies the commits in lastTag..HEAD (all of them when
// lastTag is empty).
//
// Records are separated explicitly because commit bodies span several lines:
// counting output lines counted every body line as a commit, which is where
// "Other: 131" for a 9-commit range came from (issue #185).
func countCommits(workDir, lastTag string) CommitCounts {
	const format = "--pretty=format:%s%x00%b%x1e"

	args := []string{"log", format}
	if lastTag != "" {
		args = []string{"log", lastTag + "..HEAD", format}
	}

	output, err := runGit(workDir, args...)
	if err != nil {
		return CommitCounts{}
	}

	var commits []CommitInfo
	for _, record := range strings.Split(string(output), "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}
		subject, body, _ := strings.Cut(record, "\x00")
		commits = append(commits, ParseCommit(subject, body))
	}

	return CountCommitInfos(commits)
}

// CountCommitInfos is countCommits for callers that already parsed the range.
func CountCommitInfos(commits []CommitInfo) CommitCounts {
	var counts CommitCounts
	for _, c := range commits {
		switch {
		case c.Breaking:
			counts.Breaking++
		case c.Type == "feat":
			counts.Feat++
		case c.Type == "fix":
			counts.Fix++
		default:
			counts.Other++
		}
	}
	return counts
}

// NextVersion applies the semantic bump implied by counts to current.
func NextVersion(current string, counts CommitCounts) string {
	major, minor, patch, _ := semver.Parse(current)

	switch {
	case counts.Breaking > 0:
		major++
		minor, patch = 0, 0
	case counts.Feat > 0:
		minor++
		patch = 0
	default:
		patch++
	}

	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}

// SuggestTagVersion returns the version the tag flow should create: the
// pending one when a bump already landed untagged, otherwise the semantic bump
// of the latest tag.
func SuggestTagVersion(workDir string) string {
	state := ResolveVersion(workDir)
	if state.FilesAhead() {
		return state.FileVersion
	}
	return NextVersion(state.Current(), countCommits(workDir, state.LastTag))
}

// warnChangelogTagGap reports a CHANGELOG.md whose newest released section has
// no matching tag. commitizen refuses to bump in that state ("No tag found to
// do an incremental changelog", exit 16), so surfacing it before the run beats
// discovering it half-way through. It returns true when a gap was reported.
func warnChangelogTagGap(workDir string) bool {
	version := changelogTagGap(workDir)
	if version == "" {
		return false
	}

	log.Warnf("⚠️  CHANGELOG.md's newest section is %s but no matching tag exists", version)
	log.Info("   cz bump will fail with 'No tag found to do an incremental changelog' (exit 16)")
	log.Info("   Fix: tag that version (cidx release tag prepare && cidx release tag create)")
	log.Info("        or drop the section from CHANGELOG.md")
	return true
}

// changelogTagGap returns the newest released version in CHANGELOG.md when no
// tag matches it, and "" when they agree or there is nothing to check.
func changelogTagGap(workDir string) string {
	content, err := os.ReadFile(filepath.Join(workDir, "CHANGELOG.md"))
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}

		// "## v2.1.3 (2026-07-27)" and "## [2.1.3] - 2026-07-27" both yield
		// "2.1.3"; "## [Unreleased]" yields nothing and is skipped.
		fields := strings.Fields(strings.TrimPrefix(line, "## "))
		if len(fields) == 0 {
			continue
		}
		version := semver.Trim(strings.Trim(fields[0], "[]"))
		if !semver.IsValid(version) {
			continue
		}

		if versionIsTagged(workDir, version) {
			return ""
		}
		return version
	}

	return ""
}

// versionIsTagged reports whether the version is tagged, with or without a prefix.
func versionIsTagged(workDir, version string) bool {
	for _, candidate := range []string{"v" + version, version} {
		if _, err := runGit(workDir, "rev-parse", "-q", "--verify", "refs/tags/"+candidate); err == nil {
			return true
		}
	}
	return false
}

// readVersionFile reads the VERSION file.
func readVersionFile(workDir string) (string, error) {
	content, err := os.ReadFile(filepath.Join(workDir, "VERSION"))
	if err != nil {
		return "", fmt.Errorf("failed to read VERSION file: %w", err)
	}
	return strings.TrimSpace(string(content)), nil
}
