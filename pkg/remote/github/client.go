package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// Client implements remote.Provider for GitHub Actions
type Client struct {
	client *github.Client
	owner  string
	repo   string
}

// NewClient creates a new GitHub client with token authentication
func NewClient(token, owner, repo string) *Client {
	client := github.NewClient(nil).WithAuthToken(token)

	return &Client{
		client: client,
		owner:  owner,
		repo:   repo,
	}
}

// NewClientFromEnv creates a GitHub client using environment variables and git remote
func NewClientFromEnv() (*Client, error) {
	// Get token from environment
	token := getEnvToken()
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN not set")
	}

	// Get owner/repo from git remote
	owner, repo, err := getRepoFromRemote()
	if err != nil {
		return nil, fmt.Errorf("failed to detect repository: %w", err)
	}

	return NewClient(token, owner, repo), nil
}

// getEnvToken returns GitHub token from environment or gh CLI
func getEnvToken() string {
	// 1. Check environment variables first
	for _, key := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if token := os.Getenv(key); token != "" {
			return token
		}
	}

	// 2. Fallback to gh CLI auth
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out))
	}

	return ""
}

// getRepoFromRemote extracts owner/repo from git remote URL
func getRepoFromRemote() (owner, repo string, err error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to get remote URL: %w", err)
	}

	url := string(out[:len(out)-1]) // Remove trailing newline
	return parseGitHubURL(url)
}

// parseGitHubURL extracts owner/repo from various GitHub URL formats
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle SSH format: git@github.com:owner/repo.git
	if len(url) > 15 && url[:15] == "git@github.com:" {
		path := url[15:]
		path = trimSuffix(path, ".git")
		parts := splitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	// Handle HTTPS format: https://github.com/owner/repo.git
	if len(url) > 19 && url[:19] == "https://github.com/" {
		path := url[19:]
		path = trimSuffix(path, ".git")
		parts := splitN(path, "/", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
	}

	return "", "", fmt.Errorf("unsupported URL format: %s", url)
}

// trimSuffix removes suffix from string
func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// splitN splits string into at most n parts
func splitN(s, sep string, n int) []string {
	var parts []string
	for i := 0; i < n-1; i++ {
		idx := -1
		for j := 0; j < len(s)-len(sep)+1; j++ {
			if s[j:j+len(sep)] == sep {
				idx = j
				break
			}
		}
		if idx == -1 {
			break
		}
		parts = append(parts, s[:idx])
		s = s[idx+len(sep):]
	}
	parts = append(parts, s)
	return parts
}

// GetLatestWorkflow returns the most recent workflow run for a branch
func (c *Client) GetLatestWorkflow(ctx context.Context, branch string) (*remote.Workflow, error) {
	runs, _, err := c.client.Actions.ListWorkflowRunsByFileName(
		ctx,
		c.owner,
		c.repo,
		"cidx.yml",
		&github.ListWorkflowRunsOptions{
			Branch:      branch,
			ListOptions: github.ListOptions{PerPage: 1},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	if len(runs.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no workflow runs found for branch %s", branch)
	}

	return c.convertWorkflow(ctx, runs.WorkflowRuns[0])
}

// WatchWorkflow streams updates for a running workflow
func (c *Client) WatchWorkflow(ctx context.Context, workflowID string) (<-chan remote.WorkflowUpdate, error) {
	updates := make(chan remote.WorkflowUpdate, 1)

	id, err := strconv.ParseInt(workflowID, 10, 64)
	if err != nil {
		close(updates)
		return updates, fmt.Errorf("invalid workflow ID: %w", err)
	}

	go func() {
		defer close(updates)

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				run, _, err := c.client.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, id)
				if err != nil {
					updates <- remote.WorkflowUpdate{Error: err}
					return
				}

				workflow, err := c.convertWorkflow(ctx, run)
				if err != nil {
					updates <- remote.WorkflowUpdate{Error: err}
					return
				}

				// Send update only if status changed
				currentStatus := fmt.Sprintf("%s:%s", workflow.Status, workflow.Conclusion)
				if currentStatus != lastStatus {
					updates <- remote.WorkflowUpdate{Workflow: workflow}
					lastStatus = currentStatus
				}

				// Stop when workflow completes
				if workflow.Status == "completed" {
					return
				}
			}
		}
	}()

	return updates, nil
}

// convertWorkflow converts GitHub workflow run to our Workflow type
func (c *Client) convertWorkflow(ctx context.Context, run *github.WorkflowRun) (*remote.Workflow, error) {
	workflow := &remote.Workflow{
		ID:         strconv.FormatInt(run.GetID(), 10),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
		URL:        run.GetHTMLURL(),
		Jobs:       []remote.Job{},
	}

	// Fetch jobs for this workflow
	jobs, _, err := c.client.Actions.ListWorkflowJobs(
		ctx,
		c.owner,
		c.repo,
		run.GetID(),
		&github.ListWorkflowJobsOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}

	for _, job := range jobs.Jobs {
		workflow.Jobs = append(workflow.Jobs, remote.Job{
			Name:       job.GetName(),
			Status:     job.GetStatus(),
			Conclusion: job.GetConclusion(),
		})
	}

	return workflow, nil
}

// CreatePullRequest creates a new pull request
func (c *Client) CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (int, string, error) {
	pr := &github.NewPullRequest{
		Title: github.Ptr(title),
		Body:  github.Ptr(body),
		Head:  github.Ptr(head),
		Base:  github.Ptr(base),
		Draft: github.Ptr(draft),
	}

	createdPR, _, err := c.client.PullRequests.Create(ctx, c.owner, c.repo, pr)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create pull request: %w", err)
	}

	return createdPR.GetNumber(), createdPR.GetHTMLURL(), nil
}

// MarkPullRequestReady marks a draft PR as ready for review
func (c *Client) MarkPullRequestReady(ctx context.Context, prNumber int) error {
	// GitHub's REST API doesn't support converting draft to ready directly
	// We need to use GraphQL API, which is best accessed via gh CLI
	// This is consistent with our hybrid approach: native tools for complex operations

	// Use gh CLI to mark PR as ready (uses GraphQL API internally)
	cmd := exec.Command("gh", "pr", "ready", strconv.Itoa(prNumber), "--repo", fmt.Sprintf("%s/%s", c.owner, c.repo))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to mark PR as ready: %w\n%s", err, output)
	}

	return nil
}

// GetPullRequestByBranch finds a PR for the given head branch
func (c *Client) GetPullRequestByBranch(ctx context.Context, branch string) (int, string, error) {
	prs, _, err := c.client.PullRequests.List(ctx, c.owner, c.repo, &github.PullRequestListOptions{
		Head:  fmt.Sprintf("%s:%s", c.owner, branch),
		State: "open",
	})
	if err != nil {
		return 0, "", fmt.Errorf("failed to list pull requests: %w", err)
	}

	if len(prs) == 0 {
		return 0, "", fmt.Errorf("no open pull request found for branch %s", branch)
	}

	return prs[0].GetNumber(), prs[0].GetHTMLURL(), nil
}

// MergePullRequest merges a pull request
func (c *Client) MergePullRequest(ctx context.Context, prNumber int, method string) error {
	// Validate merge method
	validMethods := map[string]bool{
		"merge":  true,
		"squash": true,
		"rebase": true,
	}
	if !validMethods[method] {
		return fmt.Errorf("invalid merge method: %s (valid: merge, squash, rebase)", method)
	}

	// Merge the PR
	options := &github.PullRequestOptions{
		MergeMethod: method,
	}

	_, _, err := c.client.PullRequests.Merge(ctx, c.owner, c.repo, prNumber, "", options)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	return nil
}

// GetPullRequestChecks returns the status of all checks/workflows for a PR
func (c *Client) GetPullRequestChecks(ctx context.Context, prNumber int) (*remote.PRChecks, error) {
	// Get PR details to get the head SHA
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	headSHA := pr.GetHead().GetSHA()

	checks := &remote.PRChecks{
		HeadSHA:      headSHA,
		Checks:       []remote.CheckRun{},
		StatusChecks: []remote.StatusCheck{},
	}

	// Get check runs (GitHub Actions)
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, headSHA, &github.ListCheckRunsOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list check runs: %w", err)
	}

	for _, run := range checkRuns.CheckRuns {
		check := remote.CheckRun{
			Name:       run.GetName(),
			Status:     run.GetStatus(),
			Conclusion: run.GetConclusion(),
			URL:        run.GetHTMLURL(),
		}
		checks.Checks = append(checks.Checks, check)

		// Count by status
		checks.TotalCount++
		if run.GetStatus() != "completed" {
			checks.Pending++
		} else if run.GetConclusion() == "success" {
			checks.Success++
		} else {
			checks.Failure++
		}
	}

	// Get commit status checks (legacy status API)
	statuses, _, err := c.client.Repositories.GetCombinedStatus(ctx, c.owner, c.repo, headSHA, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get combined status: %w", err)
	}

	for _, status := range statuses.Statuses {
		statusCheck := remote.StatusCheck{
			Context: status.GetContext(),
			State:   status.GetState(),
			URL:     status.GetTargetURL(),
		}
		checks.StatusChecks = append(checks.StatusChecks, statusCheck)

		// Count by status
		checks.TotalCount++
		if status.GetState() == "pending" {
			checks.Pending++
		} else if status.GetState() == "success" {
			checks.Success++
		} else {
			checks.Failure++
		}
	}

	// Determine overall status
	if checks.Failure > 0 {
		checks.Status = "failure"
	} else if checks.Pending > 0 {
		checks.Status = "pending"
	} else {
		checks.Status = "success"
	}

	return checks, nil
}

// WaitForChecksToStart waits for CI checks to start for a PR
// This solves the race condition where CI hasn't started yet when we query
func (c *Client) WaitForChecksToStart(ctx context.Context, prNumber int, timeout time.Duration) (string, *remote.PRChecks, error) {
	// Get PR details to get the head SHA we're waiting for
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	expectedSHA := pr.GetHead().GetSHA()
	shortSHA := expectedSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for checks to appear
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-timeoutCtx.Done():
			// Timeout reached - return current state with warning
			checks, err := c.GetPullRequestChecks(ctx, prNumber)
			if err != nil {
				return expectedSHA, nil, fmt.Errorf("timeout waiting for CI to start (waited %v): %w", timeout, err)
			}
			// If no checks after timeout, it might be a repo without CI
			if checks.TotalCount == 0 {
				return expectedSHA, checks, fmt.Errorf("no CI checks found after %v - repository may not have CI configured", timeout)
			}
			return expectedSHA, checks, nil

		case <-ticker.C:
			checks, err := c.GetPullRequestChecks(ctx, prNumber)
			if err != nil {
				continue // Retry on transient errors
			}

			// Verify we're checking the right commit
			if checks.HeadSHA != expectedSHA {
				// SHA mismatch - CI might be running for old commit, wait for new one
				continue
			}

			// Check if CI has started (at least one check exists)
			if checks.TotalCount > 0 {
				elapsed := time.Since(startTime)
				if elapsed > 2*time.Second {
					// Only log if we actually waited
					_ = elapsed // Will be used by caller for logging
				}
				return expectedSHA, checks, nil
			}

			// No checks yet, continue waiting
		}
	}
}

// GetPullRequest returns a single pull request by number
func (c *Client) GetPullRequest(ctx context.Context, prNumber int) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	return pr, nil
}

// GetPullRequestReviews returns reviews for a pull request
func (c *Client) GetPullRequestReviews(ctx context.Context, prNumber int) ([]*github.PullRequestReview, error) {
	reviews, _, err := c.client.PullRequests.ListReviews(ctx, c.owner, c.repo, prNumber, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request reviews: %w", err)
	}
	return reviews, nil
}

// ListPullRequests lists pull requests with the given state (open, closed, all)
func (c *Client) ListPullRequests(ctx context.Context, state string) ([]*github.PullRequest, error) {
	opts := &github.PullRequestListOptions{
		State:       state,
		ListOptions: github.ListOptions{PerPage: 100},
	}

	prs, _, err := c.client.PullRequests.List(ctx, c.owner, c.repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	return prs, nil
}

// WatchPullRequestChecks streams updates for PR checks until all complete
func (c *Client) WatchPullRequestChecks(ctx context.Context, prNumber int) (<-chan remote.PRChecksUpdate, error) {
	updates := make(chan remote.PRChecksUpdate, 1)

	go func() {
		defer close(updates)

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		var lastStatus string

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				checks, err := c.GetPullRequestChecks(ctx, prNumber)
				if err != nil {
					updates <- remote.PRChecksUpdate{Error: err}
					return
				}

				// Send update only if status changed
				currentStatus := fmt.Sprintf("%s:%d:%d:%d", checks.Status, checks.Pending, checks.Success, checks.Failure)
				if currentStatus != lastStatus {
					updates <- remote.PRChecksUpdate{Checks: checks}
					lastStatus = currentStatus
				}

				// Stop when all checks complete
				if checks.Pending == 0 {
					return
				}
			}
		}
	}()

	return updates, nil
}
