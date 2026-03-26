package github

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/google/go-github/v76/github"
)

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
// GetPullRequestDetails returns comprehensive PR details for TUI display
func (c *Client) GetPullRequestDetails(ctx context.Context, prNumber int) (*remote.PullRequestDetails, error) {
	// Get PR details
	pr, _, err := c.client.PullRequests.Get(ctx, c.owner, c.repo, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	details := &remote.PullRequestDetails{
		Number:       pr.GetNumber(),
		Title:        pr.GetTitle(),
		Body:         pr.GetBody(),
		State:        pr.GetState(),
		Draft:        pr.GetDraft(),
		HeadBranch:   pr.GetHead().GetRef(),
		BaseBranch:   pr.GetBase().GetRef(),
		HeadSHA:      pr.GetHead().GetSHA(),
		Author:       pr.GetUser().GetLogin(),
		CreatedAt:    pr.GetCreatedAt().Time,
		UpdatedAt:    pr.GetUpdatedAt().Time,
		Additions:    pr.GetAdditions(),
		Deletions:    pr.GetDeletions(),
		ChangedFiles: pr.GetChangedFiles(),
		Mergeable:    pr.GetMergeable(),
		URL:          pr.GetHTMLURL(),
	}

	// Get labels
	for _, label := range pr.Labels {
		details.Labels = append(details.Labels, label.GetName())
	}

	// Get reviews
	reviews, _, err := c.client.PullRequests.ListReviews(ctx, c.owner, c.repo, prNumber, &github.ListOptions{PerPage: 100})
	if err == nil {
		// Track latest review state per user
		reviewerStates := make(map[string]string)
		for _, review := range reviews {
			login := review.GetUser().GetLogin()
			state := review.GetState()
			// Only update if this is a meaningful state (APPROVED, CHANGES_REQUESTED, etc.)
			if state != "COMMENTED" && state != "DISMISSED" {
				reviewerStates[login] = state
			} else if _, exists := reviewerStates[login]; !exists {
				reviewerStates[login] = state
			}
		}
		for login, state := range reviewerStates {
			details.Reviewers = append(details.Reviewers, remote.ReviewerStatus{
				Login: login,
				State: state,
			})
		}
	}

	// Get commits
	commits, _, err := c.client.PullRequests.ListCommits(ctx, c.owner, c.repo, prNumber, &github.ListOptions{PerPage: 100})
	if err == nil {
		for _, commit := range commits {
			details.Commits = append(details.Commits, remote.CommitInfo{
				SHA:     commit.GetSHA()[:7],
				Message: strings.Split(commit.GetCommit().GetMessage(), "\n")[0], // First line only
				Author:  commit.GetCommit().GetAuthor().GetName(),
				Date:    commit.GetCommit().GetAuthor().GetDate().Time,
			})
		}
	}

	// Get linked issues from PR body (common patterns: "Fixes #123", "Closes #456", "Resolves #789")
	details.LinkedIssues = c.extractLinkedIssues(ctx, pr.GetBody())

	return details, nil
}

// extractLinkedIssues parses PR body for linked issues and fetches their details
func (c *Client) extractLinkedIssues(ctx context.Context, body string) []remote.LinkedIssue {
	var issues []remote.LinkedIssue
	seen := make(map[int]bool)

	// Match patterns like "Fixes #123", "Closes #456", "Resolves #789", "Related to #111"
	patterns := []string{
		`(?i)(?:fix(?:es)?|close[sd]?|resolve[sd]?|related\s+to)\s*#(\d+)`,
		`#(\d+)`, // Also catch plain #number references
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(body, -1)
		for _, match := range matches {
			if len(match) >= 2 {
				num, err := strconv.Atoi(match[1])
				if err != nil || seen[num] {
					continue
				}
				seen[num] = true

				// Fetch issue details
				issue, _, err := c.client.Issues.Get(ctx, c.owner, c.repo, num)
				if err == nil && !issue.IsPullRequest() {
					var labels []string
					for _, l := range issue.Labels {
						labels = append(labels, l.GetName())
					}
					var assignees []string
					for _, a := range issue.Assignees {
						assignees = append(assignees, a.GetLogin())
					}
					issues = append(issues, remote.LinkedIssue{
						Number:    num,
						Title:     issue.GetTitle(),
						Body:      issue.GetBody(),
						State:     issue.GetState(),
						URL:       issue.GetHTMLURL(),
						Labels:    labels,
						Assignees: assignees,
						CreatedAt: issue.GetCreatedAt().Time,
						UpdatedAt: issue.GetUpdatedAt().Time,
						Author:    issue.GetUser().GetLogin(),
					})
				}
			}
		}
	}

	return issues
}
