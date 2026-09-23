package features

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
	"github.com/cucumber/godog"
)

// bumpState is one scenario's release-bump world: the identities in play, the
// resolved environment, and the real repository the version and undo steps
// run against.
type bumpState struct {
	hostName, hostEmail string
	declared            map[string]string
	env                 map[string]string
	envErr              error

	repo    string
	baseSHA string
	version string
	readErr error
	undoErr error
}

// RegisterReleaseBumpSteps registers the release bump steps (#484).
func RegisterReleaseBumpSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the host git identity is "([^"]*)" <"([^"]*)">$`, tc.hostGitIdentityIs)
	ctx.Given(`^the host has no git identity$`, tc.hostHasNoGitIdentity)
	ctx.Given(`^the release action declares no git identity$`, tc.actionDeclaresNoIdentity)
	ctx.Given(`^the release action declares "([^"]*)" as "([^"]*)"$`, tc.actionDeclares)
	ctx.When(`^the bump environment is resolved$`, tc.bumpEnvironmentIsResolved)
	ctx.Then(`^the bump environment sets "([^"]*)" to "([^"]*)"$`, tc.bumpEnvironmentSets)
	ctx.Then(`^resolving the bump environment fails mentioning "([^"]*)"$`, tc.bumpEnvironmentFailsMentioning)

	ctx.Given(`^a repository with no VERSION file$`, tc.repositoryWithNoVersionFile)
	ctx.Given(`^a bump commit tagged "([^"]*)"$`, tc.bumpCommitTagged)
	ctx.Given(`^the bump created no commit$`, tc.bumpCreatedNoCommit)
	ctx.Given(`^the bump modified tracked files without committing$`, tc.bumpModifiedWithoutCommitting)
	ctx.Given(`^the starting commit is tagged "([^"]*)"$`, tc.startingCommitTagged)
	ctx.When(`^the bumped version is read$`, tc.bumpedVersionIsRead)
	ctx.Then(`^the bumped version is "([^"]*)"$`, tc.bumpedVersionIs)
	ctx.Then(`^reading the bumped version fails mentioning "([^"]*)"$`, tc.readingBumpedVersionFailsMentioning)
	ctx.When(`^the bump is undone$`, tc.bumpIsUndone)
	ctx.Then(`^the branch is back where it started$`, tc.branchIsBackWhereItStarted)
	ctx.Then(`^the tag "([^"]*)" no longer exists$`, tc.tagNoLongerExists)
	ctx.Then(`^the tag "([^"]*)" still exists$`, tc.tagStillExists)
	ctx.Then(`^the working tree is clean$`, tc.workingTreeIsClean)
}

func (tc *TestContext) bump() *bumpState {
	if tc.bumpScenario == nil {
		tc.bumpScenario = &bumpState{declared: map[string]string{}}
	}
	return tc.bumpScenario
}

func (tc *TestContext) hostGitIdentityIs(name, email string) error {
	tc.bump().hostName, tc.bump().hostEmail = name, email
	return nil
}

func (tc *TestContext) hostHasNoGitIdentity() error {
	tc.bump().hostName, tc.bump().hostEmail = "", ""
	return nil
}

func (tc *TestContext) actionDeclaresNoIdentity() error {
	// The stock action env: safe.directory, no identity.
	tc.bump().declared = map[string]string{"GIT_CONFIG_KEY_0": "safe.directory", "GIT_CONFIG_VALUE_0": "/app"}
	return nil
}

func (tc *TestContext) actionDeclares(key, value string) error {
	tc.bump().declared[key] = value
	return nil
}

func (tc *TestContext) bumpEnvironmentIsResolved() error {
	b := tc.bump()
	b.env, b.envErr = actions.BumpAuthorEnv(b.declared, b.hostName, b.hostEmail)
	return nil
}

func (tc *TestContext) bumpEnvironmentSets(key, value string) error {
	b := tc.bump()
	if b.envErr != nil {
		return fmt.Errorf("resolving failed: %w", b.envErr)
	}
	if got := b.env[key]; got != value {
		return fmt.Errorf("%s = %q, want %q", key, got, value)
	}
	return nil
}

func (tc *TestContext) bumpEnvironmentFailsMentioning(fragment string) error {
	b := tc.bump()
	if b.envErr == nil {
		return fmt.Errorf("resolving succeeded, expected a failure mentioning %q", fragment)
	}
	if !strings.Contains(b.envErr.Error(), fragment) {
		return fmt.Errorf("error %q does not mention %q", b.envErr, fragment)
	}
	return nil
}

// git runs git in the scenario repository with a fixed identity: these commits
// stand in for the ones the bump container makes.
func (tc *TestContext) bumpGit(args ...string) (string, error) {
	full := append([]string{"-c", "user.name=Scenario", "-c", "user.email=scenario@example.test"}, args...)
	out, err := vcs.Git(tc.bump().repo, full...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

func (tc *TestContext) repositoryWithNoVersionFile() error {
	dir, err := tc.scenarioDir()
	if err != nil {
		return err
	}
	b := tc.bump()
	b.repo = dir
	if _, err := tc.bumpGit("init", "-q"); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("version = \"1.3.0\"\n"), 0o644); err != nil {
		return err
	}
	if _, err := tc.bumpGit("add", "."); err != nil {
		return err
	}
	if _, err := tc.bumpGit("commit", "-q", "-m", "feat: start"); err != nil {
		return err
	}
	b.baseSHA, err = tc.bumpGit("rev-parse", "HEAD")
	return err
}

func (tc *TestContext) bumpCommitTagged(tag string) error {
	if err := os.WriteFile(filepath.Join(tc.bump().repo, "Cargo.toml"), []byte("version = \"1.4.0\"\n"), 0o644); err != nil {
		return err
	}
	if _, err := tc.bumpGit("commit", "-q", "-am", "bump: 1.3.0 → 1.4.0"); err != nil {
		return err
	}
	_, err := tc.bumpGit("tag", tag)
	return err
}

func (tc *TestContext) bumpCreatedNoCommit() error { return nil }

func (tc *TestContext) bumpModifiedWithoutCommitting() error {
	return os.WriteFile(filepath.Join(tc.bump().repo, "Cargo.toml"), []byte("version = \"1.4.0\"\n"), 0o644)
}

func (tc *TestContext) startingCommitTagged(tag string) error {
	_, err := tc.bumpGit("tag", tag)
	return err
}

func (tc *TestContext) bumpedVersionIsRead() error {
	b := tc.bump()
	b.version, b.readErr = actions.BumpedVersion(b.repo, b.baseSHA)
	return nil
}

func (tc *TestContext) bumpedVersionIs(want string) error {
	b := tc.bump()
	if b.readErr != nil {
		return fmt.Errorf("reading failed: %w", b.readErr)
	}
	if b.version != want {
		return fmt.Errorf("bumped version %q, want %q", b.version, want)
	}
	return nil
}

func (tc *TestContext) readingBumpedVersionFailsMentioning(fragment string) error {
	b := tc.bump()
	if b.readErr == nil {
		return fmt.Errorf("reading succeeded (%q), expected a failure mentioning %q", b.version, fragment)
	}
	if !strings.Contains(b.readErr.Error(), fragment) {
		return fmt.Errorf("error %q does not mention %q", b.readErr, fragment)
	}
	return nil
}

func (tc *TestContext) bumpIsUndone() error {
	b := tc.bump()
	b.undoErr = actions.UndoBump(b.repo, b.baseSHA)
	return b.undoErr
}

func (tc *TestContext) branchIsBackWhereItStarted() error {
	head, err := tc.bumpGit("rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != tc.bump().baseSHA {
		return fmt.Errorf("HEAD is %s, the release started at %s", head, tc.bump().baseSHA)
	}
	return nil
}

func (tc *TestContext) tagExists(tag string) (bool, error) {
	out, err := tc.bumpGit("tag", "--list", tag)
	return out == tag, err
}

func (tc *TestContext) tagNoLongerExists(tag string) error {
	exists, err := tc.tagExists(tag)
	if err == nil && exists {
		return fmt.Errorf("tag %s still exists", tag)
	}
	return err
}

func (tc *TestContext) tagStillExists(tag string) error {
	exists, err := tc.tagExists(tag)
	if err == nil && !exists {
		return fmt.Errorf("tag %s was deleted", tag)
	}
	return err
}

func (tc *TestContext) workingTreeIsClean() error {
	out, err := tc.bumpGit("status", "--porcelain")
	if err == nil && out != "" {
		return fmt.Errorf("working tree is not clean:\n%s", out)
	}
	return err
}
