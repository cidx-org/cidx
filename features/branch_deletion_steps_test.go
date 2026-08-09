package features

import (
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cucumber/godog"
)

// RegisterBranchDeletionSteps registers the steps for the decision that stopped
// `branch cleanup` overruling git on a branch holding unpushed work (#417).
//
// The steps drive branch.MayForceDelete. That the two other deletion sites no
// longer force at all is pinned beside each of them, in pkg/actions and
// internal/commands -- a removal nothing else would notice.
func RegisterBranchDeletionSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the branch is merged$`, tc.branchIsMerged)
	ctx.Given(`^the branch is not merged$`, tc.branchIsNotMerged)
	ctx.Given(`^the remote holds the same commit as the local branch$`, tc.remoteHoldsSameCommit)
	ctx.Given(`^the local branch is ahead of the remote$`, tc.localIsAhead)
	ctx.Given(`^the remote branch no longer exists$`, tc.remoteBranchIsGone)
	ctx.Given(`^--force was given$`, tc.forceWasGiven)

	ctx.When(`^cidx decides how to delete it$`, tc.decideHowToDelete)

	ctx.Then(`^the deletion is forced$`, tc.deletionIsForced)
	ctx.Then(`^the deletion is not forced$`, tc.deletionIsNotForced)
}

func (tc *TestContext) branchIsMerged() error {
	tc.deleteStatus = branch.StatusMerged
	return nil
}

func (tc *TestContext) branchIsNotMerged() error {
	tc.deleteStatus = branch.StatusActive
	return nil
}

func (tc *TestContext) remoteHoldsSameCommit() error {
	tc.deleteLocalHash, tc.deleteRemoteHash = "a88cb3e", "a88cb3e"
	return nil
}

func (tc *TestContext) localIsAhead() error {
	tc.deleteLocalHash, tc.deleteRemoteHash = "6414eb1", "a88cb3e"
	return nil
}

// A remote branch that is gone leaves no hash to compare against, which is the
// state the merged verdict has to decide on its own.
func (tc *TestContext) remoteBranchIsGone() error {
	tc.deleteLocalHash, tc.deleteRemoteHash = "6414eb1", ""
	return nil
}

func (tc *TestContext) forceWasGiven() error {
	tc.deleteForce = true
	return nil
}

func (tc *TestContext) decideHowToDelete() error {
	tc.deleteForced = branch.MayForceDelete(tc.deleteForce, tc.deleteStatus, tc.deleteLocalHash, tc.deleteRemoteHash)
	return nil
}

func (tc *TestContext) deletionIsForced() error {
	if !tc.deleteForced {
		return fmt.Errorf("the deletion was left to `git branch -d`, and this scenario expects -D")
	}
	return nil
}

func (tc *TestContext) deletionIsNotForced() error {
	if tc.deleteForced {
		return fmt.Errorf("the deletion was forced -- `git branch -D` on a branch git had just refused " +
			"is how the only copy of a commit goes")
	}
	return nil
}
