package vcs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	addCmd := Git(workDir, "add", ".")
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add changes: %w\n%s", err, output)
	}

	// Commit using git binary (ensures pre-commit hooks run)
	commitCmd := Git(workDir, "commit", "-m", message)
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to commit: %w\n%s", err, output)
	}

	return nil
}

// Push pushes commits to remote using git binary.
// Automatically sets upstream for new branches.
func (r *Repository) Push() error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	workDir := w.Filesystem.Root()

	// Try regular push first
	pushCmd := Git(workDir, "push")
	output, err := pushCmd.CombinedOutput()
	if err == nil {
		return nil
	}

	// If push failed due to no upstream, set it automatically
	outStr := string(output)
	if strings.Contains(outStr, "no upstream branch") || strings.Contains(outStr, "has no upstream") {
		branch, branchErr := r.GetCurrentBranch()
		if branchErr != nil {
			return fmt.Errorf("failed to push: %w\n%s", err, output)
		}

		upstreamCmd := Git(workDir, "push", "--set-upstream", "origin", branch)
		if upOutput, upErr := upstreamCmd.CombinedOutput(); upErr != nil {
			return fmt.Errorf("failed to push with upstream: %w\n%s", upErr, upOutput)
		}
		return nil
	}

	return fmt.Errorf("failed to push: %w\n%s", err, output)
}

// Pull pulls latest changes from remote using git binary
func (r *Repository) Pull() error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	workDir := w.Filesystem.Root()

	// Pull using git binary
	pullCmd := Git(workDir, "pull")
	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to pull: %w\n%s", err, output)
	}

	return nil
}

// Checkout switches to a different branch using git binary
func (r *Repository) Checkout(branch string) error {
	w, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	workDir := w.Filesystem.Root()

	// Checkout using git binary
	checkoutCmd := Git(workDir, "checkout", branch)
	if output, err := checkoutCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout branch '%s': %w\n%s", branch, err, output)
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

// GetHeadSHA returns the commit SHA of the current HEAD
func (r *Repository) GetHeadSHA() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to get HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

// HasChanges checks if there are uncommitted changes (modified or staged files only, ignoring untracked)
//
// This is the "would a release/tag be built from a dirty tree?" question: a
// stray scratch file next to the source must not block `pr create`, `tag
// create` or `release create`. Use HasChangesIncludingUntracked to ask "is
// there anything to commit?".
func (r *Repository) HasChanges() (bool, error) {
	return r.hasChanges(false)
}

// HasChangesIncludingUntracked reports uncommitted changes the way Commit()
// sees them. Commit() runs `git add .`, so a brand-new file is something to
// commit; cpw asked HasChanges instead and answered "No changes to commit"
// while quietly leaving the user's new file behind (issue #180).
func (r *Repository) HasChangesIncludingUntracked() (bool, error) {
	return r.hasChanges(true)
}

// hasChanges walks the worktree status. Ignored files are already excluded by
// go-git's Status(), so includeUntracked only ever sees files `git add .`
// would stage.
func (r *Repository) hasChanges(includeUntracked bool) (bool, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("failed to get worktree: %w", err)
	}

	status, err := w.Status()
	if err != nil {
		return false, fmt.Errorf("failed to get status: %w", err)
	}

	for _, fileStatus := range status {
		if includeUntracked {
			if fileStatus.Worktree != git.Unmodified || fileStatus.Staging != git.Unmodified {
				return true, nil
			}
			continue
		}
		if fileStatus.Worktree != git.Untracked && fileStatus.Worktree != git.Unmodified {
			return true, nil
		}
		if fileStatus.Staging != git.Untracked && fileStatus.Staging != git.Unmodified {
			return true, nil
		}
	}

	return false, nil
}

// HasActivePreCommitHook reports whether `git commit` will run a pre-commit
// hook. It asks git for the effective hooks directory, so it answers for
// core.hooksPath (how this repository installs .githooks) as well as for the
// default .git/hooks — which ships only .sample files and so never matches.
//
// Commit() shells out to the git binary precisely so hooks fire. cpw runs the
// code phase before committing (issue #307), and a hook that runs the same
// phase would run it twice: ~20 seconds paid for an answer already known.
// Nothing is inferred about what the hook checks — if a repository installed
// one, it is the repository's own gate and cpw stands aside for it.
func (r *Repository) HasActivePreCommitHook() bool {
	workDir, err := r.GetWorkDir()
	if err != nil {
		return false
	}

	cmd := Git(workDir, "rev-parse", "--git-path", "hooks")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	hooks := strings.TrimSpace(string(output))
	if !filepath.IsAbs(hooks) {
		hooks = filepath.Join(workDir, hooks)
	}

	info, err := os.Stat(filepath.Join(hooks, "pre-commit"))
	if err != nil {
		return false
	}

	// git ignores a hook it cannot execute, so an unset executable bit means
	// no hook — the same reading git itself makes.
	return !info.IsDir() && info.Mode()&0o111 != 0
}

// GetWorkDir returns the working directory path of the repository
func (r *Repository) GetWorkDir() (string, error) {
	w, err := r.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	return w.Filesystem.Root(), nil
}

// GetRemoteURL returns the URL of the origin remote
func (r *Repository) GetRemoteURL() (string, error) {
	remote, err := r.repo.Remote("origin")
	if err != nil {
		return "", fmt.Errorf("failed to get origin remote: %w", err)
	}

	if len(remote.Config().URLs) == 0 {
		return "", fmt.Errorf("no URLs configured for origin remote")
	}

	return remote.Config().URLs[0], nil
}
