package branch

import "fmt"

// Cleanup deletes the branches a run is allowed to delete.
//
// The scope is one branch: the one named by --branch, or the current one when
// nothing is named. `--all` restores the repository-wide sweep, which is what
// this command used to do unconditionally -- reaching for it to remove a single
// merged branch deleted seventeen of them (issue #269).
//
// The decision of what goes is entirely in SelectForCleanup; what follows only
// carries it out.
func (m *Manager) Cleanup(opts CleanupOptions) (*CleanupResult, error) {
	// Get current branch to avoid deleting it
	currentBranch, err := GetCurrentBranch()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// List all branches
	listResult, err := m.List(ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list branches: %w", err)
	}

	selected, skipped, err := SelectForCleanup(listResult.Branches, opts, currentBranch)
	if err != nil {
		return nil, err
	}

	result := &CleanupResult{
		Deleted: []DeletedBranch{},
		Skipped: skipped,
		Scope:   scopeOf(opts, currentBranch),
	}

	for _, branch := range selected {
		deleted := DeletedBranch{
			Name:     branch.Name,
			Location: branch.Location,
			Status:   branch.Status,
		}

		// If dry-run, just record what would be deleted
		if opts.DryRun {
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

		// Delete local branch
		if branch.Location == LocationLocal || branch.Location == LocationBoth {
			// Force delete if --force flag OR if branch is merged (confirmed by GitHub)
			// This handles local-only branches where git can't verify merge status
			forceDelete := opts.Force || branch.Status == StatusMerged
			if err := DeleteLocalBranch(branch.Name, forceDelete); err != nil {
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
			if err := DeleteRemoteBranch(branch.Name); err != nil {
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

// SelectForCleanup decides which branches a cleanup run may delete, and records
// why it leaves the others alone. It touches no repository, so the whole
// decision -- scope and safety -- is testable without a single branch at risk.
func SelectForCleanup(branches []Info, opts CleanupOptions, currentBranch string) (selected []Info, skipped []SkippedBranch, err error) {
	if opts.All {
		return sweep(branches, opts, currentBranch)
	}

	target := scopeOf(opts, currentBranch)
	info := findBranch(branches, target)
	if info == nil {
		return nil, nil, fmt.Errorf("branch %q not found", target)
	}

	if reason, ok := deletable(*info, currentBranch, opts.Force); !ok {
		return nil, []SkippedBranch{{Name: info.Name, Reason: reason}}, nil
	}

	return []Info{*info}, nil, nil
}

// scopeOf names the branch a run is limited to, empty when it sweeps.
func scopeOf(opts CleanupOptions, currentBranch string) string {
	if opts.All {
		return ""
	}
	if opts.Branch != "" {
		return opts.Branch
	}
	return currentBranch
}

// deletable reports whether a branch named as the scope may be deleted.
//
// A named branch goes when the repository is visibly finished with it: merged
// into the main branch, or carrying a PR that was merged or closed -- the same
// verdicts `branch list --merged` and `branch list --orphan` show. Anything
// else, an open PR above all, is a branch someone is still working on: it needs
// --force. Protection and the checked-out branch are not overridable -- --force
// on those would only produce a git error.
func deletable(info Info, currentBranch string, force bool) (reason string, ok bool) {
	switch {
	case info.IsProtected:
		return "protected branch -- name another one with --branch, or sweep with --all", false
	case info.Name == currentBranch:
		return "current branch -- git cannot delete the branch you are on; switch away first ('cidx pr merge' cleans up after a merge)", false
	case force:
		return "", true
	case info.Status == StatusMerged || info.Status == StatusOrphan:
		return "", true
	case info.PRStatus == PRStatusOpen:
		return fmt.Sprintf("PR #%d is still open -- pass --force to delete anyway", info.PRNumber), false
	default:
		return "not merged, and no PR says it is finished -- pass --force to delete anyway", false
	}
}

// sweep selects every branch the repository is done with, the behaviour --all
// keeps: merged branches, plus stale and orphan ones when asked for.
func sweep(branches []Info, opts CleanupOptions, currentBranch string) ([]Info, []SkippedBranch, error) {
	var selected []Info
	var skipped []SkippedBranch

	for _, branch := range branches {
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

		if branch.IsProtected {
			skipped = append(skipped, SkippedBranch{Name: branch.Name, Reason: "protected branch"})
			continue
		}

		if branch.Name == currentBranch {
			skipped = append(skipped, SkippedBranch{Name: branch.Name, Reason: "current branch"})
			continue
		}

		selected = append(selected, branch)
	}

	return selected, skipped, nil
}

// findBranch returns the branch with the given name, nil when there is none.
func findBranch(branches []Info, name string) *Info {
	for i := range branches {
		if branches[i].Name == name {
			return &branches[i]
		}
	}
	return nil
}
