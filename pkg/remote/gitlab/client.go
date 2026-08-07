package gitlab

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/cidx-org/cidx/v2/pkg/remote"
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

// GetLatestRunForTag returns the most recent pipeline triggered by a tag.
// GitLab queries pipelines by ref, which covers branches and tags alike, so the
// tag name is passed through unchanged. Unlike GetLatestWorkflow it reports an
// error rather than a nil pipeline when nothing matches, because callers watch
// the result (issue #223).
func (c *Client) GetLatestRunForTag(ctx context.Context, tag string) (*remote.Workflow, error) {
	workflow, err := c.GetLatestWorkflow(ctx, tag)
	if err != nil {
		return nil, err
	}
	if workflow == nil {
		return nil, fmt.Errorf("no pipeline found for tag %s", tag)
	}
	return workflow, nil
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

// TriggerWorkflow starts a pipeline on ref and returns it.
//
// GitLab runs one pipeline definition per project, so workflowFile selects
// nothing here -- it is reported and ignored rather than silently dropped. The
// inputs become pipeline variables, GitLab's equivalent of the key/value pairs
// a workflow_dispatch takes.
//
// Unlike GitHub's dispatch endpoint, POST /projects/:id/pipeline answers with
// the pipeline it created, so there is no run to identify afterwards and none
// of the raciness documented on the GitHub side applies (issue #266).
func (c *Client) TriggerWorkflow(ctx context.Context, workflowFile, ref string, inputs map[string]string) (*remote.Workflow, error) {
	if ref == "" {
		return nil, fmt.Errorf("a ref is required to trigger a pipeline")
	}
	if workflowFile != "" {
		log.Infof("GitLab runs one pipeline definition per project: %q selects nothing, triggering the project pipeline on %s", workflowFile, ref)
	}

	opts := &gitlab.CreatePipelineOptions{Ref: gitlab.Ptr(ref)}
	if len(inputs) > 0 {
		keys := make([]string, 0, len(inputs))
		for k := range inputs {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		vars := make([]*gitlab.PipelineVariableOptions, 0, len(keys))
		for _, k := range keys {
			vars = append(vars, &gitlab.PipelineVariableOptions{
				Key:   gitlab.Ptr(k),
				Value: gitlab.Ptr(inputs[k]),
			})
		}
		opts.Variables = &vars
	}

	pipeline, _, err := c.client.Pipelines.CreatePipeline(c.projectID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to trigger pipeline on %s (check that the ref is pushed and that .gitlab-ci.yml runs for it): %w", ref, err)
	}

	return &remote.Workflow{
		ID:         fmt.Sprintf("%d", pipeline.ID),
		Status:     mapPipelineStatus(pipeline.Status),
		Conclusion: mapPipelineConclusion(pipeline.Status),
		URL:        pipeline.WebURL,
		Jobs:       []remote.Job{},
	}, nil
}

// ListRuns returns the most recent pipelines, newest first.
//
// workflowFile selects nothing here for the same reason it selects nothing in
// TriggerWorkflow -- GitLab runs one pipeline definition per project -- so it is
// reported and ignored rather than silently dropped. Everything else carries
// over: an empty branch means every ref, and the listing is one call.
func (c *Client) ListRuns(ctx context.Context, workflowFile, branch string, limit int) ([]remote.Workflow, error) {
	if limit <= 0 {
		limit = 10
	}
	if workflowFile != "" {
		log.Infof("GitLab runs one pipeline definition per project: %q selects nothing, listing the project's pipelines", workflowFile)
	}

	opts := &gitlab.ListProjectPipelinesOptions{
		OrderBy:     gitlab.Ptr("id"),
		Sort:        gitlab.Ptr("desc"),
		ListOptions: gitlab.ListOptions{PerPage: int64(limit)},
	}
	if branch != "" {
		opts.Ref = gitlab.Ptr(branch)
	}

	pipelines, _, err := c.client.Pipelines.ListProjectPipelines(c.projectID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pipelines: %w", err)
	}

	listed := make([]remote.Workflow, 0, len(pipelines))
	for _, p := range pipelines {
		run := remote.Workflow{
			ID:         fmt.Sprintf("%d", p.ID),
			Status:     mapPipelineStatus(p.Status),
			Conclusion: mapPipelineConclusion(p.Status),
			URL:        p.WebURL,
			Name:       p.Name,
			Number:     int(p.IID),
			Branch:     p.Ref,
			HeadSHA:    p.SHA,
			Title:      p.Source,
		}
		if p.CreatedAt != nil {
			run.CreatedAt = *p.CreatedAt
		}
		listed = append(listed, run)
	}
	return listed, nil
}

// RerunWorkflow retries the failed jobs of a pipeline.
//
// GitLab's retry endpoint restarts the jobs that failed or were cancelled, which
// is exactly the recovery `--failed` asks for. There is no counterpart for
// restarting a pipeline that succeeded: the platform's answer to that is a new
// pipeline on the same ref, which cidx already has a command for, so this says
// so instead of pretending (issue #342).
func (c *Client) RerunWorkflow(ctx context.Context, runID string, failedOnly bool) error {
	if !failedOnly {
		return fmt.Errorf("GitLab retries a pipeline's failed jobs, it cannot restart one that has none: use --failed, or start a new pipeline with `cidx repo workflow run --ref <ref>`")
	}

	if _, _, err := c.client.Pipelines.RetryPipelineBuild(c.projectID, mustAtoi64(runID)); err != nil {
		return fmt.Errorf("failed to retry pipeline %s: %w", runID, err)
	}
	return nil
}

// ListRunArtifacts returns the jobs of a pipeline that produced an artifact.
//
// GitLab attaches artifacts to jobs, not to pipelines, so a job that uploaded
// something is what an artifact is here -- and the identifier the download takes
// is therefore the job's. The name is the job's name, which is what a GitLab
// user sees next to the archive in the UI.
func (c *Client) ListRunArtifacts(ctx context.Context, runID string) ([]remote.Artifact, error) {
	jobs, _, err := c.client.Jobs.ListPipelineJobs(c.projectID, mustAtoi64(runID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list the jobs of pipeline %s: %w", runID, err)
	}

	var artifacts []remote.Artifact
	for _, job := range jobs {
		if job.ArtifactsFile.Filename == "" {
			continue
		}

		artifact := remote.Artifact{
			ID:           job.ID,
			Name:         job.Name,
			SizeInBytes:  job.ArtifactsFile.Size,
			WorkflowRun:  runID,
			WorkflowName: job.Stage,
		}
		if job.CreatedAt != nil {
			artifact.CreatedAt = *job.CreatedAt
		}
		if job.ArtifactsExpireAt != nil {
			artifact.ExpiresAt = *job.ArtifactsExpireAt
			artifact.Expired = job.ArtifactsExpireAt.Before(time.Now())
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

// DownloadArtifact opens the zip archive a job uploaded. artifactID is the job's
// ID, which is what ListRunArtifacts reports.
func (c *Client) DownloadArtifact(ctx context.Context, artifactID int64) (io.ReadCloser, error) {
	archive, _, err := c.client.Jobs.GetJobArtifacts(c.projectID, artifactID)
	if err != nil {
		return nil, fmt.Errorf("failed to download the artifacts of job %d: %w", artifactID, err)
	}
	return io.NopCloser(archive), nil
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
		return 0, "", fmt.Errorf("branch '%s': %w", branch, remote.ErrNoPullRequest)
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

// GetPullRequestTitle returns the title of a merge request.
//
// GitLab prefixes a draft's title with "Draft: ", which is the platform's
// marker rather than part of what the author wrote — UpdatePullRequest below
// re-applies it for the same reason. It is stripped here so a caller reading
// the title reads the title.
func (c *Client) GetPullRequestTitle(ctx context.Context, prNumber int) (string, error) {
	mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, int64(prNumber), nil)
	if err != nil {
		return "", fmt.Errorf("failed to get merge request: %w", err)
	}

	return strings.TrimPrefix(mr.Title, "Draft: "), nil
}

// UpdatePullRequest updates the title and/or description of a merge request.
// Empty strings leave the corresponding field unchanged. The "Draft: " title
// prefix is preserved so retitling a draft MR does not mark it ready.
func (c *Client) UpdatePullRequest(ctx context.Context, prNumber int, title, body string) error {
	opts := &gitlab.UpdateMergeRequestOptions{}

	if title != "" {
		mr, _, err := c.client.MergeRequests.GetMergeRequest(c.projectID, int64(prNumber), nil)
		if err != nil {
			return fmt.Errorf("failed to get merge request: %w", err)
		}
		if strings.HasPrefix(mr.Title, "Draft: ") && !strings.HasPrefix(title, "Draft: ") {
			title = "Draft: " + title
		}
		opts.Title = gitlab.Ptr(title)
	}

	if body != "" {
		opts.Description = gitlab.Ptr(body)
	}

	_, _, err := c.client.MergeRequests.UpdateMergeRequest(c.projectID, int64(prNumber), opts)
	if err != nil {
		return fmt.Errorf("failed to update merge request: %w", err)
	}

	return nil
}

// GetPullRequestChecks returns pipeline status for an MR.
//
// The counterpart of issue #240 cannot arise here: the jobs come from the MR's
// own head pipeline, not from everything attached to the head commit, so a
// pipeline someone triggers by hand on the branch is a separate pipeline that
// is never folded in. GitHub has no such notion -- every check run of the SHA
// lands on the PR -- which is why only that side needed a filter.
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

		// FailedStep and ErrorLog are left empty here on purpose, not by
		// omission (issue #355).
		//
		// FailedStep has no counterpart to fill it with. A GitLab job's
		// `script:` is a flat list of shell commands, and the jobs API reports
		// no per-command result -- there is no step to name. What the job does
		// carry is `failure_reason`, a category (`script_failure`,
		// `runner_system_failure`, `job_execution_timeout`); rendering it where
		// GitHub renders a step name would print `failed step: script_failure`,
		// which names nothing and reads as though it did.
		//
		// ErrorLog would mean downloading the job trace, one request and a full
		// log per failed job. That is the same cost GitHub's job log was
		// weighed at and rejected for on that side, for the same tail: the last
		// lines of a trace are the runner's cleanup, not the error.
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

	// A pipeline that has not reached a terminal state can still add jobs --
	// GitLab creates the jobs of a later stage as the pipeline advances, so
	// counting the jobs listed today answers only for today (issue #367). The
	// pipeline was already fetched above, so its own status costs nothing.
	runsInProgress := 0
	if !terminalPipelineStatuses[mr.HeadPipeline.Status] {
		runsInProgress = 1
	}

	// Determine overall status
	overallStatus := "success"
	if failure > 0 {
		overallStatus = "failure"
	} else if pending > 0 || runsInProgress > 0 {
		overallStatus = "pending"
	}

	return &remote.PRChecks{
		RunsInProgress: runsInProgress,
		TotalCount:     len(checks),
		// Every check here is a job of the project's own pipeline, so they all
		// count as workflow checks (issue #257).
		WorkflowChecks: len(checks),
		Pending:        pending,
		Success:        success,
		Failure:        failure,
		Status:         overallStatus,
		HeadSHA:        mr.SHA,
		Checks:         checks,
		StatusChecks:   []remote.StatusCheck{},
	}, nil
}

// WaitForChecksToStart waits for pipeline to be created on the MR.
// When expectedSHA is set, it also waits for the MR head to reach that
// commit, so a pipeline for a previous commit is never returned (issue #167).
func (c *Client) WaitForChecksToStart(ctx context.Context, prNumber int, expectedSHA string, timeout time.Duration) (headSHA string, checks *remote.PRChecks, err error) {
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

			if expectedSHA != "" && mr.SHA != expectedSHA {
				// MR head has not caught up with the pushed commit yet
				continue
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

				// See #367: a pipeline that has not reached a terminal state can
				// still add jobs, so the jobs listed today are not the whole run.
				if checks.Complete() {
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
// terminalPipelineStatuses are the pipeline states that will not advance on
// their own. `success`, `failed`, `canceled` and `skipped` are finished.
//
// `manual` and `scheduled` are not finished, and are counted here anyway: a
// pipeline blocked on a manual job waits for a person, and a scheduled one for
// a clock. Treating either as still-running would make a watcher block until
// its timeout on a pipeline that is behaving exactly as configured -- and the
// point of #367 is to stop reporting one state as another, not to trade a false
// green for a hang.
var terminalPipelineStatuses = map[string]bool{
	"success":   true,
	"failed":    true,
	"canceled":  true,
	"skipped":   true,
	"manual":    true,
	"scheduled": true,
}

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
