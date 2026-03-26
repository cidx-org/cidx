package github

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/google/go-github/v76/github"
)

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
		UpdatedAt:    time.Now(),
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
			ID:          run.GetID(),
			Name:        run.GetName(),
			Status:      run.GetStatus(),
			Conclusion:  run.GetConclusion(),
			URL:         run.GetHTMLURL(),
			StartedAt:   run.GetStartedAt().Time,
			CompletedAt: run.GetCompletedAt().Time,
		}

		// If failed, try to get the failed step name from annotations
		if run.GetConclusion() == "failure" && run.Output != nil {
			if run.Output.Summary != nil && *run.Output.Summary != "" {
				// Truncate summary to first 200 chars for error preview
				summary := *run.Output.Summary
				if len(summary) > 200 {
					summary = summary[:200] + "..."
				}
				check.ErrorLog = summary
			}
		}

		checks.Checks = append(checks.Checks, check)

		// Count by status
		checks.TotalCount++
		switch run.GetStatus() {
		case "queued":
			checks.Queued++
			checks.Pending++
		case "in_progress":
			checks.InProgress++
			checks.Pending++
		case "completed":
			if run.GetConclusion() == "success" {
				checks.Success++
			} else {
				checks.Failure++
			}
		default:
			checks.Pending++
		}
	}

	// Get commit status checks (legacy status API)
	statuses, _, err := c.client.Repositories.GetCombinedStatus(ctx, c.owner, c.repo, headSHA, &github.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list check runs: %w", err)
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

	// Create timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Poll for checks to appear
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

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
				return expectedSHA, checks, nil
			}

			// No checks yet, continue waiting
		}
	}
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
