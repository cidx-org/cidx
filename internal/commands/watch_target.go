package commands

import "fmt"

// A watch reports the checks a provider holds for a pull request. Those checks
// belong to whatever commit the remote has, which is the commit in hand only
// when the push actually happened.
//
// When it did not, every layer behaves correctly and the answer is still wrong:
// `cpw` returns on "No changes to commit" before reaching its push step, the
// branch keeps its local commits, the provider reports green for the commit
// before them, and the watch prints "All checks passed" — about code CI has
// never seen. Nothing errors anywhere along that path.
//
// So the commit under test is stated, and when it is not the commit in hand the
// watch refuses instead of reporting. Same answer the scan gate gives: a verdict
// needs to be about the thing it claims to be about.

// shortSHA is a commit as a reader recognises it — the form git log prints and
// the form a GitHub URL ends in. Comparison always happens on the full string;
// only the message is abbreviated.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// JudgeWatchTarget decides whether a watch may report, given the commit in hand
// and the commit the provider holds checks for.
//
// Either SHA may be empty, meaning it could not be established. That does not
// refuse: the guard exists to stop a confident wrong answer, and "I could not
// tell" is not one — a watch is still the useful thing to run. What it must not
// do is stay quiet, because an unverified report would then be indistinguishable
// from a verified one.
func JudgeWatchTarget(localSHA, prHeadSHA string) (proceed bool, message string) {
	switch {
	case localSHA == "" || prHeadSHA == "":
		return true, "⚠️  Could not verify which commit these checks are for — the report below may not be about the commit you have"

	case localSHA == prHeadSHA:
		return true, fmt.Sprintf("📍 Watching CI for commit %s", shortSHA(prHeadSHA))

	default:
		return false, fmt.Sprintf(
			"refusing to report: these checks are for %s, and you have %s\n"+
				"   The commit you are on never reached the remote, so CI has not seen it.\n"+
				"   Push it and watch that instead: cidx cpw -m \"your message\"",
			shortSHA(prHeadSHA), shortSHA(localSHA))
	}
}
