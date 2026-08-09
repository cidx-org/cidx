package actions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// squashMergedRepo builds a repository whose `feat` branch was pushed and then
// squash-merged into main, which is the state postMergeCleanup runs in.
//
// Only git being absent is worth skipping over. Every other failure here is
// this helper being wrong, and skipping would report success by not running.
func squashMergedRepo(t *testing.T) string {
	t.Helper()

	workDir, bare := t.TempDir(), t.TempDir()
	if err := vcs.Git(workDir, "--version").Run(); err != nil {
		t.Skipf("git is not available: %v", err)
	}

	run := func(dir string, args ...string) {
		t.Helper()
		if out, err := vcs.Git(dir, args...).CombinedOutput(); err != nil {
			t.Fatalf("setup step `git %v` failed: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	run(bare, "init", "--bare")
	run(workDir, "init", "-b", "main")
	run(workDir, "config", "user.email", "deletion@example.test")
	run(workDir, "config", "user.name", "deletion")
	write("root.txt", "root\n")
	run(workDir, "add", "-A")
	run(workDir, "commit", "-m", "root")
	run(workDir, "remote", "add", "origin", bare)
	run(workDir, "push", "--set-upstream", "origin", "main")

	run(workDir, "checkout", "-b", "feat")
	write("feat.txt", "work\n")
	run(workDir, "add", "-A")
	run(workDir, "commit", "-m", "feat: work")
	run(workDir, "push", "--set-upstream", "origin", "feat")

	run(workDir, "checkout", "main")
	run(workDir, "merge", "--squash", "feat")
	run(workDir, "commit", "-m", "feat: work (#1)")
	run(workDir, "push", "origin", "main")

	return workDir
}

func branchExists(t *testing.T, workDir, branch string) bool {
	t.Helper()
	return vcs.Git(workDir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch).Run() == nil
}

// TestPostMergeCleanup_KeepsABranchHoldingAnUnpushedCommit is the whole of #417
// for this call site.
//
// The old sequence ran `git branch -d`, and when git refused, ran `git branch -D`
// instead. Since -d compares against the upstream, its refusal here means one
// thing only: the branch holds a commit the remote has never seen. Forcing past
// that deleted the only copy of it, and the merge had squashed a different
// commit, so nothing anywhere else held the work.
func TestPostMergeCleanup_KeepsABranchHoldingAnUnpushedCommit(t *testing.T) {
	workDir := squashMergedRepo(t)

	// The accident: a commit made on the branch that never reached the remote.
	for _, args := range [][]string{
		{"checkout", "feat"},
		{"commit", "--allow-empty", "-m", "only on this machine"},
		{"checkout", "main"},
	} {
		if out, err := vcs.Git(workDir, args...).CombinedOutput(); err != nil {
			t.Fatalf("failed to add an unpushed commit: %v\n%s", err, out)
		}
	}

	repo, err := vcs.OpenRepository(workDir)
	if err != nil {
		t.Fatalf("failed to open the repository: %v", err)
	}

	action := &PRAction{repo: repo}
	if err := action.postMergeCleanup("feat"); err != nil {
		t.Fatalf("cleanup after a merge is not allowed to fail on a branch it decided to keep: %v", err)
	}

	if !branchExists(t, workDir, "feat") {
		t.Error("the branch was deleted although it held a commit that existed nowhere else -- " +
			"that commit is now reachable only from the reflog, and nothing said so")
	}
}

// TestPostMergeCleanup_DeletesABranchWhoseWorkIsSafe keeps the fix from becoming
// a cleanup that never cleans anything: `git branch -d` accepts a squash-merged
// branch whose commits are all on its upstream, and that is the ordinary case.
func TestPostMergeCleanup_DeletesABranchWhoseWorkIsSafe(t *testing.T) {
	workDir := squashMergedRepo(t)

	repo, err := vcs.OpenRepository(workDir)
	if err != nil {
		t.Fatalf("failed to open the repository: %v", err)
	}

	action := &PRAction{repo: repo}
	if err := action.postMergeCleanup("feat"); err != nil {
		t.Fatalf("cleanup failed on the ordinary case: %v", err)
	}

	if branchExists(t, workDir, "feat") {
		t.Error("a squash-merged branch fully present on its upstream was kept -- " +
			"a guard that keeps every branch is one that gets removed")
	}
}

// TestPostMergeCleanupNeverForces fails if the `-D` fallback comes back.
//
// The behavioural tests above cover what the code does today; this covers what
// nothing else would notice. Reintroducing the fallback makes both of them pass
// again except the one assertion about a branch surviving -- and that assertion
// is easy to read as flaky and delete. Naming the flag directly leaves no doubt
// about which line was the bug.
func TestPostMergeCleanupNeverForces(t *testing.T) {
	source, err := os.ReadFile("pr.go")
	if err != nil {
		t.Fatalf("failed to read pr.go: %v", err)
	}

	if strings.Contains(string(source), `"branch", "-D"`) {
		t.Error(`pr.go runs "git branch -D" again. -d refuses a branch only when it holds commits ` +
			`its upstream does not have, so forcing past that refusal deletes the one copy of them (#417). ` +
			`If a branch has to go regardless, that is the user's call to make with git, not this cleanup's.`)
	}
}
