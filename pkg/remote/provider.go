package remote

import (
	"context"
	"time"
)

// Provider is the interface for CI/CD providers (GitHub, GitLab, etc.)
type Provider interface {
	// GetLatestWorkflow returns the most recent workflow run for a branch
	GetLatestWorkflow(ctx context.Context, branch string) (*Workflow, error)

	// GetLatestRunForBranch returns the most recent workflow run on a branch
	// across all workflows in the repository, regardless of workflow file name.
	// Useful for watching runs on non-PR branches (e.g., direct pushes to main).
	GetLatestRunForBranch(ctx context.Context, branch string) (*Workflow, error)

	// GetWorkflowRun returns a workflow run by its provider-specific ID.
	GetWorkflowRun(ctx context.Context, runID string) (*Workflow, error)

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

	// WaitForChecksToStart waits for CI checks to start for a PR.
	// expectedSHA pins the commit whose checks are awaited (e.g. the commit
	// just pushed); when empty, the PR's current head is resolved from the
	// provider API. Right after a push that API read can lag behind the true
	// head, so callers that know the pushed SHA must pass it (issue #167).
	// Returns the HEAD SHA being checked and the initial checks status.
	WaitForChecksToStart(ctx context.Context, prNumber int, expectedSHA string, timeout time.Duration) (headSHA string, checks *PRChecks, err error)

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
	Queued       int
	InProgress   int
	Status       string // pending, success, failure
	HeadSHA      string // The commit SHA these checks are for
	UpdatedAt    time.Time
	Checks       []CheckRun
	StatusChecks []StatusCheck
}

// CheckRun represents a GitHub Actions check run
type CheckRun struct {
	ID          int64
	Name        string
	Status      string // queued, in_progress, completed
	Conclusion  string // success, failure, cancelled, skipped
	URL         string
	StartedAt   time.Time
	CompletedAt time.Time
	FailedStep  string // Name of the failed step (if any)
	ErrorLog    string // Last lines of error log (if failed)
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

// Artifact represents a workflow artifact
type Artifact struct {
	ID           int64
	Name         string
	SizeInBytes  int64
	CreatedAt    time.Time
	ExpiresAt    time.Time
	Expired      bool
	WorkflowRun  string // Workflow run that created this artifact
	WorkflowName string // Name of the workflow
}

// ArtifactStats represents artifact storage statistics
type ArtifactStats struct {
	TotalCount int
	TotalSize  int64
	Artifacts  []Artifact
}

// PullRequestDetails contains full details about a pull request for TUI display
type PullRequestDetails struct {
	Number       int
	Title        string
	Body         string
	State        string // open, closed, merged
	Draft        bool
	HeadBranch   string
	BaseBranch   string
	HeadSHA      string
	Author       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Additions    int
	Deletions    int
	ChangedFiles int
	Mergeable    bool
	MergeMethod  string // merge, squash, rebase
	URL          string
	Labels       []string
	Reviewers    []ReviewerStatus
	LinkedIssues []LinkedIssue
	Commits      []CommitInfo
}

// ReviewerStatus represents a reviewer and their review status
type ReviewerStatus struct {
	Login  string
	State  string // PENDING, APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED
	Avatar string
}

// LinkedIssue represents an issue linked to a PR
type LinkedIssue struct {
	Number    int
	Title     string
	Body      string
	State     string // open, closed
	URL       string
	Labels    []string
	Assignees []string
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    string
}

// CommitInfo represents a commit in a PR
type CommitInfo struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}
