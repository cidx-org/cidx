package vcs

import (
	"fmt"
	"os/exec"
	"regexp"

	"github.com/go-git/go-git/v5"
)

// Repository represents a Git repository
type Repository struct {
	repo *git.Repository
}

// OpenRepository opens a git repository at the given path
func OpenRepository(path string) (*Repository, error) {
	r, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	return &Repository{repo: r}, nil
}

// Commit creates a commit with all changes using git binary
// This ensures pre-commit hooks are executed
func (r *Repository) Commit(message string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	workDir := w.Filesystem.Root()

	// Add all changes using git binary
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = workDir
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add changes: %w\n%s", err, output)
	}

	// Commit using git binary (ensures pre-commit hooks run)
	commitCmd := exec.Command("git", "commit", "-m", message)
	commitCmd.Dir = workDir
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit: %w\n%s", err, output)
	}

	return nil
}

// Push pushes commits to remote using git binary
func (r *Repository) Push() error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	workDir := w.Filesystem.Root()

	// Push using git binary
	pushCmd := exec.Command("git", "push")
	pushCmd.Dir = workDir
	if output, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to push: %w\n%s", err, output)
	}

	return nil
}

// GetRemoteInfo extracts owner and repo name from remote origin URL
func (r *Repository) GetRemoteInfo() (owner, repo string, err error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	if len(remote.Config().URLs) == 0 {
		return "", "", fmt.Errorf("no URLs configured for origin remote")
	}

	url := remote.Config().URLs[0]

	// Parse SSH URL: git@github.com:owner/repo.git
	sshPattern := regexp.MustCompile(`git@[^:]+:([^/]+)/(.+?)(?:\.git)?$`)
	if matches := sshPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2], nil
	}

	// Parse HTTPS URL: https://github.com/owner/repo.git
	httpsPattern := regexp.MustCompile(`https://[^/]+/([^/]+)/(.+?)(?:\.git)?$`)
	if matches := httpsPattern.FindStringSubmatch(url); len(matches) == 3 {
		return matches[1], matches[2], nil
	}

	return "", "", fmt.Errorf("unable to parse remote URL: %s", url)
}

// GetCurrentBranch returns the name of the current branch
func (r *Repository) GetCurrentBranch() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	// Extract branch name from refs/heads/branch-name
	branchName := head.Name().Short()
	return branchName, nil
}

// HasChanges checks if there are uncommitted changes (modified or staged files only, ignoring untracked)
func (r *Repository) HasChanges() (bool, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	// Check only for modified or staged files, ignore untracked files
	for _, fileStatus := range status {
		if fileStatus.Worktree != git.Untracked && fileStatus.Worktree != git.Unmodified {
			return true, nil
		}
		if fileStatus.Staging != git.Untracked && fileStatus.Staging != git.Unmodified {
			return true, nil
		}
	}

	return false, nil
}

// GetWorkDir returns the working directory path of the repository
func (r *Repository) GetWorkDir() (string, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	return w.Filesystem.Root(), nil
}
