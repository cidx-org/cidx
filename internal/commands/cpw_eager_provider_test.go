package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
)

// TestCPWRefusesBeforeTouchingTheRepository is the standing guard behind the
// decision of issue #358.
//
// Every other command was moved to a lazily-built provider by #227, #350 and
// #356: one whose local-only steps never reach the remote must not fail on an
// unusable one. cpw is the last that looks like that family and is deliberately
// left out of it, because deferring here would not move construction — it would
// change what an unusable remote means for a command that has already mutated
// the repository.
//
// The choice is to fail first. cpw's contract is commit, push and watch; an
// unparseable origin or a missing token resolved late would leave a commit
// made, a push attempted, and the user somewhere they did not ask to be —
// after the side effects rather than before. Refusing up front leaves the
// working tree exactly as it was, which is the same posture #307 chose for the
// code phase.
//
// So this asserts the property rather than the wiring: with a remote that
// cannot be resolved, the command fails and the change is still uncommitted.
func TestCPWRefusesBeforeTouchingTheRepository(t *testing.T) {
	dir := repositoryOnABranchWithUnusableRemote(t)

	// Work in progress, exactly what cpw exists to commit.
	if err := os.WriteFile(filepath.Join(dir, "work.txt"), []byte("in progress\n"), 0o600); err != nil {
		t.Fatalf("failed to stage the work: %v", err)
	}

	before := headOf(t, dir)

	err := NewApp().Run([]string{"cidx", "cpw", "-m", "feat: something", "--no-verify"})
	if err == nil {
		t.Fatal("cpw succeeded with a remote it cannot resolve: the commit it makes could then never be pushed or watched")
	}

	if after := headOf(t, dir); after != before {
		t.Errorf("cpw committed before finding out the remote is unusable: HEAD moved %s → %s", before, after)
	}

	status := gitOutput(t, dir, "status", "--porcelain")
	if !strings.Contains(status, "work.txt") {
		t.Errorf("the work is no longer uncommitted, so something touched the repository:\n%s", status)
	}
}

// TestCPWBuildsItsProviderBeforeItCommits is the same decision seen from the
// wiring: the provider is asked for, and the refusal comes from that.
func TestCPWBuildsItsProviderBeforeItCommits(t *testing.T) {
	repositoryOnABranchWithUnusableRemote(t)

	original := createProvider
	t.Cleanup(func() { createProvider = original })

	asked := false
	createProvider = func(repo *vcs.Repository) (remote.Provider, error) {
		asked = true
		return original(repo)
	}

	_ = NewApp().Run([]string{"cidx", "cpw", "-m", "feat: something", "--no-verify"})

	if !asked {
		t.Error("cpw never asked for a provider: it would find out the remote is unusable after committing (#358)")
	}
}

// repositoryOnABranchWithUnusableRemote is repositoryOnMainWithUnusableRemote
// on a feature branch, since cpw refuses to push to main before anything else.
func repositoryOnABranchWithUnusableRemote(t *testing.T) string {
	t.Helper()

	dir := repositoryOnMainWithUnusableRemote(t)
	if out, err := exec.Command("git", "-C", dir, "checkout", "-q", "-b", "feat/work").CombinedOutput(); err != nil {
		t.Fatalf("git checkout failed: %v\n%s", err, out)
	}

	return dir
}

func headOf(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gitOutput(t, dir, "rev-parse", "HEAD"))
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return string(out)
}
