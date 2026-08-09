package actions

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/vcs"
)

// `cidx release commit` commits the release notes a human just reviewed. It also
// staged `.cidx/release-version`, which this repository's own .gitignore lists
// beside `.cidx/tag-version` and `.cidx/tag-message` -- the scratch state the
// prepare step writes for the create step to read back.
//
// So the command could not run here at all: `git add` refuses an ignored path,
// and the release stopped on `hint: Use -f if you really want to add them`. Two
// parts of the same tool disagreed about whether that file belongs in history,
// and the disagreement only surfaced when someone tried to cut a release (#418).

// preparedRelease builds a repository holding a prepared release: notes staged
// for review, a version file, and the .gitignore that made this fail.
func preparedRelease(t *testing.T, version string) (*vcs.Repository, string) {
	t.Helper()

	workDir := t.TempDir()
	if err := vcs.Git(workDir, "--version").Run(); err != nil {
		t.Skipf("git is not available: %v", err)
	}

	run := func(args ...string) {
		t.Helper()
		if out, err := vcs.Git(workDir, args...).CombinedOutput(); err != nil {
			t.Fatalf("setup step `git %v` failed: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(workDir, name)), 0o755); err != nil {
			t.Fatalf("failed to create the directory for %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(workDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("failed to write %s: %v", name, err)
		}
	}

	run("init", "-b", "main")
	run("config", "user.email", "release@example.test")
	run("config", "user.name", "release")

	// The line this test exists for.
	write(".gitignore", ".cidx/release-version\n")
	run("add", "-A")
	run("commit", "-m", "chore: ignore the prepared version")

	write(GetReleaseNotesFile(version), "# Release v"+version+"\n\nNotes a human reviewed.\n")
	write(ReleaseVersionFile, version+"\n")

	repo, err := vcs.OpenRepository(workDir)
	if err != nil {
		t.Fatalf("failed to open the repository: %v", err)
	}
	return repo, workDir
}

// TestReleaseCommit_CommitsTheNotesWithoutStagingIgnoredState is the bug: the
// release could not be cut through cidx at all.
func TestReleaseCommit_CommitsTheNotesWithoutStagingIgnoredState(t *testing.T) {
	repo, workDir := preparedRelease(t, "3.2.0")

	if err := NewReleaseCommit(repo, false).Execute(context.Background()); err != nil {
		t.Fatalf("release commit failed on an ordinary prepared release: %v", err)
	}

	out, err := vcs.Git(workDir, "show", "--name-only", "--format=", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to read the commit: %v\n%s", err, out)
	}
	committed := strings.Fields(string(out))

	if len(committed) != 1 || committed[0] != GetReleaseNotesFile("3.2.0") {
		t.Errorf("the commit should hold the release notes and nothing else, it holds %v", committed)
	}
}

// The version file is scratch state the create step reads back, so it has to
// survive the commit -- deleting it instead of staging it would trade one
// broken release for another.
func TestReleaseCommit_LeavesThePreparedVersionInPlace(t *testing.T) {
	repo, workDir := preparedRelease(t, "3.2.0")

	if err := NewReleaseCommit(repo, false).Execute(context.Background()); err != nil {
		t.Fatalf("release commit failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, ReleaseVersionFile)); err != nil {
		t.Errorf("the prepared version is gone after the commit, and `release create` reads it back: %v", err)
	}
}

// TestReleaseCommitStagesOnlyTheNotes names the line, so a revert is reported as
// itself rather than as a puzzling failure inside a temporary repository.
func TestReleaseCommitStagesOnlyTheNotes(t *testing.T) {
	source, err := os.ReadFile("release_commit.go")
	if err != nil {
		t.Fatalf("failed to read release_commit.go: %v", err)
	}

	if strings.Contains(string(source), `"add", ReleaseVersionFile`) {
		t.Error(`release commit stages ReleaseVersionFile again. .gitignore lists it beside ` +
			`.cidx/tag-version and .cidx/tag-message -- it is scratch state between prepare and create -- ` +
			`so git refuses the add and no release can be cut through cidx (#418).`)
	}
}
