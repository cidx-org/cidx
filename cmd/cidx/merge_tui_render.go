package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/cidx-org/cidx/pkg/remote"
)

func (m mergeModel) renderTreeView(width int) string {
	var b strings.Builder

	// Tree characters
	const (
		treeRoot   = "📦"
		treeBranch = "├──"
		treeLast   = "└──"
		treeVert   = "│  "
		treeSpace  = "   "
	)

	// If we have linked issues, show Issue → PR tree
	if len(m.prDetails.LinkedIssues) > 0 {
		// For each linked issue, create a tree
		for i, issue := range m.prDetails.LinkedIssues {
			// Issue root
			issueIcon := "🟢"
			if issue.State == "closed" {
				issueIcon = "🟣"
			}
			fmt.Fprintf(&b, "%s Issue #%d\n", issueIcon, issue.Number)

			// Issue details
			fmt.Fprintf(&b, "%s %s\n", treeVert, mergeLabelStyle.Render(issue.Title))

			// Issue body (truncated)
			if issue.Body != "" {
				bodyPreview := truncateStr(strings.ReplaceAll(issue.Body, "\n", " "), 60)
				fmt.Fprintf(&b, "%s %s\n", treeVert, mergeDimStyle.Render(bodyPreview))
			}

			// Issue metadata
			if len(issue.Labels) > 0 {
				labelsStr := strings.Join(issue.Labels, ", ")
				fmt.Fprintf(&b, "%s 🏷 %s\n", treeVert, mergeDimStyle.Render(labelsStr))
			}
			if issue.Author != "" {
				fmt.Fprintf(&b, "%s 👤 @%s\n", treeVert, mergeDimStyle.Render(issue.Author))
			}

			// PR as child of issue
			fmt.Fprintf(&b, "%s\n", treeVert)
			fmt.Fprintf(&b, "%s 🔀 PR #%d\n", treeLast, m.prDetails.Number)

			// PR details (indented under the issue)
			prIndent := treeSpace
			fmt.Fprintf(&b, "%s%s %s\n", prIndent, treeBranch, mergeLabelStyle.Render(m.prDetails.Title))

			// Branch info
			branchLine := fmt.Sprintf("%s → %s", m.prDetails.HeadBranch, m.prDetails.BaseBranch)
			fmt.Fprintf(&b, "%s%s 🌿 %s\n", prIndent, treeVert, mergeDimStyle.Render(branchLine))

			// Stats
			statsLine := fmt.Sprintf("+%d -%d • %d files", m.prDetails.Additions, m.prDetails.Deletions, m.prDetails.ChangedFiles)
			fmt.Fprintf(&b, "%s%s 📊 %s\n", prIndent, treeVert, mergeDimStyle.Render(statsLine))

			// Author
			fmt.Fprintf(&b, "%s%s 👤 @%s\n", prIndent, treeVert, mergeDimStyle.Render(m.prDetails.Author))

			// Reviews
			if len(m.prDetails.Reviewers) > 0 {
				fmt.Fprintf(&b, "%s%s 👥 Reviews:\n", prIndent, treeVert)
				for j, reviewer := range m.prDetails.Reviewers {
					prefix := treeVert + treeVert
					if j == len(m.prDetails.Reviewers)-1 {
						prefix = treeVert + treeSpace
					}
					reviewIcon := "○"
					var reviewStyle lipgloss.Style
					switch reviewer.State {
					case "APPROVED":
						reviewIcon = "✓"
						reviewStyle = mergeReviewApprovedStyle
					case "CHANGES_REQUESTED":
						reviewIcon = "✗"
						reviewStyle = mergeReviewChangesStyle
					default:
						reviewStyle = mergeReviewPendingStyle
					}
					reviewLine := fmt.Sprintf("%s @%s (%s)", reviewIcon, reviewer.Login, reviewer.State)
					fmt.Fprintf(&b, "%s%s    %s\n", prIndent, prefix, reviewStyle.Render(reviewLine))
				}
			}

			// Commits
			if len(m.prDetails.Commits) > 0 {
				fmt.Fprintf(&b, "%s%s 📝 Commits (%d):\n", prIndent, treeVert, len(m.prDetails.Commits))
				// Show last 3 commits
				start := 0
				if len(m.prDetails.Commits) > 3 {
					start = len(m.prDetails.Commits) - 3
					fmt.Fprintf(&b, "%s%s    %s\n", prIndent, treeVert, mergeDimStyle.Render(fmt.Sprintf("... %d earlier commits", start)))
				}
				for j := start; j < len(m.prDetails.Commits); j++ {
					c := m.prDetails.Commits[j]
					isLast := j == len(m.prDetails.Commits)-1
					prefix := treeVert
					if isLast {
						prefix = treeSpace
					}
					commitLine := fmt.Sprintf("%s %s", c.SHA[:7], truncateStr(c.Message, 40))
					fmt.Fprintf(&b, "%s%s    %s\n", prIndent, prefix, mergeDimStyle.Render(commitLine))
				}
			}

			// Mergeable status
			if m.prDetails.Mergeable {
				fmt.Fprintf(&b, "%s%s %s\n", prIndent, treeLast, mergeSuccessStyle.Render("✓ Ready to merge"))
			} else {
				fmt.Fprintf(&b, "%s%s %s\n", prIndent, treeLast, mergeWarningStyle.Render("⚠ Not mergeable"))
			}

			// Add separator between multiple issues
			if i < len(m.prDetails.LinkedIssues)-1 {
				b.WriteString("\n")
			}
		}
	} else {
		// No linked issues - show PR only
		fmt.Fprintf(&b, "🔀 PR #%d\n", m.prDetails.Number)
		fmt.Fprintf(&b, "%s %s\n", treeBranch, mergeLabelStyle.Render(m.prDetails.Title))

		// Branch info
		branchLine := fmt.Sprintf("%s → %s", m.prDetails.HeadBranch, m.prDetails.BaseBranch)
		fmt.Fprintf(&b, "%s 🌿 %s\n", treeVert, mergeDimStyle.Render(branchLine))

		// Stats
		statsLine := fmt.Sprintf("+%d -%d • %d files", m.prDetails.Additions, m.prDetails.Deletions, m.prDetails.ChangedFiles)
		fmt.Fprintf(&b, "%s 📊 %s\n", treeVert, mergeDimStyle.Render(statsLine))

		// Author
		fmt.Fprintf(&b, "%s 👤 @%s\n", treeVert, mergeDimStyle.Render(m.prDetails.Author))

		// Labels
		if len(m.prDetails.Labels) > 0 {
			labelsLine := strings.Join(m.prDetails.Labels, ", ")
			fmt.Fprintf(&b, "%s 🏷 %s\n", treeVert, mergeDimStyle.Render(labelsLine))
		}

		// Reviews
		if len(m.prDetails.Reviewers) > 0 {
			fmt.Fprintf(&b, "%s 👥 Reviews:\n", treeVert)
			for i, reviewer := range m.prDetails.Reviewers {
				prefix := treeVert
				if i == len(m.prDetails.Reviewers)-1 {
					prefix = treeSpace
				}
				reviewIcon := "○"
				var reviewStyle lipgloss.Style
				switch reviewer.State {
				case "APPROVED":
					reviewIcon = "✓"
					reviewStyle = mergeReviewApprovedStyle
				case "CHANGES_REQUESTED":
					reviewIcon = "✗"
					reviewStyle = mergeReviewChangesStyle
				default:
					reviewStyle = mergeReviewPendingStyle
				}
				reviewLine := fmt.Sprintf("%s @%s (%s)", reviewIcon, reviewer.Login, reviewer.State)
				fmt.Fprintf(&b, "%s    %s\n", prefix, reviewStyle.Render(reviewLine))
			}
		}

		// Commits
		if len(m.prDetails.Commits) > 0 {
			fmt.Fprintf(&b, "%s 📝 Commits (%d):\n", treeVert, len(m.prDetails.Commits))
			start := 0
			if len(m.prDetails.Commits) > 3 {
				start = len(m.prDetails.Commits) - 3
				fmt.Fprintf(&b, "%s    %s\n", treeVert, mergeDimStyle.Render(fmt.Sprintf("... %d earlier commits", start)))
			}
			for j := start; j < len(m.prDetails.Commits); j++ {
				c := m.prDetails.Commits[j]
				commitLine := fmt.Sprintf("%s %s", c.SHA[:7], truncateStr(c.Message, 40))
				fmt.Fprintf(&b, "%s    %s\n", treeVert, mergeDimStyle.Render(commitLine))
			}
		}

		// Mergeable status
		if m.prDetails.Mergeable {
			fmt.Fprintf(&b, "%s %s\n", treeLast, mergeSuccessStyle.Render("✓ Ready to merge"))
		} else {
			fmt.Fprintf(&b, "%s %s\n", treeLast, mergeWarningStyle.Render("⚠ Not mergeable"))
		}
	}

	return mergeBoxStyle.Width(width).Render(b.String())
}

func (m mergeModel) renderChecks(width int) string {
	var b strings.Builder

	// Use mainPipelineCheck when in merged mode, otherwise use checks
	checks := m.checks
	if m.merged && m.mainPipelineCheck != nil {
		checks = m.mainPipelineCheck
	}

	// Header with auto-refresh status and merged mode indicator
	headerLabel := "🔍 CI Status"
	if m.merged {
		headerLabel = "🔍 Main Pipeline"
	}
	autoRefreshStatus := ""
	if m.merged && !m.pipelineComplete {
		autoRefreshStatus = " [watching]"
	} else if m.autoRefresh {
		autoRefreshStatus = " [auto]"
	}
	if m.refreshing {
		autoRefreshStatus = " [refreshing...]"
	}
	b.WriteString(mergeLabelStyle.Render(fmt.Sprintf("%s%s", headerLabel, autoRefreshStatus)))
	b.WriteString("\n")

	if checks == nil || checks.TotalCount == 0 {
		if m.merged {
			b.WriteString(mergeDimStyle.Render("  ⏳ Waiting for main pipeline to start..."))
		} else {
			b.WriteString(mergeDimStyle.Render("  ⏳ Waiting for CI to start..."))
		}
		b.WriteString("\n")
		if !m.merged {
			b.WriteString(mergeDimStyle.Render("  Press 'r' to refresh • 'a' to toggle auto-refresh"))
		}
		return mergeBoxStyle.Width(width).Render(b.String())
	}

	// Commit SHA (verify this is the right CI run)
	shortSHA := checks.HeadSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	if m.merged {
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  📌 Branch: main (workflow %s)", shortSHA)))
	} else {
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  📌 Commit: %s", shortSHA)))
	}
	b.WriteString("\n")

	// Last refresh time
	if !checks.UpdatedAt.IsZero() {
		refreshTime := checks.UpdatedAt.Format("15:04:05")
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  🔄 Last refresh: %s", refreshTime)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Progress bar
	b.WriteString(m.renderProgressBarWithChecks(checks, width-6))
	b.WriteString("\n\n")

	// Summary with detailed breakdown
	var statusIcon string
	var statusStyle lipgloss.Style
	switch checks.Status {
	case "success":
		statusIcon = "✓"
		statusStyle = mergeCheckSuccessStyle
	case "failure":
		statusIcon = "✗"
		statusStyle = mergeCheckFailureStyle
	default:
		statusIcon = "○"
		statusStyle = mergeCheckPendingStyle
	}

	// Status summary line
	summary := fmt.Sprintf("  %s %d/%d checks passed", statusIcon, checks.Success, checks.TotalCount)
	b.WriteString(statusStyle.Render(summary))
	b.WriteString("\n")

	// Detailed breakdown if there are pending checks
	if checks.Pending > 0 || checks.Queued > 0 || checks.InProgress > 0 {
		breakdown := fmt.Sprintf("     (%d queued, %d running, %d completed)",
			checks.Queued, checks.InProgress, checks.Success+checks.Failure)
		b.WriteString(mergeDimStyle.Render(breakdown))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// All checks (show all, not limited)
	hasFailures := false
	for _, check := range checks.Checks {
		var icon string
		var style lipgloss.Style
		var statusText string

		switch check.Status {
		case "completed":
			switch check.Conclusion {
			case "success":
				icon = "✓"
				style = mergeCheckSuccessStyle
				if !check.CompletedAt.IsZero() {
					duration := check.CompletedAt.Sub(check.StartedAt).Round(time.Second)
					statusText = fmt.Sprintf(" (%s)", duration)
				}
			case "failure":
				icon = "✗"
				style = mergeCheckFailureStyle
				statusText = " (failed)"
				hasFailures = true
			case "cancelled":
				icon = "⊘"
				style = mergeCheckPendingStyle
				statusText = " (cancelled)"
			case "skipped":
				icon = "⊘"
				style = mergeDimStyle
				statusText = " (skipped)"
			default:
				icon = "?"
				style = mergeCheckPendingStyle
				statusText = fmt.Sprintf(" (%s)", check.Conclusion)
			}
		case "in_progress":
			icon = "●"
			style = mergeCheckPendingStyle
			if !check.StartedAt.IsZero() {
				elapsed := time.Since(check.StartedAt).Round(time.Second)
				statusText = fmt.Sprintf(" (running %s)", elapsed)
			} else {
				statusText = " (running)"
			}
		case "queued":
			icon = "○"
			style = mergeDimStyle
			statusText = " (queued)"
		default:
			icon = "○"
			style = mergeDimStyle
			statusText = fmt.Sprintf(" (%s)", check.Status)
		}

		checkLine := fmt.Sprintf("    %s %s%s", icon, check.Name, statusText)
		b.WriteString(style.Render(checkLine))
		b.WriteString("\n")

		// Show error log if enabled and check failed
		if m.showErrorLogs && check.Conclusion == "failure" && check.ErrorLog != "" {
			b.WriteString(mergeErrorStyle.Render("      └─ Error: "))
			// Show error log, indented
			errorLines := strings.Split(check.ErrorLog, "\n")
			for i, line := range errorLines {
				if i > 0 {
					b.WriteString("         ")
				}
				b.WriteString(mergeDimStyle.Render(line))
				if i < len(errorLines)-1 {
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}

		// Show failed step if available
		if check.Conclusion == "failure" && check.FailedStep != "" {
			b.WriteString(mergeErrorStyle.Render(fmt.Sprintf("      └─ Failed step: %s", check.FailedStep)))
			b.WriteString("\n")
		}
	}

	// Error logs toggle hint
	if hasFailures {
		if m.showErrorLogs {
			b.WriteString(mergeDimStyle.Render("\n  Press 'e' to hide error details"))
		} else {
			b.WriteString(mergeDimStyle.Render("\n  Press 'e' to show error details"))
		}
	}

	return mergeBoxStyle.Width(width).Render(b.String())
}

// renderProgressBarWithChecks creates a visual progress bar for given checks
func (m mergeModel) renderProgressBarWithChecks(checks *remote.PRChecks, width int) string {
	if checks == nil || checks.TotalCount == 0 {
		return ""
	}

	// Calculate progress
	completed := checks.Success + checks.Failure
	total := checks.TotalCount
	if total == 0 {
		total = 1 // Avoid division by zero
	}

	// Bar dimensions
	barWidth := width - 20 // Leave space for label and percentage
	if barWidth < 20 {
		barWidth = 20
	}

	successWidth := (checks.Success * barWidth) / total
	failureWidth := (checks.Failure * barWidth) / total
	runningWidth := (checks.InProgress * barWidth) / total
	queuedWidth := barWidth - successWidth - failureWidth - runningWidth

	// Ensure at least 1 char for running jobs if any
	if checks.InProgress > 0 && runningWidth == 0 {
		runningWidth = 1
		if queuedWidth > 0 {
			queuedWidth--
		}
	}

	// Build progress bar
	var bar strings.Builder
	bar.WriteString("  [")

	// Success (green)
	if successWidth > 0 {
		bar.WriteString(mergeCheckSuccessStyle.Render(strings.Repeat("█", successWidth)))
	}

	// Failure (red)
	if failureWidth > 0 {
		bar.WriteString(mergeCheckFailureStyle.Render(strings.Repeat("█", failureWidth)))
	}

	// Running (yellow/orange)
	if runningWidth > 0 {
		bar.WriteString(mergeCheckPendingStyle.Render(strings.Repeat("▓", runningWidth)))
	}

	// Queued (dim)
	if queuedWidth > 0 {
		bar.WriteString(mergeDimStyle.Render(strings.Repeat("░", queuedWidth)))
	}

	bar.WriteString("]")

	// Percentage
	percentage := (completed * 100) / total
	fmt.Fprintf(&bar, " %d%%", percentage)

	return bar.String()
}

func (m mergeModel) renderConfirmation() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(mergeWarningStyle.Render("  ⚠ Confirm Merge"))
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "  You are about to merge PR #%d into %s\n",
		m.prNumber, m.prDetails.BaseBranch)
	fmt.Fprintf(&b, "  Method: %s\n", mergeMethods[m.mergeMethod])
	b.WriteString("\n")

	// Show what will happen after merge
	var postActions []string
	if m.prConfig.DeleteBranchAfterMerge {
		postActions = append(postActions, fmt.Sprintf("Delete branch '%s'", m.prDetails.HeadBranch))
	}
	if m.prConfig.CheckoutAfterMerge {
		postActions = append(postActions, "Checkout to main")
	}
	if m.prConfig.SyncAfterMerge {
		postActions = append(postActions, "Sync with remote")
	}

	if len(postActions) > 0 {
		b.WriteString("  After merge:\n")
		for _, action := range postActions {
			fmt.Fprintf(&b, "    • %s\n", action)
		}
		b.WriteString("\n")
	}

	b.WriteString(mergeSelectedStyle.Render("  Press [Y] to confirm, [N] or [Esc] to cancel"))
	b.WriteString("\n")

	return b.String()
}

func (m mergeModel) renderQuitConfirmation() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(mergeSuccessStyle.Render("  ✓ Merge Complete"))
	b.WriteString("\n\n")

	if m.mainPipelineCheck != nil {
		switch m.mainPipelineCheck.Status {
		case "success":
			b.WriteString("  All CI pipelines passed!\n")
		case "failure":
			b.WriteString(mergeWarningStyle.Render("  ⚠ Some CI pipelines failed\n"))
		}
	}

	b.WriteString("\n")
	b.WriteString(mergeSelectedStyle.Render("  Press [Q] to quit, [Enter] to stay"))
	b.WriteString("\n")

	return b.String()
}

func (m mergeModel) renderMergeMethod(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeMethod {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("⚙ Merge Method"))
	b.WriteString("\n")

	for i, method := range mergeMethods {
		var prefix string
		var style lipgloss.Style
		if i == m.mergeMethod {
			prefix = "● "
			style = mergeSelectedStyle
		} else {
			prefix = "○ "
			style = mergeUnselectedStyle
		}

		var description string
		switch method {
		case "squash":
			description = "Squash and merge (combine all commits into one)"
		case "merge":
			description = "Create a merge commit"
		case "rebase":
			description = "Rebase and merge (linear history)"
		}

		line := fmt.Sprintf("  %s%s - %s", prefix, method, description)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return boxStyle.Render(b.String())
}

func (m mergeModel) renderCommitMessage(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeMessage {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("✏ Commit Message"))
	b.WriteString("\n")

	// Title
	titleLabel := "Title:"
	if m.focus == focusMergeMessage && !m.editingBody {
		titleLabel = "Title: (editing)"
	}
	b.WriteString(mergeDimStyle.Render("  " + titleLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.commitTitle.View())
	b.WriteString("\n\n")

	// Body
	bodyLabel := "Body:"
	if m.focus == focusMergeMessage && m.editingBody {
		bodyLabel = "Body: (editing)"
	}
	b.WriteString(mergeDimStyle.Render("  " + bodyLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.commitBody.View())

	return boxStyle.Render(b.String())
}

func (m mergeModel) renderActions(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeActions {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("🚀 Actions"))
	b.WriteString("\n")

	for i, action := range mergeActions {
		var style lipgloss.Style
		var prefix string
		if i == m.actionCursor && m.focus == focusMergeActions {
			prefix = "▶ "
			style = mergeSelectedStyle
		} else {
			prefix = "  "
			style = mergeUnselectedStyle
		}

		b.WriteString(style.Render(prefix + action))
		b.WriteString("\n")
	}

	return boxStyle.Render(b.String())
}

// truncateStr truncates a string to maxLen with ellipsis
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
