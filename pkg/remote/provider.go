package remote

import "context"

// Provider is the interface for CI/CD providers (GitHub, GitLab, etc.)
type Provider interface {
	// GetLatestWorkflow returns the most recent workflow run for a branch
	GetLatestWorkflow(ctx context.Context, branch string) (*Workflow, error)

	// WatchWorkflow streams updates for a running workflow
	WatchWorkflow(ctx context.Context, workflowID string) (<-chan WorkflowUpdate, error)
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
