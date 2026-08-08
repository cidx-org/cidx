package actions

import "fmt"

// A watch reports the checks a provider holds for a pull request; a merge lands
// them. Both belong to whatever commit the *remote* has, which is the commit in
// hand only when the push actually happened.
//
// When it did not, every layer behaves correctly and the answer is still wrong.
// `cpw` returns on "No changes to commit" before reaching its push step, so a
// branch with local commits and a clean tree never gets pushed; the provider
// then reports on the commit before them. A watch prints "All checks passed"
// about code CI has never seen (#414). A merge is worse: it squashes the commit
// before, and postMergeCleanup then deletes the branch holding the work —
// falling through to `git branch -D` when `-d` refuses, which is git's own
// safety net being stepped over.
//
// So both state the commit they are acting on, and refuse when it is not the
// one in hand. The same answer the scan gate gives: a verdict has to be about
// the thing it claims to be about.

// JudgeWatchTarget decides whether a watch may report, given the commit in hand
// and the commit the provider holds checks for.
//
// Either SHA may be empty, meaning it could not be established. That does not
// refuse: the guard exists to stop a confident wrong answer, and "I could not
// tell" is not one — a watch is still the useful thing to run. What it must not
// do is stay quiet, because an unverified report would then be
// indistinguishable from a verified one.
func JudgeWatchTarget(localSHA, prHeadSHA string) (proceed bool, message string) {
	if unverifiable(localSHA, prHeadSHA) {
		return true, unverifiableMessage("the report below may not be about the commit you have")
	}

	if localSHA == prHeadSHA {
		return true, fmt.Sprintf("📍 Watching CI for commit %s", shortSHA(prHeadSHA))
	}

	return false, fmt.Sprintf(
		"refusing to report: these checks are for %s, and you have %s\n"+
			"   The commit you are on never reached the remote, so CI has not seen it.\n"+
			"   Push it and watch that instead: cidx cpw -m \"your message\"",
		shortSHA(prHeadSHA), shortSHA(localSHA))
}

// JudgeMergeTarget decides whether a merge may land, given the same two commits.
//
// Both directions refuse, and they are different accidents. Ahead — commits
// that never reached the remote — is the destructive one: they are merged
// around, then deleted with the branch. Behind destroys nothing but merges code
// the reader has not seen. Telling them apart needs the remote commit to exist
// locally, which is exactly what is not guaranteed here, so the refusal names
// both remedies rather than guessing.
func JudgeMergeTarget(localSHA, prHeadSHA string) (proceed bool, message string) {
	if unverifiable(localSHA, prHeadSHA) {
		return true, unverifiableMessage("the merge below may not land the commit you have")
	}

	if localSHA == prHeadSHA {
		return true, fmt.Sprintf("📍 Merging commit %s", shortSHA(prHeadSHA))
	}

	return false, fmt.Sprintf(
		"refusing to merge: the pull request is at %s, and you have %s\n"+
			"   Merging now would land something other than what you have.\n"+
			"   If a commit never left this machine, push it:  cidx cpw -m \"your message\"\n"+
			"   If the remote moved ahead of you, catch up:    git pull\n"+
			"   The local branch is deleted after a merge, so a commit only you have goes with it.",
		shortSHA(prHeadSHA), shortSHA(localSHA))
}

// unverifiable reports whether either commit could not be established.
func unverifiable(localSHA, prHeadSHA string) bool {
	return localSHA == "" || prHeadSHA == ""
}

func unverifiableMessage(consequence string) string {
	return "⚠️  Could not verify which commit this is about — " + consequence
}
