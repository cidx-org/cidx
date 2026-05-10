package gitlab

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/cidx-org/cidx/pkg/remote"
)

var log = logrus.New()

// Client implements remote.Provider for GitLab
type Client struct {
	client    *gitlab.Client
	projectID string // "owner/repo" format
	owner     string
	repo      string
	baseURL   string
}

// NewClient creates a new GitLab client for gitlab.com
func NewClient(token, owner, repo string) *Client {
	client, err := gitlab.NewClient(token)
	if err != nil {
		log.Errorf("Failed to create GitLab client: %v", err)
		return nil
	}

	return &Client{
		client:    client,
		projectID: fmt.Sprintf("%s/%s", owner, repo),
		owner:     owner,
		repo:      repo,
		baseURL:   "",
	}
}

// NewClientWithBaseURL creates a new GitLab client for self-hosted instances
func NewClientWithBaseURL(token, owner, repo, baseURL string) (*Client, error) {
	client, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{
		client:    client,
		projectID: fmt.Sprintf("%s/%s", owner, repo),
		owner:     owner,
		repo:      repo,
		baseURL:   baseURL,
	}, nil
}

// GetToken resolves GitLab token from environment or glab CLI
// Order: GITLAB_TOKEN → GITLAB_PRIVATE_TOKEN → GL_TOKEN → glab auth token
func GetToken(hostname string) (string, error) {
	// Check environment variables
	envVars := []string{"GITLAB_TOKEN", "GITLAB_PRIVATE_TOKEN", "GL_TOKEN"}
	for _, envVar := range envVars {
		if token := os.Getenv(envVar); token != "" {
			return token, nil
		}
	}

	// Fallback to glab CLI
	args := []string{"auth", "token"}
	if hostname != "" && hostname != "gitlab.com" {
		args = append(args, "--hostname", hostname)
	}

	cmd := exec.Command("glab", args...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab token: set GITLAB_TOKEN or run 'glab auth login'")
	}

	return strings.TrimSpace(string(output)), nil
}

// GetLatestRunForBranch returns the most recent pipeline for the given branch.
// On GitLab, all pipelines on a branch share the same .gitlab-ci.yml, so this
// is equivalent to GetLatestWorkflow.
func (c *Client) GetLatestRunForBranch(ctx context.Context, branch string) (*remote.Workflow, error) {
	return c.GetLatestWorkflow(ctx, branch)
}

// GetWorkflowRun returns a pipeline by its ID.
func (c *Client) GetWorkflowRun(ctx context.Context, runID string) (*remote.Workflow, error) {
	pipelineID := mustAtoi64(runID)
	pipeline, _, err := c.client.Pipelines.GetPipeline(c.projectID, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pipeline %s: %w", runID, err)
	}

	return &remote.Workflow{
		ID:         fmt.Sprintf("%d", pipeline.ID),
		Status:     mapPipelineStatus(pipeline.Status),
		Conclusion: mapPipelineConclusion(pipeline.Status),
		URL:        pipeline.WebURL,
		Jobs:       []remote.Job{},
	}, nil
}

// GetLatestWorkflow returns the latest pipeline for the given branch
func (c *Client) GetLatestWorkflow(ctx context.Context, branch string) (*remote.Workflow, error) {
	pipelines, _, err := c.client.Pipelines.ListProjectPipelines(c.projectID, &gitlab.ListProjectPipelinesOptions{
		Ref:     gitlab.Ptr(branch),
		OrderBy: gitlab.Ptr("id"),
		Sort:    gitlab.Ptr("desc"),
		ListOptions: gitlab.ListOptions{
			PerPage: 1,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pipelines: %w", err)
	}

	if len(pipelines) == 0 {
		return nil, nil
	}

	pipeline := pipelines[0]
	return &remote.Workflow{
		ID:         fmt.Sprintf("%d", pipeline.ID),
		Status:     mapPipelineStatus(pipeline.Status),
		Conclusion: mapPipelineConclusion(pipeline.Status),
		URL:        pipeline.WebURL,
		Jobs:       []remote.Job{},
	}, nil
}

// WatchWorkflow watches a pipeline and sends updates
func (c *Client) WatchWorkflow(ctx context.Context, workflowID string) (<-chan remote.WorkflowUpdate, error) {
	updates := make(chan remote.WorkflowUpdate)

	go func() {
		defer close(updates)

		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		var lastStatus string
		pipelineID := mustAtoi64(workflowID)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pipeline, _, err := c.client.Pipelines.GetPipeline(c.projectID, pipelineID)
				if err != nil {
					updates <- remote.WorkflowUpdate{Error: err}
					return
				}

				status := mapPipelineStatus(pipeline.Status)
				conclusion := mapPipelineConclusion(pipeline.Status)

				if status != lastStatus {
					lastStatus = status
					updates <- remote.WorkflowUpdate{
						Workflow: &remote.Workflow{
							ID:         workflowID,
							Status:     status,
							Conclusion: conclusion,
							URL:        pipeline.WebURL,
							Jobs:       []remote.Job{},
						},
					}
				}

				if status == "completed" {
					return
				}
			}
		}
	}()

	return updates, nil
}

// CreatePullRequest creates a new merge request
func (c *Client) CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (number int, url string, err error) {
	// GitLab uses "Draft: " prefix for draft MRs
	if draft {
		title = "Draft: " + title
	}

	mr, _, err := c.client.MergeRequests.CreateMergeRequest(c.projectID, &gitlab.CreateMergeRequestOptions{
		Title:        gitlab.Ptr(title),
		Description:  gitlab.Ptr(body),
		SourceBranch: gitlab.Ptr(head),
		TargetBranch: gitlab.Ptr(base),
	})
	if err != nil {
		return 0, "", fmt.Errorf("failed to create merge request: %w", err)
	}

	return int(mr.IID), mr.WebURL, nil
}

// MarkPullRequestReady marks a draft MR as ready for review
func (c *Client) MarkPullRequestReady(ctx context.Context, prNumber int) error {
	// Get current MR to check title
	mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, int64(prNumber), nil)
	if err != nil {
		return fmt.Errorf("failed to get merge request: %w", err)
	}

	// Remove "Draft: " prefix if present
	newTitle := strings.TrimPrefix(mr.Title, "Draft: ")
	if newTitle == mr.Title {
		// Already not a draft
		return nil
	}

	_, _, err = c.client.MergeRequests.UpdateMergeRequest(c.projectID, int64(prNumber), &gitlab.UpdateMergeRequestOptions{
		Title: gitlab.Ptr(newTitle),
	})
	if err != nil {
		return fmt.Errorf("failed to mark merge request as ready: %w", err)
	}

	return nil
}

// GetPullRequestByBranch finds an open MR for the given source branch
func (c *Client) GetPullRequestByBranch(ctx context.Context, branch string) (number int, url string, err error) {
	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(c.projectID, &gitlab.ListProjectMergeRequestsOptions{
		SourceBranch: gitlab.Ptr(branch),
		State:        gitlab.Ptr("opened"),
		ListOptions: gitlab.ListOptions{
			PerPage: 1,
		},
	})
	if err != nil {
		return 0, "", fmt.Errorf("failed to list merge requests: %w", err)
	}

	if len(mrs) == 0 {
		return 0, "", fmt.Errorf("no open merge request found for branch '%s'", branch)
	}

	return int(mrs[0].IID), mrs[0].WebURL, nil
}

// MergePullRequest merges a merge request
func (c *Client) MergePullRequest(ctx context.Context, prNumber int, method string) error {
	opts := &gitlab.AcceptMergeRequestOptions{}

	// Map merge methods
	switch method {
	case "squash":
		opts.Squash = gitlab.Ptr(true)
	case "rebase":
		// GitLab handles rebase differently - no specific option needed for manual merge
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(c.projectID, int64(prNumber), opts)
	if err != nil {
		return fmt.Errorf("failed to merge merge request: %w", err)
	}

	return nil
}

// GetPullRequestChecks returns pipeline status for an MR
func (c *Client) GetPullRequestChecks(ctx context.Context, prNumber int) (*remote.PRChecks, error) {
	// Get MR to find associated pipeline
	mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, int64(prNumber), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}

	if mr.HeadPipeline == nil {
		return &remote.PRChecks{
			TotalCount:   0,
			Checks:       []remote.CheckRun{},
			StatusChecks: []remote.StatusCheck{},
			HeadSHA:      mr.SHA,
		}, nil
	}

	// Get pipeline jobs
	jobs, _, err := c.client.Jobs.ListPipelineJobs(c.projectID, mr.HeadPipeline.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pipeline jobs: %w", err)
	}

	checks := make([]remote.CheckRun, 0, len(jobs))
	var pending, success, failure int

	for _, job := range jobs {
		status := mapJobStatus(job.Status)
		conclusion := mapJobConclusion(job.Status)

		checks = append(checks, remote.CheckRun{
			Name:       job.Name,
			Status:     status,
			Conclusion: conclusion,
			URL:        job.WebURL,
		})

		// Count by status
		if status != "completed" {
			pending++
		} else if conclusion == "success" {
			success++
		} else {
			failure++
		}
	}

	// Determine overall status
	overallStatus := "success"
	if failure > 0 {
		overallStatus = "failure"
	} else if pending > 0 {
		overallStatus = "pending"
	}

	return &remote.PRChecks{
		TotalCount:   len(checks),
		Pending:      pending,
		Success:      success,
		Failure:      failure,
		Status:       overallStatus,
		HeadSHA:      mr.SHA,
		Checks:       checks,
		StatusChecks: []remote.StatusCheck{},
	}, nil
}

// WaitForChecksToStart waits for pipeline to be created on the MR
func (c *Client) WaitForChecksToStart(ctx context.Context, prNumber int, timeout time.Duration) (headSHA string, checks *remote.PRChecks, err error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", nil, fmt.Errorf("timeout waiting for pipeline to start")
			}

			mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, int64(prNumber), nil)
			if err != nil {
				return "", nil, fmt.Errorf("failed to get merge request: %w", err)
			}

			if mr.HeadPipeline != nil {
				prChecks, err := c.GetPullRequestChecks(ctx, prNumber)
				if err != nil {
					return "", nil, err
				}
				return mr.SHA, prChecks, nil
			}
		}
	}
}

// WatchPullRequestChecks watches pipeline status for an MR
func (c *Client) WatchPullRequestChecks(ctx context.Context, prNumber int) (<-chan remote.PRChecksUpdate, error) {
	updates := make(chan remote.PRChecksUpdate)

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

				if checks.Status != lastStatus {
					lastStatus = checks.Status
					updates <- remote.PRChecksUpdate{Checks: checks}
				}

				if checks.Pending == 0 {
					return
				}
			}
		}
	}()

	return updates, nil
}

// mapPipelineStatus maps GitLab pipeline status to remote.Workflow status
func mapPipelineStatus(status string) string {
	switch status {
	case "pending", "created", "waiting_for_resource", "preparing":
		return "queued"
	case "running":
		return "in_progress"
	case "success", "failed", "canceled", "skipped", "manual", "scheduled":
		return "completed"
	default:
		return status
	}
}

// mapPipelineConclusion maps GitLab pipeline status to conclusion
func mapPipelineConclusion(status string) string {
	switch status {
	case "success":
		return "success"
	case "failed":
		return "failure"
	case "canceled":
		return "cancelled"
	case "skipped":
		return "skipped"
	default:
		return ""
	}
}

// mapJobStatus maps GitLab job status to check status
func mapJobStatus(status string) string {
	switch status {
	case "pending", "created", "waiting_for_resource", "preparing":
		return "queued"
	case "running":
		return "in_progress"
	case "success", "failed", "canceled", "skipped", "manual":
		return "completed"
	default:
		return status
	}
}

// mapJobConclusion maps GitLab job status to conclusion
func mapJobConclusion(status string) string {
	switch status {
	case "success":
		return "success"
	case "failed":
		return "failure"
	case "canceled":
		return "cancelled"
	case "skipped":
		return "skipped"
	default:
		return ""
	}
}

// mustAtoi64 converts string to int64
func mustAtoi64(s string) int64 {
	var n int64
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}
