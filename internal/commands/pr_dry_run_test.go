package commands

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// TestPRCreateDryRunNeverBuildsAProvider is the guard behind issue #350.
//
// `pr create` was wired through withRepoAndProvider, which resolves the remote
// URL and looks up a token before the action runs. Both are local lookups --
// no network, no mutation, so outside what #276 fixed -- but they still gated
// the preview: a repository whose origin is a filesystem path failed with
// `unable to parse remote URL` before printing a line. The dry-run path never
// uses a provider, so it must never build one.
//
// The repository built here has exactly that origin, so the assertion holds
// twice: nothing is allowed to call createProvider, and a call would fail
// anyway.
func TestPRCreateDryRunNeverBuildsAProvider(t *testing.T) {
	repositoryOnMainWithUnusableRemote(t)

	original := createProvider
	t.Cleanup(func() { createProvider = original })
	createProvider = func(repo *vcs.Repository) (remote.Provider, error) {
		t.Error("`cidx pr create --dry-run` built a remote provider it never uses")
		return original(repo)
	}

	if err := NewApp().Run([]string{"cidx", "pr", "create", "--dry-run", "feat: something"}); err != nil {
		t.Fatalf("the preview failed: %v", err)
	}
}

// repositoryOnMainWithUnusableRemote builds a one-commit repository on main
// whose origin is a path, and not a path that exists, and makes it the working
// directory -- which is where the commands look for a repository.
func repositoryOnMainWithUnusableRemote(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	for _, args := range [][]string{
		{"-c", "init.defaultBranch=main", "init", "-q", dir},
		{"-C", dir, "config", "user.email", "test@example.test"},
		{"-C", dir, "config", "user.name", "cidx test"},
		{"-C", dir, "commit", "-q", "--allow-empty", "-m", "chore: root commit"},
		{"-C", dir, "remote", "add", "origin", filepath.Join(dir, "there-is-no-remote-here.git")},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	t.Chdir(dir)
	return dir
}
