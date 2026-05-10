package github

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/google/go-github/v76/github"
)

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

// GetLatestRunForBranch returns the most recent workflow run on a branch across
// all workflows in the repository. Unlike GetLatestWorkflow, it is not scoped to
// a specific workflow filename, so it works for repositories that don't use
// cidx-generated workflows. Used by `cidx workflow watch` to support watching
// runs on non-PR branches (issue #125).
func (c *Client) GetLatestRunForBranch(ctx context.Context, branch string) (*remote.Workflow, error) {
	runs, _, err := c.client.Actions.ListRepositoryWorkflowRuns(
		ctx,
		c.owner,
		c.repo,
		&github.ListWorkflowRunsOptions{
			Branch:      branch,
			ListOptions: github.ListOptions{PerPage: 1},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	if len(runs.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no workflow runs found for branch %s", branch)
	}

	return c.convertWorkflow(ctx, runs.WorkflowRuns[0])
}

// GetWorkflowRun returns a workflow run by its ID. Used by `cidx workflow watch
// --run <id>` to watch a specific run.
func (c *Client) GetWorkflowRun(ctx context.Context, runID string) (*remote.Workflow, error) {
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow run ID %q: %w", runID, err)
	}

	run, _, err := c.client.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, id)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch workflow run %s: %w", runID, err)
	}

	return c.convertWorkflow(ctx, run)
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
