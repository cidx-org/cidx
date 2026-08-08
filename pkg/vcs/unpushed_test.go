package vcs

import (
	"testing"
)

// HasUnpushedCommits is what tells "nothing to commit" apart from "nothing to
// do" (#416). Getting it wrong in either direction is quiet: answering false on
// a branch that is ahead strands the work, and answering true on one that is
// level makes cpw push on every run.
//
// These drive real repositories, because the answer comes from git's output and
// the case that matters most is one git reports as an *error*.

// gitRepo builds a repository with one commit and no remote.
func gitRepo(t *testing.T) *Repository {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "unpushed@example.test"},
		{"config", "user.name", "unpushed"},
		{"commit", "--allow-empty", "-m", "root"},
	} {
		if out, err := Git(dir, args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable or refusing to init here (%v): %s", err, out)
		}
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("failed to open the temporary repository: %v", err)
	}
	return repo
}

// workDirOf is the path git commands run in for this repository.
func workDirOf(t *testing.T, repo *Repository) string {
	t.Helper()

	dir, err := repo.GetWorkDir()
	if err != nil {
		t.Fatalf("failed to read the work directory: %v", err)
	}
	return dir
}

// TestHasUnpushedCommits_ABranchWithNoUpstreamCountsAsAhead is the case this
// function exists for. git errors on `@{u}` rather than answering zero, and
// reading that as "nothing to push" would strand every brand-new branch —
// exactly the ones cpw is most often run on.
func TestHasUnpushedCommits_ABranchWithNoUpstreamCountsAsAhead(t *testing.T) {
	repo := gitRepo(t)

	unpushed, err := repo.HasUnpushedCommits()
	if err != nil {
		t.Fatalf("a branch with no upstream is an ordinary state, not an error: %v", err)
	}
	if !unpushed {
		t.Error("a branch that was never pushed has everything to push, so cpw must not stop on it")
	}
}

// TestHasUnpushedCommits_LevelWithTheRemoteIsNothingToPush uses a second
// repository as the remote, which is enough for git to resolve @{u}.
func TestHasUnpushedCommits_LevelWithTheRemoteIsNothingToPush(t *testing.T) {
	repo := gitRepo(t)
	dir := workDirOf(t, repo)
	remote := t.TempDir()

	for _, args := range [][]string{
		{"init", "--bare"},
	} {
		if out, err := Git(remote, args...).CombinedOutput(); err != nil {
			t.Skipf("cannot create a bare repository here (%v): %s", err, out)
		}
	}
	for _, args := range [][]string{
		{"remote", "add", "origin", remote},
		{"push", "--set-upstream", "origin", "main"},
	} {
		if out, err := Git(dir, args...).CombinedOutput(); err != nil {
			t.Skipf("cannot push to the local bare repository (%v): %s", err, out)
		}
	}

	unpushed, err := repo.HasUnpushedCommits()
	if err != nil {
		t.Fatalf("failed to count unpushed commits: %v", err)
	}
	if unpushed {
		t.Error("everything is on the remote, so cpw has nothing to push and must say so")
	}

	// And one commit later, it does.
	if out, err := Git(dir, "commit", "--allow-empty", "-m", "ahead").CombinedOutput(); err != nil {
		t.Fatalf("failed to add a commit: %v\n%s", err, out)
	}

	unpushed, err = repo.HasUnpushedCommits()
	if err != nil {
		t.Fatalf("failed to count unpushed commits: %v", err)
	}
	if !unpushed {
		t.Error("a commit the remote does not have is exactly what cpw must push -- " +
			"answering false here is the bug that let a branch stay local while cpw reported success")
	}
}
