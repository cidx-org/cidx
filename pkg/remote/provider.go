package remote

import "context"

// Provider is the interface for CI/CD providers (GitHub, GitLab, etc.)
type Provider interface {
	// GetLatestWorkflow returns the most recent workflow run for a branch
	GetLatestWorkflow(ctx context.Context, branch string) (*Workflow, error)

	// WatchWorkflow streams updates for a running workflow
	WatchWorkflow(ctx context.Context, workflowID string) (<-chan WorkflowUpdate, error)

	// CreatePullRequest creates a new pull request
	CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (number int, url string, err error)

	// MarkPullRequestReady marks a draft PR as ready for review
	MarkPullRequestReady(ctx context.Context, prNumber int) error

	// GetPullRequestByBranch finds a PR for the given head branch
	GetPullRequestByBranch(ctx context.Context, branch string) (number int, url string, err error)

	// MergePullRequest merges a pull request
	MergePullRequest(ctx context.Context, prNumber int, method string) error

	// GetPullRequestChecks returns the status of all checks/workflows for a PR
	GetPullRequestChecks(ctx context.Context, prNumber int) (*PRChecks, error)

	// WatchPullRequestChecks streams updates for PR checks until all complete
	WatchPullRequestChecks(ctx context.Context, prNumber int) (<-chan PRChecksUpdate, error)
}

// Workflow represents a CI/CD workflow run
type Workflow struct {
	ID         string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped
	Jobs       []Job
	URL        string
}

// Job represents a job within a workflow
type Job struct {
	Name       string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped
}

// WorkflowUpdate represents a workflow status update
type WorkflowUpdate struct {
	Workflow *Workflow
	Error    error
}

// PRChecks represents the status of all checks for a PR
type PRChecks struct {
	TotalCount   int
	Pending      int
	Success      int
	Failure      int
	Status       string // pending, success, failure
	Checks       []CheckRun
	StatusChecks []StatusCheck
}

// CheckRun represents a GitHub Actions check run
type CheckRun struct {
	Name       string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped
	URL        string
}

// StatusCheck represents a commit status check
type StatusCheck struct {
	Context string
	State   string // pending, success, failure, error
	URL     string
}

// PRChecksUpdate represents a PR checks status update
type PRChecksUpdate struct {
	Checks *PRChecks
	Error  error
}
