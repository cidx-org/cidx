package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/arcker/cidx/pkg/remote"
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
