package actions

// CPWPlan is what a cpw run has to do, decided before it does any of it.
type CPWPlan int

const (
	// CPWNothingToDo: a clean tree, and the remote already has every commit.
	CPWNothingToDo CPWPlan = iota
	// CPWPushOnly: a clean tree, but commits that never reached the remote.
	CPWPushOnly
	// CPWCommitAndPush: work in the tree, which is what cpw was built for.
	CPWCommitAndPush
)

// PlanCommitPushWatch decides what a cpw run does.
//
// The bug this exists to prevent is one step: cpw asked whether there was
// anything to commit and returned when there was not, before ever reaching its
// push. A branch whose commits were already written and whose tree was clean
// therefore never left the machine, while the message — "No changes to commit"
// — was true and read like "nothing to do".
//
// Everything downstream then behaved correctly about the wrong commit: the
// provider reported checks for the commit before, and a watch called them
// green. #414 and #415 refuse those answers; this is why they rarely have to.
func PlanCommitPushWatch(hasChanges, hasUnpushed bool) (plan CPWPlan, message string) {
	switch {
	case hasChanges:
		return CPWCommitAndPush, ""

	case hasUnpushed:
		return CPWPushOnly, "📤 Nothing to commit, but this branch has commits that never reached the remote -- pushing those"

	default:
		return CPWNothingToDo, "Nothing to commit, and nothing waiting to be pushed"
	}
}

// RunsCodePhase reports whether a plan puts something new in front of CI.
//
// A push of commits written by hand earns the same gate as a push of one cpw
// just made (#307): the phase is about what CI is going to run, not about how
// the commit came to exist.
func (p CPWPlan) RunsCodePhase() bool {
	return p != CPWNothingToDo
}
