package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a minimal git repo in a temp directory with an origin remote
func initTestRepo(t *testing.T, remoteURL string) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "remote", "add", "origin", remoteURL},
	}

	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("command %v failed: %v\n%s", args, err, output)
		}
	}

	// Create an initial commit so HEAD exists
	emptyFile := filepath.Join(dir, ".gitkeep")
	if err := os.WriteFile(emptyFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, output)
	}
	cmd = exec.Command("git", "commit", "-m", "init", "--no-gpg-sign")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, output)
	}

	return dir
}

func TestOpenRepository(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestOpenRepository_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := OpenRepository(dir)
	if err == nil {
		t.Error("expected error for non-repo directory")
	}
}

func TestGetRemoteInfo_HTTPS(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{
			name:      "GitHub HTTPS",
			url:       "https://github.com/cidx-org/cidx.git",
			wantOwner: "cidx-org",
			wantRepo:  "cidx",
		},
		{
			name:      "GitHub HTTPS without .git",
			url:       "https://github.com/cidx-org/cidx",
			wantOwner: "cidx-org",
			wantRepo:  "cidx",
		},
		{
			name:      "GitLab HTTPS",
			url:       "https://gitlab.com/mygroup/myproject.git",
			wantOwner: "mygroup",
			wantRepo:  "myproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initTestRepo(t, tt.url)
			repo, err := OpenRepository(dir)
			if err != nil {
				t.Fatal(err)
			}

			owner, repoName, err := repo.GetRemoteInfo()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if owner != tt.wantOwner {
				t.Errorf("owner: got %q, want %q", owner, tt.wantOwner)
			}
			if repoName != tt.wantRepo {
				t.Errorf("repo: got %q, want %q", repoName, tt.wantRepo)
			}
		})
	}
}

func TestGetRemoteInfo_SSH(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
	}{
		{
			name:      "GitHub SSH",
			url:       "git@github.com:cidx-org/cidx.git",
			wantOwner: "cidx-org",
			wantRepo:  "cidx",
		},
		{
			name:      "GitHub SSH without .git",
			url:       "git@github.com:cidx-org/cidx",
			wantOwner: "cidx-org",
			wantRepo:  "cidx",
		},
		{
			name:      "GitLab SSH",
			url:       "git@gitlab.com:mygroup/myproject.git",
			wantOwner: "mygroup",
			wantRepo:  "myproject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := initTestRepo(t, tt.url)
			repo, err := OpenRepository(dir)
			if err != nil {
				t.Fatal(err)
			}

			owner, repoName, err := repo.GetRemoteInfo()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if owner != tt.wantOwner {
				t.Errorf("owner: got %q, want %q", owner, tt.wantOwner)
			}
			if repoName != tt.wantRepo {
				t.Errorf("repo: got %q, want %q", repoName, tt.wantRepo)
			}
		})
	}
}

func TestGetCurrentBranch(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	branch, err := repo.GetCurrentBranch()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default branch is typically "master" or "main" depending on git config
	if branch == "" {
		t.Error("expected non-empty branch name")
	}
}

func TestGetRemoteURL(t *testing.T) {
	expectedURL := "https://github.com/cidx-org/cidx.git"
	dir := initTestRepo(t, expectedURL)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	url, err := repo.GetRemoteURL()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if url != expectedURL {
		t.Errorf("got %q, want %q", url, expectedURL)
	}
}

func TestGetWorkDir(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	workDir, err := repo.GetWorkDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if workDir == "" {
		t.Error("expected non-empty work dir")
	}
}

func TestHasChanges_Clean(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := repo.HasChanges()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasChanges {
		t.Error("expected no changes in fresh repo")
	}
}

func TestHasChanges_WithModifiedFile(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	// Modify existing file
	if err := os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := repo.HasChanges()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasChanges {
		t.Error("expected changes after modifying a tracked file")
	}
}

// writeUntracked drops a brand-new file (git status "??") into dir.
func writeUntracked(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "brand_new.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

// TestHasChanges_IgnoresUntracked pins the narrow contract the release, tag and
// pr guards rely on: a stray scratch file is not a reason to refuse a tag.
func TestHasChanges_IgnoresUntracked(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")
	writeUntracked(t, dir)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := repo.HasChanges()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hasChanges {
		t.Error("HasChanges must stay tracked-only: an untracked file is not a dirty tree for release/tag/pr")
	}
}

// TestHasChangesIncludingUntracked_SeesNewFile is issue #180: cpw asked the
// tracked-only question, so a brand-new file produced "No changes to commit"
// and was silently left behind — even though Commit() runs `git add .` and
// would have committed it.
func TestHasChangesIncludingUntracked_SeesNewFile(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")
	writeUntracked(t, dir)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := repo.HasChangesIncludingUntracked()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasChanges {
		t.Error("expected an untracked file to count as something to commit")
	}
}

// TestHasChangesIncludingUntracked_CommitPicksItUp closes the loop: the answer
// is only useful if Commit() actually captures the file it reported.
func TestHasChangesIncludingUntracked_CommitPicksItUp(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")
	writeUntracked(t, dir)

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := repo.Commit("chore: add brand new file"); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	cmd := exec.Command("git", "show", "--name-only", "--format=", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git show failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "brand_new.go") {
		t.Errorf("commit does not contain the untracked file, got:\n%s", out)
	}

	// And the tree is clean afterwards.
	hasChanges, err := repo.HasChangesIncludingUntracked()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("expected a clean tree after committing the untracked file")
	}
}

// TestHasChangesIncludingUntracked_IgnoresGitignored guards the obvious
// regression: build output listed in .gitignore must not make cpw commit.
func TestHasChangesIncludingUntracked_IgnoresGitignored(t *testing.T) {
	dir := initTestRepo(t, "https://github.com/owner/repo.git")

	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "add", ".gitignore")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "ignore bin", "--no-gpg-sign")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	if err := os.Mkdir(filepath.Join(dir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bin", "cidx"), []byte("binary"), 0644); err != nil {
		t.Fatal(err)
	}

	repo, err := OpenRepository(dir)
	if err != nil {
		t.Fatal(err)
	}

	hasChanges, err := repo.HasChangesIncludingUntracked()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("gitignored files must not count as changes to commit")
	}
}
