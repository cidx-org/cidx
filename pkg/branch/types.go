package branch

import "time"

// Status represents the status of a branch
type Status string

const (
	StatusActive    Status = "active"
	StatusStale     Status = "stale"
	StatusMerged    Status = "merged"
	StatusProtected Status = "protected"
	StatusOrphan    Status = "orphan" // Has PR but PR was closed without merge
)

// Location represents where the branch exists
type Location string

const (
	LocationLocal  Location = "local"
	LocationRemote Location = "remote"
	LocationBoth   Location = "both"
)

// PRStatus represents the status of an associated PR
type PRStatus string

const (
	PRStatusNone   PRStatus = ""
	PRStatusOpen   PRStatus = "open"
	PRStatusMerged PRStatus = "merged"
	PRStatusClosed PRStatus = "closed"
)

// Info contains all information about a branch
type Info struct {
	Name     string
	Location Location
	Status   Status

	// Local branch info
	LocalCommitDate    time.Time
	LocalCommitHash    string
	LocalAuthor        string
	LocalCommitSubject string

	// Remote branch info
	RemoteCommitDate    time.Time
	RemoteCommitHash    string
	RemoteAuthor        string
	RemoteCommitSubject string

	// Computed/derived fields
	LastCommit  time.Time // Most recent of local/remote
	AheadBehind string    // e.g., "2 ahead, 3 behind"

	// PR info
	PRNumber int
	PRStatus PRStatus
	PRTitle  string

	// Branch metadata
	IsProtected  bool
	TracksBranch string // Remote tracking branch
}

// ListOptions configures the branch list operation
type ListOptions struct {
	All        bool   // Include remote branches
	Mine       bool   // Only show branches by current user
	Stale      bool   // Only show stale branches
	Merged     bool   // Only show merged branches
	Orphan     bool   // Only show orphan branches
	Author     string // Filter by author
	StaleDays  int    // Days threshold for stale (default 30)
	MainBranch string // Main branch name (default "main")
	JSON       bool   // Output as JSON
}

// ListResult contains the result of a branch list operation
type ListResult struct {
	Branches       []Info
	TotalCount     int
	Summary        Summary
	HasGitHubToken bool
	CurrentBranch  string
}

// Summary contains branch statistics
type Summary struct {
	Total     int
	Active    int
	Stale     int
	Merged    int
	Protected int
	Orphan    int
	Local     int
	Remote    int
}

// PRInfo contains detailed PR information for a branch
type PRInfo struct {
	Number      int
	Title       string
	Status      PRStatus
	URL         string
	Draft       bool
	Checks      *PRChecksInfo
	Reviews     *PRReviewsInfo
	Mergeable   bool
	BranchName  string
	BaseBranch  string
	AuthorLogin string

	// HeadSHA is the commit the pull request's checks belong to. A watch
	// compares it against local HEAD before reporting anything: they are the
	// same commit only when the push actually happened (#414).
	HeadSHA string
}

// PRChecksInfo contains check/CI status
type PRChecksInfo struct {
	Total   int
	Pending int
	Success int
	Failure int
	// RunsInProgress carries remote.PRChecks.RunsInProgress through, so the
	// branch views stop on the same condition the watchers do (issue #367).
	RunsInProgress int
	// WorkflowChecks counts the checks a workflow of the repository posted, as
	// opposed to another app's (#257). Zero means CI has not started, which is
	// not the same as finished — and a watch that cannot tell them apart calls
	// an empty list a green run (issue #382).
	WorkflowChecks int
	Status         string // "success", "failure", "pending"
	// Failed names the checks behind the Failure count. Without it "4/5
	// passed" is a number with no way to act on it: which check, and whether
	// to fix, rerun or ignore, both lived in the web UI (issue #347).
	Failed []FailedCheck
}

// FailedCheck is a check that did not pass, and what the provider says about
// why. Step and Log are filled only when the provider reports them -- a commit
// status has neither, and a check run carries them only if the app that posted
// it did.
type FailedCheck struct {
	Name string
	Step string // the step that failed, when the provider names one
	Log  string // error excerpt, when the provider carries one
}

// PRReviewsInfo contains review status
type PRReviewsInfo struct {
	Approved         int
	ChangesRequested int
	Pending          int
}

// CleanupOptions configures the cleanup operation
type CleanupOptions struct {
	DryRun        bool   // Show what would be deleted without actually deleting
	Branch        string // Branch to clean up (default: the current branch)
	All           bool   // Sweep every merged branch instead of a single one
	IncludeStale  bool   // Also delete stale branches (with All)
	IncludeOrphan bool   // Also delete orphan branches (with All)
	Force         bool   // Delete a branch the repository is not finished with
}

// CleanupResult contains the result of a cleanup operation
type CleanupResult struct {
	Deleted       []DeletedBranch
	Skipped       []SkippedBranch
	Scope         string // Branch the run was limited to, empty when it swept
	TotalDeleted  int
	LocalDeleted  int
	RemoteDeleted int
}

// DeletedBranch represents a successfully deleted branch
type DeletedBranch struct {
	Name          string
	Location      Location
	Status        Status
	LocalDeleted  bool
	RemoteDeleted bool
}

// SkippedBranch represents a branch that was skipped during cleanup
type SkippedBranch struct {
	Name   string
	Reason string
}
