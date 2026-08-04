package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/google/go-github/v76/github"
)

// listRunsFunc lists the workflow runs of a single workflow file, with the
// branch filter already bound. It exists as a seam so latestRunFromCandidates
// can be unit-tested without a real GitHub client.
type listRunsFunc func(ctx context.Context, workflowFile string) (*github.WorkflowRuns, *github.Response, error)

// GetLatestWorkflow returns the most recent workflow run for a branch.
// The workflow filename is not hardcoded: the candidate names from
// remote.CandidateWorkflowFiles are probed in preference order, so both
// generated projects (cidx.yml) and repos with a conventional ci.yml work
// without configuration (issue #170).
func (c *Client) GetLatestWorkflow(ctx context.Context, branch string) (*remote.Workflow, error) {
	run, err := latestRunFromCandidates(ctx, branch, func(ctx context.Context, workflowFile string) (*github.WorkflowRuns, *github.Response, error) {
		return c.client.Actions.ListWorkflowRunsByFileName(
			ctx,
			c.owner,
			c.repo,
			workflowFile,
			&github.ListWorkflowRunsOptions{
				Branch:      branch,
				ListOptions: github.ListOptions{PerPage: 1},
			},
		)
	})
	if err != nil {
		return nil, err
	}

	return c.convertWorkflow(ctx, run)
}

// latestRunFromCandidates probes remote.CandidateWorkflowFiles in preference
// order and returns the latest run of the first workflow that has one for the
// branch. A 404 (workflow file absent from the repo) or an empty run list
// falls through to the next candidate; any other API error aborts.
func latestRunFromCandidates(ctx context.Context, branch string, list listRunsFunc) (*github.WorkflowRun, error) {
	for _, file := range remote.CandidateWorkflowFiles {
		runs, resp, err := list(ctx, file)
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			return nil, fmt.Errorf("failed to list workflow runs for %s: %w", file, err)
		}
		if runs == nil || len(runs.WorkflowRuns) == 0 {
			continue
		}
		return runs.WorkflowRuns[0], nil
	}

	return nil, fmt.Errorf("no workflow runs found for branch %s (tried %s)", branch, strings.Join(remote.CandidateWorkflowFiles, ", "))
}

// listRepoRunsFunc lists repository-wide workflow runs under the given filters.
// It exists as a seam so latestRunForRef can be unit-tested without a real
// GitHub client.
type listRepoRunsFunc func(ctx context.Context, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error)

// GetLatestRunForBranch returns the most recent workflow run on a branch across
// all workflows in the repository. Unlike GetLatestWorkflow, it is not scoped to
// a specific workflow filename, so it works for repositories that don't use
// cidx-generated workflows. Used by `cidx workflow watch` to support watching
// runs on non-PR branches (issue #125).
func (c *Client) GetLatestRunForBranch(ctx context.Context, branch string) (*remote.Workflow, error) {
	run, err := latestRunForRef(ctx, branch, "branch", c.listRepositoryRuns)
	if err != nil {
		return nil, err
	}

	return c.convertWorkflow(ctx, run)
}

// GetLatestRunForTag returns the most recent workflow run triggered by the push
// of a tag. GitHub records the ref that started a run in head_branch -- for a
// tag push that is the tag name -- and the repository-wide runs listing filters
// on it. Resolving by ref instead of by workflow file name means the release
// workflow is found whatever it is called, and the runs that a branch push of
// the very same commit produced (CI, nightly) are left out because their ref is
// the branch, not the tag (issue #223).
func (c *Client) GetLatestRunForTag(ctx context.Context, tag string) (*remote.Workflow, error) {
	run, err := latestRunForRef(ctx, tag, "tag", c.listRepositoryRuns)
	if err != nil {
		return nil, err
	}

	return c.convertWorkflow(ctx, run)
}

// listRepositoryRuns is the real implementation behind listRepoRunsFunc.
func (c *Client) listRepositoryRuns(ctx context.Context, opts *github.ListWorkflowRunsOptions) (*github.WorkflowRuns, *github.Response, error) {
	return c.client.Actions.ListRepositoryWorkflowRuns(ctx, c.owner, c.repo, opts)
}

// latestRunForRef returns the most recent run whose ref is the given branch or
// tag. refKind only shapes the error message, so the user is told what was
// actually searched.
func latestRunForRef(ctx context.Context, ref, refKind string, list listRepoRunsFunc) (*github.WorkflowRun, error) {
	runs, _, err := list(ctx, &github.ListWorkflowRunsOptions{
		Branch:      ref,
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}

	if runs == nil || len(runs.WorkflowRuns) == 0 {
		return nil, fmt.Errorf("no workflow runs found for %s %s", refKind, ref)
	}

	return runs.WorkflowRuns[0], nil
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

// ListRuns returns the most recent runs, newest first.
//
// With no workflow file it reads the repository-wide listing, which is the
// answer to "what ran on this branch": a check that has just failed names a job,
// not the workflow file that carries it, and having to guess the file before
// being allowed to look is what sent this question to `gh` (issue #342).
//
// A workflow file the repository does not have is not an error -- the caller
// gets an empty listing and says so, the same way the gh-backed version did.
func (c *Client) ListRuns(ctx context.Context, workflowFile, branch string, limit int) ([]remote.Workflow, error) {
	if limit <= 0 {
		limit = 10
	}

	opts := &github.ListWorkflowRunsOptions{
		Branch:      branch,
		ListOptions: github.ListOptions{PerPage: limit},
	}

	var (
		runs *github.WorkflowRuns
		resp *github.Response
		err  error
	)
	if workflowFile == "" {
		runs, resp, err = c.client.Actions.ListRepositoryWorkflowRuns(ctx, c.owner, c.repo, opts)
	} else {
		runs, resp, err = c.client.Actions.ListWorkflowRunsByFileName(ctx, c.owner, c.repo, remote.NormalizeWorkflowFile(workflowFile), opts)
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list workflow runs: %w", err)
	}
	if runs == nil {
		return nil, nil
	}

	listed := make([]remote.Workflow, 0, len(runs.WorkflowRuns))
	for _, run := range runs.WorkflowRuns {
		listed = append(listed, summarizeRun(run))
	}
	return listed, nil
}

// summarizeRun describes a run without fetching its jobs, which is what makes a
// listing one API call instead of one per row.
func summarizeRun(run *github.WorkflowRun) remote.Workflow {
	return remote.Workflow{
		ID:         strconv.FormatInt(run.GetID(), 10),
		Status:     run.GetStatus(),
		Conclusion: run.GetConclusion(),
		URL:        run.GetHTMLURL(),
		Name:       run.GetName(),
		Number:     run.GetRunNumber(),
		Branch:     run.GetHeadBranch(),
		HeadSHA:    run.GetHeadSHA(),
		Title:      run.GetDisplayTitle(),
		CreatedAt:  run.GetCreatedAt().Time,
	}
}

// RerunWorkflow restarts a run, or only the jobs of it that failed.
//
// Both endpoints answer 201 with no body and start a *new attempt of the same
// run*, so the run ID stays valid and the caller can watch it straight away --
// which is the whole point of having this next to `workflow watch` (issue #342).
func (c *Client) RerunWorkflow(ctx context.Context, runID string, failedOnly bool) error {
	id, err := strconv.ParseInt(runID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid workflow run ID %q: %w", runID, err)
	}

	if failedOnly {
		_, err = c.client.Actions.RerunFailedJobsByID(ctx, c.owner, c.repo, id)
	} else {
		_, err = c.client.Actions.RerunWorkflowByID(ctx, c.owner, c.repo, id)
	}
	if err != nil {
		return fmt.Errorf("failed to rerun run %s: %w", runID, err)
	}
	return nil
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
