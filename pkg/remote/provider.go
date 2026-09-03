package remote

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNoPullRequest reports that a branch has no open pull request -- the
// absence itself, not a failure to find out.
//
// The two used to be one string, so every caller that wanted to tolerate "there
// is none" would have tolerated "the token expired" and "the network is down"
// with it, and reported a healthy repository for a broken one (issue #362).
// Providers wrap this; callers ask with errors.Is.
var ErrNoPullRequest = errors.New("no open pull request for this branch")

// ProviderFunc resolves a Provider on demand. Creating one reads the git
// remote, so a command whose local-only steps -- commit analysis, version
// computation, dry-run output -- never reach the remote must take a
// ProviderFunc and call it only at the step that does (issue #227).
type ProviderFunc func() (Provider, error)

// Provider is the interface for CI/CD providers (GitHub, GitLab, etc.)
type Provider interface {
	// GetLatestWorkflow returns the most recent workflow run for a branch
	GetLatestWorkflow(ctx context.Context, branch string) (*Workflow, error)

	// GetLatestRunForBranch returns the most recent workflow run on a branch
	// across all workflows in the repository, regardless of workflow file name.
	// Useful for watching runs on non-PR branches (e.g., direct pushes to main).
	GetLatestRunForBranch(ctx context.Context, branch string) (*Workflow, error)

	// GetLatestRunForTag returns the most recent workflow run triggered by the
	// push of a tag, identified by the tag itself rather than by a workflow
	// file name. A release workflow lives in its own file (release.yml), so the
	// CI candidates cidx knows about never match it (issue #223).
	GetLatestRunForTag(ctx context.Context, tag string) (*Workflow, error)

	// GetWorkflowRun returns a workflow run by its provider-specific ID.
	GetWorkflowRun(ctx context.Context, runID string) (*Workflow, error)

	// ListRuns returns recent runs, most recent first, capped at limit.
	// workflowFile names a single workflow; empty means every workflow of the
	// repository, which is the "what ran on this branch" view a failing check
	// sends you looking for before you know which workflow owns it (issue
	// #342). branch filters on the ref; empty means every ref.
	//
	// The returned runs carry no Jobs: a listing would otherwise cost one API
	// call per row for a column nobody prints.
	ListRuns(ctx context.Context, workflowFile, branch string, limit int) ([]Workflow, error)

	// RerunWorkflow restarts a run. failedOnly restarts only the jobs that
	// failed -- the recovery path when a job dies on an infrastructure flake
	// rather than on the change (issue #342).
	RerunWorkflow(ctx context.Context, runID string, failedOnly bool) error

	// ListRunArtifacts returns the artifacts a single run produced. Scoping to
	// one run is the point: the readers of these files compare images against
	// each other, so mixing two runs' results silently answers a different
	// question than the one asked (issue #285).
	ListRunArtifacts(ctx context.Context, runID string) ([]Artifact, error)

	// DownloadArtifact opens the zip archive of one artifact. The caller closes
	// it.
	DownloadArtifact(ctx context.Context, artifactID int64) (io.ReadCloser, error)

	// WatchWorkflow streams updates for a running workflow
	WatchWorkflow(ctx context.Context, workflowID string) (<-chan WorkflowUpdate, error)

	// TriggerWorkflow starts a run of workflowFile on ref and returns the run
	// it created, so the caller can chain straight into watching it. inputs are
	// the trigger's parameters, passed through unchanged.
	//
	// GitHub's dispatch endpoint answers 204 with no body, so the created run
	// has to be identified afterwards and that identification is best-effort --
	// see the implementation for exactly what it guarantees. GitLab's
	// create-pipeline call returns the pipeline, so there is nothing to guess
	// on that side (issue #266).
	TriggerWorkflow(ctx context.Context, workflowFile, ref string, inputs map[string]string) (*Workflow, error)

	// CreatePullRequest creates a new pull request
	CreatePullRequest(ctx context.Context, title, body, head, base string, draft bool) (number int, url string, err error)

	// MarkPullRequestReady marks a draft PR as ready for review
	MarkPullRequestReady(ctx context.Context, prNumber int) error

	// GetPullRequestByBranch finds a PR for the given head branch
	GetPullRequestByBranch(ctx context.Context, branch string) (number int, url string, err error)

	// MergePullRequest merges a pull request
	MergePullRequest(ctx context.Context, prNumber int, method string) error

	// UpdatePullRequest updates the title and/or body of a pull request.
	// Empty strings leave the corresponding field unchanged.
	UpdatePullRequest(ctx context.Context, prNumber int, title, body string) error

	// GetPullRequestTitle returns the title of a pull request.
	//
	// The interface could write a title before it could read one, which is how
	// a mistyped conventional-commit type survived to the release: the squash
	// subject comes from the title, and the title is the last thing anyone
	// re-reads (issue #361).
	GetPullRequestTitle(ctx context.Context, prNumber int) (string, error)

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

// Workflow represents a CI/CD workflow run.
//
// ID is the identifier every command of the namespace speaks -- watch, rerun,
// artifact download -- and Number is the one the provider's web UI displays.
// They are different numbers and printing only the second one is what made
// `workflow watch` answer 404 on what `workflow list` had just shown (#291),
// so both are carried and both are listed.
type Workflow struct {
	ID         string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped
	Jobs       []Job
	URL        string

	// Descriptive fields, filled by listings. A watch does not need them and
	// leaves them zero.
	Name      string // workflow name, e.g. "CI"
	Number    int    // run number as the provider's UI shows it, e.g. 640
	Branch    string // ref the run was triggered on
	HeadSHA   string
	Title     string // commit subject or display title
	CreatedAt time.Time
}

// Job represents a job within a workflow
type Job struct {
	Name       string
	Status     string // queued, in_progress, completed
	Conclusion string // success, failure, cancelled, skipped
	FailedStep string // first failed step, when the provider exposes it
}

// WorkflowUpdate represents a workflow status update
type WorkflowUpdate struct {
	Workflow *Workflow
	Error    error
}

// PRChecks represents the status of all checks for a PR
type PRChecks struct {
	TotalCount int
	// WorkflowChecks counts the checks produced by a workflow of the
	// repository itself (GitHub Actions runs, GitLab pipeline jobs). Checks
	// posted by other apps -- GitHub's own .github/dependabot.yml config
	// validation, an external service -- are excluded, so "CI has started"
	// can be told apart from "some app posted a check" (issue #257).
	WorkflowChecks int
	Pending        int
	Success        int
	Failure        int
	Queued         int
	InProgress     int
	// RunsInProgress counts the workflow runs on this commit that have not
	// finished. It is the only field that knows about a job which does not
	// exist yet: a provider creates a check when its job becomes eligible, so
	// on a workflow with `needs:` the checks of a later stage are absent, not
	// pending, while an earlier one runs. Counting checks cannot see them --
	// the list is authoritative about what exists, never about what is still
	// coming -- and `Pending == 0` therefore meant "done" and "not started
	// yet" at once (issue #367).
	RunsInProgress int
	Status         string // pending, success, failure
	HeadSHA        string // The commit SHA these checks are for
	UpdatedAt      time.Time
	Checks         []CheckRun
	StatusChecks   []StatusCheck
}

// Complete reports whether there is nothing left to wait for: every check that
// exists has finished, and no run is still able to create another one.
//
// This is the test a watcher stops on and a merge gate passes on. `Pending == 0`
// alone is what let `cpw` announce a green CI having watched one job out of
// five, and `pr merge` merge on it (issue #367).
func (c *PRChecks) Complete() bool {
	return c.Pending == 0 && c.RunsInProgress == 0
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

	// FailedStep names the step a failed check died on. GitHub fills it from
	// the Actions job behind the check run, once per failed check (issue #355).
	// GitLab leaves it empty: a job's `script:` is a flat command list the API
	// reports no per-command result for, so there is no step to name.
	FailedStep string

	// ErrorLog is an error excerpt, when the provider hands one over. It is
	// whatever the app that posted the check summarised it with -- GitHub
	// Actions summarises nothing, so on Actions checks this is empty and
	// FailedStep carries the answer. Neither provider downloads a job log for
	// it; see the clients for what that was weighed against (issue #355).
	ErrorLog string
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
