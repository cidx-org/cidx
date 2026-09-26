package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cucumber/godog"
)

// RegisterReleaseHousekeepingSteps registers the release housekeeping steps
// (#488, #489). The repository ones reuse the release-bump world: a real
// temporary repository driven by tc.bumpGit.
func RegisterReleaseHousekeepingSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a commit log holding "([^"]*)" and the commit that opened the PR branch$`, tc.commitLogWithInitCommit)
	ctx.When(`^the commit log is parsed$`, tc.commitLogIsParsed)
	ctx.Then(`^the parsed history holds "([^"]*)"$`, tc.parsedHistoryHolds)
	ctx.Then(`^the parsed history does not hold the commit that opened the PR branch$`, tc.parsedHistoryLacksInitCommit)

	ctx.Given(`^a repository whose release notes for "([^"]*)" are committed$`, tc.releaseNotesCommitted)
	ctx.Given(`^a repository whose release notes for "([^"]*)" were prepared but not committed$`, tc.releaseNotesPrepared)
	ctx.When(`^the prepared release files are cleaned up$`, tc.preparedReleaseFilesCleanedUp)
	ctx.Then(`^the release notes for "([^"]*)" still exist$`, tc.releaseNotesStillExist)
	ctx.Then(`^the release notes for "([^"]*)" no longer exist$`, tc.releaseNotesNoLongerExist)

	ctx.Given(`^the remote has the branch "([^"]*)"$`, tc.remoteHasBranch)
	ctx.When(`^the release in flight is looked up$`, tc.releaseInFlightLookedUp)
	ctx.Then(`^the release in flight is "([^"]*)"$`, tc.releaseInFlightIs)
	ctx.Then(`^no release is in flight$`, tc.noReleaseInFlight)
}

func (tc *TestContext) commitLogWithInitCommit(subject string) error {
	// Newest first, the order `git log` prints: the fix, then the empty commit
	// `pr create` opened the branch with — produced from the same constant.
	tc.Output = "1111111111111111|" + subject + "|<<<END>>>\n" +
		"2222222222222222|" + actions.InitCommitPrefix + "fix(release): undo a failed bump|<<<END>>>"
	return nil
}

func (tc *TestContext) commitLogIsParsed() error {
	var subjects []string
	for _, c := range actions.ParseCommitLog(tc.Output) {
		subjects = append(subjects, c.Subject)
	}
	tc.Config["parsed_subjects"] = strings.Join(subjects, "\n")
	return nil
}

func (tc *TestContext) parsedHistoryHolds(subject string) error {
	parsed, _ := tc.Config["parsed_subjects"].(string)
	if !strings.Contains(parsed, subject) {
		return fmt.Errorf("parsed history %q does not hold %q", parsed, subject)
	}
	return nil
}

func (tc *TestContext) parsedHistoryLacksInitCommit() error {
	parsed, _ := tc.Config["parsed_subjects"].(string)
	if strings.Contains(parsed, "initialize PR branch") {
		return fmt.Errorf("parsed history still holds the PR-branch commit: %q", parsed)
	}
	return nil
}

func (tc *TestContext) writeReleaseNotes(version string) (string, error) {
	if err := tc.repositoryWithNoVersionFile(); err != nil {
		return "", err
	}
	file := actions.GetReleaseNotesFile(version)
	path := filepath.Join(tc.bump().repo, file)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return file, os.WriteFile(path, []byte("# Release v"+version+"\n"), 0o644)
}

func (tc *TestContext) releaseNotesCommitted(version string) error {
	file, err := tc.writeReleaseNotes(version)
	if err != nil {
		return err
	}
	if _, err := tc.bumpGit("add", file); err != nil {
		return err
	}
	_, err = tc.bumpGit("commit", "-q", "-m", "chore: prepare release v"+version)
	return err
}

func (tc *TestContext) releaseNotesPrepared(version string) error {
	_, err := tc.writeReleaseNotes(version)
	tc.Config["notes_version"] = version
	return err
}

func (tc *TestContext) preparedReleaseFilesCleanedUp() error {
	version := "3.4.3"
	if v, ok := tc.Config["notes_version"].(string); ok {
		version = v
	}
	_, err := actions.CleanupPreparedNotes(tc.bump().repo, version)
	return err
}

func (tc *TestContext) notesExist(version string) bool {
	_, err := os.Stat(filepath.Join(tc.bump().repo, actions.GetReleaseNotesFile(version)))
	return err == nil
}

func (tc *TestContext) releaseNotesStillExist(version string) error {
	if !tc.notesExist(version) {
		return fmt.Errorf("the release notes for %s were deleted", version)
	}
	return nil
}

func (tc *TestContext) releaseNotesNoLongerExist(version string) error {
	if tc.notesExist(version) {
		return fmt.Errorf("the release notes for %s are still there", version)
	}
	return nil
}

// remoteHasBranch stages one line of `git ls-remote --heads origin` output.
func (tc *TestContext) remoteHasBranch(branch string) error {
	tc.Output += "0123456789abcdef0123456789abcdef01234567\trefs/heads/" + branch + "\n"
	return nil
}

func (tc *TestContext) releaseInFlightLookedUp() error {
	tc.Config["in_flight"] = actions.ReleaseBranchInFlight(tc.Output)
	return nil
}

func (tc *TestContext) releaseInFlightIs(want string) error {
	if got, _ := tc.Config["in_flight"].(string); got != want {
		return fmt.Errorf("release in flight %q, want %q", got, want)
	}
	return nil
}

func (tc *TestContext) noReleaseInFlight() error {
	if got, _ := tc.Config["in_flight"].(string); got != "" {
		return fmt.Errorf("%q was taken for a release in flight", got)
	}
	return nil
}
