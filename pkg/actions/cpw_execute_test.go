package actions

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// noPRProvider stops Execute after the push: with no pull request for the
// branch, watchCI reports that and returns, which is exactly the part of the
// run these tests are not about.
type noPRProvider struct {
	remote.Provider
}

func (p *noPRProvider) GetPullRequestByBranch(context.Context, string) (int, string, error) {
	return 0, "", errors.New("no pull request in this test")
}

// branchWithARemote builds a feature branch with a real bare remote behind it,
// so Push() does what it does in anger.
func branchWithARemote(t *testing.T) (repo *vcs.Repository, workDir, bare string) {
	t.Helper()

	workDir, bare = t.TempDir(), t.TempDir()
	if out, err := vcs.Git(bare, "init", "--bare").CombinedOutput(); err != nil {
		t.Skipf("cannot create a bare repository here (%v): %s", err, out)
	}

	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "cpw@example.test"},
		{"config", "user.name", "cpw"},
		{"commit", "--allow-empty", "-m", "root"},
		{"remote", "add", "origin", bare},
		{"push", "--set-upstream", "origin", "main"},
		{"checkout", "-b", "feat/already-committed"},
	} {
		if out, err := vcs.Git(workDir, args...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable or refusing to work here (%v): %s", err, out)
		}
	}

	repo, err := vcs.OpenRepository(workDir)
	if err != nil {
		t.Fatalf("failed to open the temporary repository: %v", err)
	}
	return repo, workDir, bare
}

// remoteHas reports whether the bare repository holds sha.
func remoteHas(t *testing.T, bare, sha string) bool {
	t.Helper()

	out, err := vcs.Git(bare, "cat-file", "-t", sha).CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "commit"
}

// TestExecute_PushesCommitsThatWereAlreadyWritten is the whole of #416.
//
// A clean tree with commits the remote has not seen used to return on "No
// changes to commit", before the push step: the work stayed local, cpw reported
// success, and a watch then went green on the commit before it. Reverting the
// plan in Execute breaks no other test in this package, so the regression is
// pinned here, against a real remote.
func TestExecute_PushesCommitsThatWereAlreadyWritten(t *testing.T) {
	repo, workDir, bare := branchWithARemote(t)

	if out, err := vcs.Git(workDir, "commit", "--allow-empty", "-m", "feat: written by hand").CombinedOutput(); err != nil {
		t.Fatalf("failed to add a commit: %v\n%s", err, out)
	}

	sha, err := repo.GetHeadSHA()
	if err != nil {
		t.Fatalf("failed to read HEAD: %v", err)
	}
	if remoteHas(t, bare, sha) {
		t.Fatal("the remote already has the commit, so this test would pass without pushing anything")
	}

	// verify=false: the code phase needs a container runtime, and what is under
	// test here is which steps run, not what they check.
	action := NewCommitPushWatch(repo, &noPRProvider{}, "", false)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("a clean tree with unpushed commits is an ordinary cpw run: %v", err)
	}

	if !remoteHas(t, bare, sha) {
		t.Error("cpw returned without pushing a commit the remote had never seen -- " +
			"this is the branch staying local while cpw reports success, which is how " +
			"a watch went green on its parent (#414)")
	}
}

// TestExecute_StopsWhenThereIsGenuinelyNothingToDo keeps the fix from becoming
// a push on every invocation.
func TestExecute_StopsWhenThereIsGenuinelyNothingToDo(t *testing.T) {
	repo, workDir, _ := branchWithARemote(t)

	if out, err := vcs.Git(workDir, "push", "--set-upstream", "origin", "feat/already-committed").CombinedOutput(); err != nil {
		t.Skipf("cannot push the feature branch (%v): %s", err, out)
	}

	action := NewCommitPushWatch(repo, &noPRProvider{}, "", false)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("a branch level with its remote is not an error: %v", err)
	}
}
