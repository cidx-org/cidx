package branch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/pkg/remote/github"
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
		info.Checks = &PRChecksInfo{
			Total:   checks.TotalCount,
			Pending: checks.Pending,
			Success: checks.Success,
			Failure: checks.Failure,
			Status:  checks.Status,
		}
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

// Cleanup removes branches that are merged (and optionally stale/orphan)
func (m *Manager) Cleanup(opts CleanupOptions) (*CleanupResult, error) {
	// Get current branch to avoid deleting it
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// List all branches
	listOpts := ListOptions{All: true}
	listResult, err := m.List(listOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	result := &CleanupResult{
		Deleted: []DeletedBranch{},
		Skipped: []SkippedBranch{},
	}

	for _, branch := range listResult.Branches {
		// Determine if this branch should be cleaned up
		shouldCleanup := false
		switch branch.Status {
		case StatusMerged:
			shouldCleanup = true
		case StatusStale:
			shouldCleanup = opts.IncludeStale
		case StatusOrphan:
			shouldCleanup = opts.IncludeOrphan
		}

		if !shouldCleanup {
			continue
		}

		// Skip protected branches
		if branch.IsProtected {
			result.Skipped = append(result.Skipped, SkippedBranch{
				Name:   branch.Name,
				Reason: "protected branch",
			})
			continue
		}

		// Skip current branch
		if branch.Name == currentBranch {
			result.Skipped = append(result.Skipped, SkippedBranch{
				Name:   branch.Name,
				Reason: "current branch",
			})
			continue
		}

		// If dry-run, just record what would be deleted
		if opts.DryRun {
			deleted := DeletedBranch{
				Name:     branch.Name,
				Location: branch.Location,
				Status:   branch.Status,
			}
			if branch.Location == LocationLocal || branch.Location == LocationBoth {
				deleted.LocalDeleted = true
				result.LocalDeleted++
			}
			if branch.Location == LocationRemote || branch.Location == LocationBoth {
				deleted.RemoteDeleted = true
				result.RemoteDeleted++
			}
			result.Deleted = append(result.Deleted, deleted)
			result.TotalDeleted++
			continue
		}

		// Actually delete the branch
		deleted := DeletedBranch{
			Name:     branch.Name,
			Location: branch.Location,
			Status:   branch.Status,
		}

		// Delete local branch
		if branch.Location == LocationLocal || branch.Location == LocationBoth {
			// Force delete if --force flag OR if branch is merged (confirmed by GitHub)
			// This handles local-only branches where git can't verify merge status
			forceDelete := opts.Force || branch.Status == StatusMerged
			err := DeleteLocalBranch(branch.Name, forceDelete)
			if err != nil {
				result.Skipped = append(result.Skipped, SkippedBranch{
					Name:   branch.Name,
					Reason: fmt.Sprintf("failed to delete local: %v", err),
				})
				continue
			}
			deleted.LocalDeleted = true
			result.LocalDeleted++
		}

		// Delete remote branch
		if branch.Location == LocationRemote || branch.Location == LocationBoth {
			err := DeleteRemoteBranch(branch.Name)
			if err != nil {
				// If local was deleted but remote failed, still record partial success
				if deleted.LocalDeleted {
					result.Deleted = append(result.Deleted, deleted)
					result.TotalDeleted++
				}
				result.Skipped = append(result.Skipped, SkippedBranch{
					Name:   branch.Name,
					Reason: fmt.Sprintf("failed to delete remote: %v", err),
				})
				continue
			}
			deleted.RemoteDeleted = true
			result.RemoteDeleted++
		}

		result.Deleted = append(result.Deleted, deleted)
		result.TotalDeleted++
	}

	return result, nil
}
