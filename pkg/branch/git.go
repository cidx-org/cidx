package branch

import (
	"bufio"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitBranch represents raw git branch data
type GitBranch struct {
	Name       string
	IsRemote   bool
	IsCurrent  bool
	CommitHash string
	CommitDate time.Time
	Author     string
	Subject    string
}

// GetCurrentUser returns the current git user email
func GetCurrentUser() (string, error) {
	cmd := exec.Command("git", "config", "user.email")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetCurrentBranch returns the current branch name
func GetCurrentBranch() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// GetDefaultBranch tries to determine the default branch (main or master)
func GetDefaultBranch() string {
	// Try to get from remote HEAD
	cmd := exec.Command("git", "symbolic-ref", "refs/remotes/origin/HEAD")
	out, err := cmd.Output()
	if err == nil {
		ref := strings.TrimSpace(string(out))
		// refs/remotes/origin/main -> main
		parts := strings.Split(ref, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}

	// Fallback: check if main exists, otherwise master
	cmd = exec.Command("git", "rev-parse", "--verify", "refs/heads/main")
	if err := cmd.Run(); err == nil {
		return "main"
	}

	return "master"
}

// ListLocalBranches returns all local branches with their info
func ListLocalBranches() ([]GitBranch, error) {
	// Format: refname:short|objectname:short|committerdate:unix|authoremail|subject
	format := "%(refname:short)|%(objectname:short)|%(committerdate:unix)|%(authoremail)|%(subject)"
	cmd := exec.Command("git", "for-each-ref", "--format="+format, "refs/heads/")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list local branches: %w", err)
	}

	return parseBranchOutput(string(out), false)
}

// ListRemoteBranches returns all remote branches with their info
func ListRemoteBranches() ([]GitBranch, error) {
	format := "%(refname:short)|%(objectname:short)|%(committerdate:unix)|%(authoremail)|%(subject)"
	cmd := exec.Command("git", "for-each-ref", "--format="+format, "refs/remotes/origin/")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remote branches: %w", err)
	}

	branches, err := parseBranchOutput(string(out), true)
	if err != nil {
		return nil, err
	}

	// Filter out HEAD
	filtered := make([]GitBranch, 0, len(branches))
	for _, b := range branches {
		if !strings.HasSuffix(b.Name, "/HEAD") {
			filtered = append(filtered, b)
		}
	}

	return filtered, nil
}

// parseBranchOutput parses git for-each-ref output
func parseBranchOutput(output string, isRemote bool) ([]GitBranch, error) {
	var branches []GitBranch
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 5)
		if len(parts) < 5 {
			continue
		}

		name := parts[0]
		if isRemote {
			// Remove origin/ prefix for display
			name = strings.TrimPrefix(name, "origin/")
		}

		timestamp, _ := strconv.ParseInt(parts[2], 10, 64)
		commitDate := time.Unix(timestamp, 0)

		// Clean up author email (remove <> brackets)
		author := strings.Trim(parts[3], "<>")

		branches = append(branches, GitBranch{
			Name:       name,
			IsRemote:   isRemote,
			CommitHash: parts[1],
			CommitDate: commitDate,
			Author:     author,
			Subject:    parts[4],
		})
	}

	return branches, scanner.Err()
}

// IsBranchMerged checks if a branch has been merged into the target branch
func IsBranchMerged(branch, target string) bool {
	cmd := exec.Command("git", "branch", "--merged", target)
	out, err := cmd.Output()
	if err != nil {
		return false
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		merged := strings.TrimSpace(scanner.Text())
		merged = strings.TrimPrefix(merged, "* ") // Remove current branch marker
		if merged == branch {
			return true
		}
	}

	return false
}

// GetAheadBehind returns how many commits a branch is ahead/behind the target
func GetAheadBehind(branch, target string) (ahead, behind int, err error) {
	cmd := exec.Command("git", "rev-list", "--left-right", "--count", fmt.Sprintf("%s...%s", target, branch))
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, err
	}

	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) == 2 {
		behind, _ = strconv.Atoi(parts[0])
		ahead, _ = strconv.Atoi(parts[1])
	}

	return ahead, behind, nil
}

// GetTrackingBranch returns the remote tracking branch for a local branch
func GetTrackingBranch(branch string) string {
	cmd := exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.remote", branch))
	remoteOut, err := cmd.Output()
	if err != nil {
		return ""
	}
	remote := strings.TrimSpace(string(remoteOut))

	cmd = exec.Command("git", "config", "--get", fmt.Sprintf("branch.%s.merge", branch))
	mergeOut, err := cmd.Output()
	if err != nil {
		return ""
	}
	merge := strings.TrimSpace(string(mergeOut))
	merge = strings.TrimPrefix(merge, "refs/heads/")

	if remote != "" && merge != "" {
		return fmt.Sprintf("%s/%s", remote, merge)
	}
	return ""
}

// FetchPrune fetches and prunes remote branches
func FetchPrune() error {
	cmd := exec.Command("git", "fetch", "--prune")
	return cmd.Run()
}

// GetRemoteBranchInfo returns info for a specific remote branch
func GetRemoteBranchInfo(branchName string) (*GitBranch, error) {
	ref := fmt.Sprintf("refs/remotes/origin/%s", branchName)
	format := "%(refname:short)|%(objectname:short)|%(committerdate:unix)|%(authoremail)|%(subject)"
	cmd := exec.Command("git", "for-each-ref", "--format="+format, ref)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get remote branch info: %w", err)
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return nil, nil // Branch doesn't exist on remote
	}

	branches, err := parseBranchOutput(output, true)
	if err != nil || len(branches) == 0 {
		return nil, err
	}

	return &branches[0], nil
}

// BuildRemoteBranchMap creates a map of branch name -> GitBranch for remote branches
func BuildRemoteBranchMap(remoteBranches []GitBranch) map[string]GitBranch {
	result := make(map[string]GitBranch)
	for _, rb := range remoteBranches {
		result[rb.Name] = rb
	}
	return result
}

// DeleteLocalBranch deletes a local branch
func DeleteLocalBranch(name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	cmd := exec.Command("git", "branch", flag, name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete local branch %s: %s", name, string(output))
	}
	return nil
}

// DeleteRemoteBranch deletes a remote branch
func DeleteRemoteBranch(name string) error {
	cmd := exec.Command("git", "push", "origin", "--delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete remote branch %s: %s", name, string(output))
	}
	return nil
}
