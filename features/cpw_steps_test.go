package features

import (
	"fmt"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cucumber/godog"
)

// RegisterCPWSteps registers the steps for what a cpw run decides to do (#416).
//
// The steps drive actions.PlanCommitPushWatch, the whole of the decision. That
// Execute follows the plan -- commits only when told to, pushes whenever told
// to -- is covered by the unit tests of pkg/actions, which reach the private
// flow directly.
func RegisterCPWSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the working tree has changes$`, tc.treeHasChanges)
	ctx.Given(`^the working tree has nothing to commit$`, tc.treeIsClean)
	ctx.Given(`^the branch has commits the remote has not seen$`, tc.branchIsAhead)
	ctx.Given(`^the branch is level with the remote$`, tc.branchIsLevel)

	ctx.When(`^cidx plans what cpw will do$`, tc.planCPW)

	ctx.Then(`^cpw commits and pushes$`, tc.cpwCommitsAndPushes)
	ctx.Then(`^cpw pushes without committing$`, tc.cpwPushesOnly)
	ctx.Then(`^cpw does nothing$`, tc.cpwDoesNothing)
	ctx.Then(`^cpw says it is pushing commits that never reached the remote$`, tc.cpwSaysPushingUnpushed)
	ctx.Then(`^cpw says there is nothing waiting to be pushed$`, tc.cpwSaysNothingWaiting)
	ctx.Then(`^cpw runs the code phase first$`, tc.cpwRunsCodePhase)
	ctx.Then(`^cpw runs no code phase$`, tc.cpwRunsNoCodePhase)
}

func (tc *TestContext) treeHasChanges() error { tc.cpwHasChanges = true; return nil }
func (tc *TestContext) treeIsClean() error    { tc.cpwHasChanges = false; return nil }
func (tc *TestContext) branchIsAhead() error  { tc.cpwHasUnpushed = true; return nil }
func (tc *TestContext) branchIsLevel() error  { tc.cpwHasUnpushed = false; return nil }

func (tc *TestContext) planCPW() error {
	tc.cpwPlan, tc.cpwMessage = actions.PlanCommitPushWatch(tc.cpwHasChanges, tc.cpwHasUnpushed)
	return nil
}

func (tc *TestContext) cpwCommitsAndPushes() error {
	return tc.expectPlan(actions.CPWCommitAndPush, "commit and push")
}

func (tc *TestContext) cpwPushesOnly() error {
	return tc.expectPlan(actions.CPWPushOnly, "push without committing")
}

func (tc *TestContext) cpwDoesNothing() error {
	return tc.expectPlan(actions.CPWNothingToDo, "do nothing")
}

func (tc *TestContext) expectPlan(want actions.CPWPlan, described string) error {
	if tc.cpwPlan != want {
		return fmt.Errorf("expected cpw to %s, it planned %v (%q)", described, tc.cpwPlan, tc.cpwMessage)
	}
	return nil
}

func (tc *TestContext) cpwSaysPushingUnpushed() error {
	return tc.cpwMessageContains("never reached the remote")
}

func (tc *TestContext) cpwSaysNothingWaiting() error {
	return tc.cpwMessageContains("nothing waiting to be pushed")
}

func (tc *TestContext) cpwRunsCodePhase() error {
	if !tc.cpwPlan.RunsCodePhase() {
		return fmt.Errorf("a run that puts something new in front of CI has to run the code phase first")
	}
	return nil
}

func (tc *TestContext) cpwRunsNoCodePhase() error {
	if tc.cpwPlan.RunsCodePhase() {
		return fmt.Errorf("a run with nothing to push must not spend ~20s on a code phase")
	}
	return nil
}

func (tc *TestContext) cpwMessageContains(want string) error {
	if !strings.Contains(tc.cpwMessage, want) {
		return fmt.Errorf("expected the message to contain %q, got: %q", want, tc.cpwMessage)
	}
	return nil
}
