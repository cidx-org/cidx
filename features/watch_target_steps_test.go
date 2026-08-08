package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/internal/commands"
	"github.com/cucumber/godog"
)

// RegisterWatchTargetSteps registers the steps for the guard that stops a watch
// reporting on a commit the reader does not have.
//
// The steps drive commands.JudgeWatchTarget, the whole of the decision. The
// plumbing around it — reading local HEAD, carrying the pull request's head SHA
// through PRInfo — is covered by the unit tests of internal/commands, the same
// split failing_checks.feature already makes.
func RegisterWatchTargetSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^local HEAD is "([^"]*)"$`, tc.localHeadIs)
	ctx.Given(`^local HEAD cannot be read$`, tc.localHeadCannotBeRead)
	ctx.Given(`^the pull request reports its head commit as "([^"]*)"$`, tc.prReportsHeadCommit)
	ctx.Given(`^the pull request reports no head commit$`, tc.prReportsNoHeadCommit)

	ctx.When(`^cidx checks what the watch is about to report on$`, tc.judgeTheWatchTarget)

	ctx.Then(`^the watch proceeds$`, tc.theWatchProceeds)
	ctx.Then(`^the watch refuses$`, tc.theWatchRefuses)
	ctx.Then(`^the watch names the commit "([^"]*)"$`, tc.theWatchNamesTheCommit)
	ctx.Then(`^the refusal names the commit "([^"]*)" as the local one$`, tc.refusalNamesLocal)
	ctx.Then(`^the refusal names the commit "([^"]*)" as the one under test$`, tc.refusalNamesUnderTest)
	ctx.Then(`^the refusal mentions "([^"]*)"$`, tc.refusalMentions)
	ctx.Then(`^the watch says the commit could not be verified$`, tc.watchSaysUnverified)
}

func (tc *TestContext) localHeadIs(sha string) error {
	tc.watchLocalSHA = sha
	return nil
}

func (tc *TestContext) localHeadCannotBeRead() error {
	tc.watchLocalSHA = ""
	return nil
}

func (tc *TestContext) prReportsHeadCommit(sha string) error {
	tc.watchRemoteSHA = sha
	return nil
}

func (tc *TestContext) prReportsNoHeadCommit() error {
	tc.watchRemoteSHA = ""
	return nil
}

func (tc *TestContext) judgeTheWatchTarget() error {
	tc.watchProceeds, tc.watchMessage = commands.JudgeWatchTarget(tc.watchLocalSHA, tc.watchRemoteSHA)
	return nil
}

func (tc *TestContext) theWatchProceeds() error {
	if !tc.watchProceeds {
		return fmt.Errorf("the watch refused, and this scenario expects it to proceed: %s", tc.watchMessage)
	}
	return nil
}

func (tc *TestContext) theWatchRefuses() error {
	if tc.watchProceeds {
		return fmt.Errorf("the watch proceeded, and this scenario expects a refusal: %s", tc.watchMessage)
	}
	return nil
}

func (tc *TestContext) theWatchNamesTheCommit(sha string) error {
	return tc.watchMessageContains(sha)
}

// The two SHAs mean opposite things, so naming them is not enough — a message
// that swapped them would satisfy a plain substring check on both.
func (tc *TestContext) refusalNamesLocal(sha string) error {
	return tc.watchMessageContains("you have " + sha)
}

func (tc *TestContext) refusalNamesUnderTest(sha string) error {
	return tc.watchMessageContains("these checks are for " + sha)
}

func (tc *TestContext) refusalMentions(text string) error {
	return tc.watchMessageContains(text)
}

func (tc *TestContext) watchSaysUnverified() error {
	return tc.watchMessageContains("Could not verify which commit")
}

func (tc *TestContext) watchMessageContains(want string) error {
	if !strings.Contains(tc.watchMessage, want) {
		return fmt.Errorf("expected the message to contain %q, got:\n%s", want, tc.watchMessage)
	}
	return nil
}
