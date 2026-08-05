package branch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/remote/github"
)

// Manager handles branch operations
type Manager struct {
	config     Config
	ghClient   *github.Client
	staleDays  int
	mainBranch string
	protected  []string
}

// Config for branch manager
type Config struct {
	StaleDays     int
	NamingPattern string
	AutoCleanup   bool
	Protected     []string
}

// NewManager creates a new branch manager
func NewManager(cfg Config) *Manager {
	staleDays := cfg.StaleDays
	if staleDays == 0 {
		staleDays = 30
	}

	protected := cfg.Protected
	if len(protected) == 0 {
		protected = []string{"main", "master", "develop"}
	}

	// Try to create GitHub client (may fail if no token)
	ghClient, _ := github.NewClientFromEnv()

	return &Manager{
		config:     cfg,
		ghClient:   ghClient,
		staleDays:  staleDays,
		mainBranch: GetDefaultBranch(),
		protected:  protected,
	}
}

// List returns all branches matching the given options
func (m *Manager) List(opts ListOptions) (*ListResult, error) {
	// Fetch latest from remote first
	_ = FetchPrune()

	// Get current branch
	currentBranch, _ := GetCurrentBranch()

	// Get local branches
	localBranches, err := ListLocalBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to list local branches: %w", err)
	}

	// Get remote branches
	remoteBranches, err := ListRemoteBranches()
	if err != nil {
		return nil, fmt.Errorf("failed to list remote branches: %w", err)
	}

	// Build remote branch map for quick lookup
	remoteBranchMap := BuildRemoteBranchMap(remoteBranches)

	// Get current user for "mine" filter
	currentUser := ""
	if opts.Mine {
		currentUser, _ = GetCurrentUser()
	}

	// Build branch map (merge local and remote info)
	branchMap := make(map[string]*Info)

	// Process local branches
	for _, lb := range localBranches {
		info := m.buildBranchInfo(lb, LocationLocal)

		// Check if remote version exists
		if rb, ok := remoteBranchMap[lb.Name]; ok {
			info.Location = LocationBoth
			info.RemoteCommitDate = rb.CommitDate
			info.RemoteCommitHash = rb.CommitHash
			info.RemoteAuthor = rb.Author
			info.RemoteCommitSubject = rb.Subject

			// LastCommit is the most recent of local or remote
			if rb.CommitDate.After(info.LocalCommitDate) {
				info.LastCommit = rb.CommitDate
			}
		}

		branchMap[lb.Name] = info
	}

	// Process remote-only branches
	for _, rb := range remoteBranches {
		if _, ok := branchMap[rb.Name]; !ok && opts.All {
			// Remote-only branch
			info := m.buildBranchInfo(rb, LocationRemote)
			branchMap[rb.Name] = info
		}
	}

	// Fetch PR info for all branches
	m.enrichWithPRInfo(branchMap)

	// Convert to slice and apply filters
	var branches []Info
	for _, info := range branchMap {
		if m.matchesFilters(info, opts, currentUser) {
			branches = append(branches, *info)
		}
	}

	// Sort by last commit date (most recent first)
	sort.Slice(branches, func(i, j int) bool {
		return branches[i].LastCommit.After(branches[j].LastCommit)
	})

	// Build summary
	summary := m.buildSummary(branches)

	return &ListResult{
		Branches:       branches,
		TotalCount:     len(branches),
		Summary:        summary,
		HasGitHubToken: m.ghClient != nil,
		CurrentBranch:  currentBranch,
	}, nil
}

// buildBranchInfo creates a BranchInfo from git branch data
func (m *Manager) buildBranchInfo(gb GitBranch, location Location) *Info {
	info := &Info{
		Name:        gb.Name,
		Location:    location,
		LastCommit:  gb.CommitDate,
		IsProtected: m.isProtected(gb.Name),
	}

	// Set local or remote info based on location
	if location == LocationLocal || location == LocationBoth {
		info.LocalCommitDate = gb.CommitDate
		info.LocalCommitHash = gb.CommitHash
		info.LocalAuthor = gb.Author
		info.LocalCommitSubject = gb.Subject
	}

	if location == LocationRemote {
		info.RemoteCommitDate = gb.CommitDate
		info.RemoteCommitHash = gb.CommitHash
		info.RemoteAuthor = gb.Author
		info.RemoteCommitSubject = gb.Subject
	}

	// Determine status
	info.Status = m.determineStatus(info, gb)

	// Get ahead/behind for local branches
	if location == LocationLocal || location == LocationBoth {
		ahead, behind, err := GetAheadBehind(gb.Name, m.mainBranch)
		if err == nil && (ahead > 0 || behind > 0) {
			parts := []string{}
			if ahead > 0 {
				parts = append(parts, fmt.Sprintf("%d ahead", ahead))
			}
			if behind > 0 {
				parts = append(parts, fmt.Sprintf("%d behind", behind))
			}
			info.AheadBehind = strings.Join(parts, ", ")
		}

		// Get tracking branch
		info.TracksBranch = GetTrackingBranch(gb.Name)
	}

	return info
}

// determineStatus determines the status of a branch
func (m *Manager) determineStatus(info *Info, gb GitBranch) Status {
	if info.IsProtected {
		return StatusProtected
	}

	// Check if merged
	if IsBranchMerged(gb.Name, m.mainBranch) {
		return StatusMerged
	}

	// Check if stale
	staleThreshold := time.Now().AddDate(0, 0, -m.staleDays)
	if gb.CommitDate.Before(staleThreshold) {
		return StatusStale
	}

	return StatusActive
}

// isProtected checks if a branch is protected
func (m *Manager) isProtected(name string) bool {
	for _, p := range m.protected {
		if name == p {
			return true
		}
	}
	return false
}

// enrichWithPRInfo adds PR information to branches
func (m *Manager) enrichWithPRInfo(branchMap map[string]*Info) {
	// Skip if no GitHub client
	if m.ghClient == nil {
		return
	}

	ctx := context.Background()

	// Get all PRs (open and closed)
	prs, err := m.ghClient.ListPullRequests(ctx, "all")
	if err != nil {
		// Silently ignore PR lookup errors
		return
	}

	for _, pr := range prs {
		branchName := pr.GetHead().GetRef()
		if info, ok := branchMap[branchName]; ok {
			info.PRNumber = pr.GetNumber()
			info.PRTitle = pr.GetTitle()

			switch pr.GetState() {
			case "open":
				info.PRStatus = PRStatusOpen
			case "closed":
				if pr.MergedAt != nil {
					info.PRStatus = PRStatusMerged
					// Also mark branch as merged (handles squash/rebase merges)
					if info.Status != StatusProtected {
						info.Status = StatusMerged
					}
				} else {
					info.PRStatus = PRStatusClosed
					// If PR was closed without merge, it's an orphan
					if info.Status != StatusProtected && info.Status != StatusMerged {
						info.Status = StatusOrphan
					}
				}
			}
		}
	}
}

// matchesFilters checks if a branch matches the given filter options
func (m *Manager) matchesFilters(info *Info, opts ListOptions, currentUser string) bool {
	// Get author (prefer local, fallback to remote)
	author := info.LocalAuthor
	if author == "" {
		author = info.RemoteAuthor
	}

	// Author filter
	if opts.Author != "" && !strings.Contains(strings.ToLower(author), strings.ToLower(opts.Author)) {
		return false
	}

	// Mine filter
	if opts.Mine && currentUser != "" && !strings.Contains(strings.ToLower(author), strings.ToLower(currentUser)) {
		return false
	}

	// Status filters
	if opts.Stale && info.Status != StatusStale {
		return false
	}
	if opts.Merged && info.Status != StatusMerged {
		return false
	}
	if opts.Orphan && info.Status != StatusOrphan {
		return false
	}

	return true
}

// buildSummary builds statistics from the branch list
func (m *Manager) buildSummary(branches []Info) Summary {
	summary := Summary{
		Total: len(branches),
	}

	for _, b := range branches {
		switch b.Status {
		case StatusActive:
			summary.Active++
		case StatusStale:
			summary.Stale++
		case StatusMerged:
			summary.Merged++
		case StatusProtected:
			summary.Protected++
		case StatusOrphan:
			summary.Orphan++
		}

		switch b.Location {
		case LocationLocal:
			summary.Local++
		case LocationRemote:
			summary.Remote++
		case LocationBoth:
			summary.Local++
			summary.Remote++
		}
	}

	return summary
}

// GetPRInfo returns detailed PR information for a branch
func (m *Manager) GetPRInfo(branchName string) (*PRInfo, error) {
	if m.ghClient == nil {
		return nil, fmt.Errorf("no GitHub token available")
	}

	ctx := context.Background()

	// Get PR for this branch
	prNumber, prURL, err := m.ghClient.GetPullRequestByBranch(ctx, branchName)
	if err != nil {
		return nil, err
	}

	// Get detailed PR info
	pr, err := m.ghClient.GetPullRequest(ctx, prNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get PR details: %w", err)
	}

	info := &PRInfo{
		Number:      prNumber,
		Title:       pr.GetTitle(),
		URL:         prURL,
		Draft:       pr.GetDraft(),
		Mergeable:   pr.GetMergeable(),
		BranchName:  branchName,
		BaseBranch:  pr.GetBase().GetRef(),
		AuthorLogin: pr.GetUser().GetLogin(),
	}

	// Determine status
	switch pr.GetState() {
	case "open":
		info.Status = PRStatusOpen
	case "closed":
		if pr.MergedAt != nil {
			info.Status = PRStatusMerged
		} else {
			info.Status = PRStatusClosed
		}
	}

	// Get checks status
	checks, err := m.ghClient.GetPullRequestChecks(ctx, prNumber)
	if err == nil {
		info.Checks = ChecksInfo(checks)
	}

	// Get reviews
	reviews, err := m.ghClient.GetPullRequestReviews(ctx, prNumber)
	if err == nil {
		info.Reviews = &PRReviewsInfo{}
		for _, review := range reviews {
			switch review.GetState() {
			case "APPROVED":
				info.Reviews.Approved++
			case "CHANGES_REQUESTED":
				info.Reviews.ChangesRequested++
			case "PENDING":
				info.Reviews.Pending++
			}
		}
	}

	return info, nil
}

// ChecksInfo reduces a provider's checks to what the PR views display: the
// counts, the overall status, and the names behind the failures. It is the one
// place that mapping happens, so `cidx pr status` and `cidx pr watch` cannot
// summarise the same pull request differently.
func ChecksInfo(checks *remote.PRChecks) *PRChecksInfo {
	return &PRChecksInfo{
		Total:          checks.TotalCount,
		Pending:        checks.Pending,
		Success:        checks.Success,
		Failure:        checks.Failure,
		RunsInProgress: checks.RunsInProgress,
		Status:         checks.Status,
		Failed:         failedChecks(checks),
	}
}

// Complete reports whether there is nothing left to wait for. It is
// remote.PRChecks.Complete, restated for the reduced view: every check that
// exists has finished, and no run can still create another one (issue #367).
func (c *PRChecksInfo) Complete() bool {
	return c.Pending == 0 && c.RunsInProgress == 0
}

// failedChecks names the checks the provider counted as failures, so the count
// it also returns can say which one is the missing fifth of "4/5 passed"
// (issue #347).
//
// The predicate mirrors the counting in pkg/remote exactly -- a completed run
// whose conclusion is not success, skipped or neutral, and a commit status that
// is neither pending nor success. Any other rule would name a set that does not
// add up to the number printed next to it, which is worse than printing no
// names at all.
func failedChecks(checks *remote.PRChecks) []FailedCheck {
	var failed []FailedCheck

	for _, run := range checks.Checks {
		if run.Status != "completed" {
			continue
		}
		switch run.Conclusion {
		case "success", "skipped", "neutral":
			continue
		}
		failed = append(failed, FailedCheck{
			Name: run.Name,
			Step: run.FailedStep,
			Log:  run.ErrorLog,
		})
	}

	for _, status := range checks.StatusChecks {
		if status.State == "pending" || status.State == "success" {
			continue
		}
		failed = append(failed, FailedCheck{Name: status.Context})
	}

	return failed
}
