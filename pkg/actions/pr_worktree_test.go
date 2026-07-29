package actions

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeHolding(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantPath string
		wantHeld bool
	}{
		{
			name:     "current git wording",
			output:   "fatal: 'main' is already used by worktree at '/home/dev/cidx-baseline'\n",
			wantPath: "/home/dev/cidx-baseline",
			wantHeld: true,
		},
		{
			name:     "older git wording",
			output:   "fatal: 'main' is already checked out at '/home/dev/cidx-baseline'\n",
			wantPath: "/home/dev/cidx-baseline",
			wantHeld: true,
		},
		{
			name:   "an unrelated checkout failure is still a failure",
			output: "error: Your local changes to the following files would be overwritten by checkout:\n\tpkg/actions/pr.go\n",
		},
		{
			name:   "no output at all",
			output: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, held := worktreeHolding(tt.output)
			if held != tt.wantHeld {
				t.Fatalf("held = %v, want %v", held, tt.wantHeld)
			}
			if path != tt.wantPath {
				t.Errorf("path = %q, want %q", path, tt.wantPath)
			}
		})
	}
}

// TestWorktreeHoldingMatchesRealGit pins the detector to what the git on this
// machine actually prints, rather than to a message quoted from an issue.
func TestWorktreeHoldingMatchesRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	baseline := filepath.Join(root, "baseline")

	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		// Keep the user's identity and hooks out of the fixture.
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=cidx", "GIT_AUTHOR_EMAIL=cidx@example.test",
			"GIT_COMMITTER_NAME=cidx", "GIT_COMMITTER_EMAIL=cidx@example.test",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	git(root, "init", "-b", "main", "repo")
	git(repo, "commit", "--allow-empty", "-m", "initial")
	git(repo, "checkout", "-b", "feat/x")
	// A baseline worktree on main, exactly the setup issue #266 describes.
	git(repo, "worktree", "add", baseline, "main")

	// Now do what postMergeCleanup does first.
	checkout := exec.Command("git", "checkout", "main")
	checkout.Dir = repo
	output, err := checkout.CombinedOutput()
	if err == nil {
		t.Fatalf("expected git to refuse the checkout, it printed: %s", output)
	}

	path, held := worktreeHolding(string(output))
	if !held {
		t.Fatalf("real git output was not recognized as a worktree conflict: %q", output)
	}
	if !strings.Contains(path, "baseline") {
		t.Errorf("path = %q, want the baseline worktree", path)
	}
}
