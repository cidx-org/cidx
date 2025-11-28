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
	Name         string
	Location     Location
	Status       Status
	LastCommit   time.Time
	LastAuthor   string
	CommitHash   string
	AheadBehind  string // e.g., "2 ahead, 3 behind"
	PRNumber     int
	PRStatus     PRStatus
	PRTitle      string
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
	Branches   []Info
	TotalCount int
	Summary    Summary
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
