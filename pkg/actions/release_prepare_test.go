package actions

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
)

// initRepoWithoutRemote creates a git repository with one conventional commit
// and no `origin`. That is the shape of a throwaway repo used to try the
// release flow -- and what `release prepare` used to refuse to run in.
func initRepoWithoutRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "VERSION"), []byte("1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
		{"add", "."},
		{"commit", "-m", "feat: something worth releasing", "--no-gpg-sign"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, output)
		}
	}

	return dir
}

// TestReleasePrepare_DryRunWithoutRemote covers #227: preparing release notes
// reads the git log and the version files only, so it must work in a
// repository that has no remote. It used to fail on "failed to get remote URL"
// before any of that ran, which is why the #185 and #212 end-to-end runs had
// to fake an origin.
func TestReleasePrepare_DryRunWithoutRemote(t *testing.T) {
	dir := initRepoWithoutRemote(t)

	repo, err := vcs.OpenRepository(dir)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}

	// Sanity check on the premise: there really is no remote here.
	if _, err := repo.GetRemoteURL(); err == nil {
		t.Fatal("fixture repository unexpectedly has a remote")
	}

	action := NewReleasePrepare(repo, config.DefaultReleaseConfig(), true)
	if err := action.Execute(context.Background()); err != nil {
		t.Fatalf("release prepare --dry-run must not need a remote, got: %v", err)
	}

	// A dry-run writes nothing.
	if _, err := os.Stat(filepath.Join(dir, ".cidx")); err == nil {
		t.Error("dry-run must not create .cidx")
	}
}

// TestGhCommandError_NamesCommandAndStderr covers the second half of #227:
// "Could not fetch PRs: exit status 1" named neither the tool that failed nor
// the reason.
func TestGhCommandError_NamesCommandAndStderr(t *testing.T) {
	// A real failing gh-shaped invocation, so the ExitError carries stderr.
	cmd := exec.Command("sh", "-c", "echo 'gh: not logged in' >&2; exit 1")
	_, execErr := cmd.Output()
	if execErr == nil {
		t.Fatal("fixture command was supposed to fail")
	}

	err := ghCommandError([]string{"pr", "list", "--state", "merged"}, execErr)

	for _, want := range []string{"gh pr list --state merged", "exit status 1", "not logged in"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// TestGhCommandError_WithoutStderrStillNamesCommand keeps the message useful
// when the tool failed silently (or is not installed at all).
func TestGhCommandError_WithoutStderrStillNamesCommand(t *testing.T) {
	_, execErr := exec.Command("cidx-no-such-binary-for-tests").Output()
	if execErr == nil {
		t.Fatal("fixture command was supposed to fail")
	}

	err := ghCommandError([]string{"pr", "list"}, execErr)
	if !strings.Contains(err.Error(), "gh pr list") {
		t.Errorf("error should name the invocation, got: %v", err)
	}
}
