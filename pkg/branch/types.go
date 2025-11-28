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
}

// PRChecksInfo contains check/CI status
type PRChecksInfo struct {
	Total   int
	Pending int
	Success int
	Failure int
	Status  string // "success", "failure", "pending"
}

// PRReviewsInfo contains review status
type PRReviewsInfo struct {
	Approved         int
	ChangesRequested int
	Pending          int
}

// CleanupOptions configures the cleanup operation
type CleanupOptions struct {
	DryRun       bool   // Show what would be deleted without actually deleting
	IncludeStale bool   // Also delete stale branches
	IncludeOrphan bool  // Also delete orphan branches
	Force        bool   // Force delete even if not fully merged
}

// CleanupResult contains the result of a cleanup operation
type CleanupResult struct {
	Deleted       []DeletedBranch
	Skipped       []SkippedBranch
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
