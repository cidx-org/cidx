package vcs

import "os/exec"

// CIDX reads what git prints. It decides that a checkout failed only because
// another worktree holds the branch, that a push failed only because the remote
// ref was already gone, that a branch has no upstream yet -- all by matching
// git's own sentences, because git has no exit code that says which of those it
// was. Those sentences are translated. On a French machine git answers "la
// référence distante n'existe pas", every match misses, and CIDX reports a
// failure for the cases it was written to forgive (issue #364).
//
// So git's output is made an interface CIDX controls instead of a setting the
// user happens to have. LC_ALL=C is the whole fix, and it is enough on its own:
// gettext ignores LANGUAGE when the locale is C, so there is no second variable
// to neutralize. It pins messages only in the sense that matters -- git writes
// commit subjects, paths and refs as the bytes it stored, not as text it
// re-encodes for the locale, so UTF-8 content still arrives intact.
//
// The English that CIDX matches on is also the English that reaches the user in
// an error, which is the same language the rest of CIDX speaks.
//
// TestEveryGitInvocationPinsTheLocale keeps this the only place that builds a
// git command: a call site that spells out exec.Command("git", ...) is born
// reading whatever git was configured to say, and the next parse written
// against it would be wrong on the same machines.
func Git(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	// Set before Environ: it derives PWD from Dir.
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "LC_ALL=C")

	return cmd
}
