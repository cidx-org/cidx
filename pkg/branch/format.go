package branch

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
)

// Location markers (ASCII for alignment)
const (
	markerLocal  = "[L]"
	markerRemote = "[R]"
	markerBoth   = "[B]"
)

// FormatList formats the branch list for terminal output
func FormatList(result *ListResult, asJSON bool) string {
	if asJSON {
		return formatJSON(result)
	}
	return formatTable(result)
}

// formatJSON returns JSON output
func formatJSON(result *ListResult) string {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting JSON: %v", err)
	}
	return string(data)
}

// formatTable returns a formatted table
func formatTable(result *ListResult) string {
	if len(result.Branches) == 0 {
		return "No branches found matching the criteria."
	}

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("\n%s%-3s %-32s %-10s %-6s %-12s %-12s %-18s %-30s%s\n",
		colorBold,
		"",
		"BRANCH",
		"STATUS",
		"PR",
		"LOCAL",
		"REMOTE",
		"LAST AUTHOR",
		"SUBJECT",
		colorReset,
	))
	sb.WriteString(strings.Repeat("─", 130) + "\n")

	// Branches
	for _, b := range result.Branches {
		sb.WriteString(formatBranchLine(b))
	}

	// Footer
	sb.WriteString(strings.Repeat("─", 130) + "\n")
	sb.WriteString(formatSummary(result.Summary))

	// Suggestions
	if result.Summary.Merged > 0 {
		sb.WriteString(fmt.Sprintf("\n%sTip: Run 'cidx branch cleanup' to remove %d merged branch(es)%s\n",
			colorDim, result.Summary.Merged, colorReset))
	}
	if result.Summary.Stale > 0 {
		sb.WriteString(fmt.Sprintf("%sTip: Run 'cidx branch stale' to see %d stale branch(es)%s\n",
			colorDim, result.Summary.Stale, colorReset))
	}

	return sb.String()
}

// formatBranchLine formats a single branch line
func formatBranchLine(b Info) string {
	// Location marker
	marker := getLocationMarker(b.Location)

	// Branch name
	name := truncate(b.Name, 30)

	// Status with color
	status := formatStatus(b.Status)

	// PR info
	pr := formatPR(b.PRNumber, b.PRStatus)

	// Local info (age + hash)
	localInfo := formatCommitInfo(b.LocalCommitDate, b.LocalCommitHash)

	// Remote info (age + hash)
	remoteInfo := formatCommitInfo(b.RemoteCommitDate, b.RemoteCommitHash)

	// Author (prefer local, fallback to remote)
	author := b.LocalAuthor
	if author == "" {
		author = b.RemoteAuthor
	}
	author = truncate(author, 16)

	// Subject (prefer local, fallback to remote)
	subject := b.LocalCommitSubject
	if subject == "" {
		subject = b.RemoteCommitSubject
	}
	subject = truncate(subject, 28)

	return fmt.Sprintf("%s %-30s %-10s %-6s %-12s %-12s %-16s %s\n",
		marker, name, status, pr, localInfo, remoteInfo, author, subject)
}

// formatCommitInfo formats commit date and hash
func formatCommitInfo(t time.Time, hash string) string {
	if t.IsZero() {
		return fmt.Sprintf("%s--%s", colorDim, colorReset)
	}

	age := formatAge(t)
	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}

	return fmt.Sprintf("%-4s %s", age, shortHash)
}

// getLocationMarker returns the marker for branch location
func getLocationMarker(loc Location) string {
	switch loc {
	case LocationLocal:
		return markerLocal
	case LocationRemote:
		return markerRemote
	case LocationBoth:
		return markerBoth
	default:
		return "   "
	}
}

// formatStatus formats the branch status with color
func formatStatus(s Status) string {
	switch s {
	case StatusActive:
		return fmt.Sprintf("%sactive%s", colorGreen, colorReset)
	case StatusStale:
		return fmt.Sprintf("%sstale%s", colorYellow, colorReset)
	case StatusMerged:
		return fmt.Sprintf("%smerged%s", colorCyan, colorReset)
	case StatusProtected:
		return fmt.Sprintf("%sprotected%s", colorBlue, colorReset)
	case StatusOrphan:
		return fmt.Sprintf("%sorphan%s", colorRed, colorReset)
	default:
		return string(s)
	}
}

// formatPR formats the PR number and status
func formatPR(number int, status PRStatus) string {
	if number == 0 {
		return fmt.Sprintf("%s--%s", colorDim, colorReset)
	}

	var color string
	switch status {
	case PRStatusOpen:
		color = colorGreen
	case PRStatusMerged:
		color = colorCyan
	case PRStatusClosed:
		color = colorRed
	default:
		color = ""
	}

	return fmt.Sprintf("%s#%d%s", color, number, colorReset)
}

// formatAge formats the time since last commit
func formatAge(t time.Time) string {
	duration := time.Since(t)

	days := int(duration.Hours() / 24)
	if days == 0 {
		hours := int(duration.Hours())
		if hours == 0 {
			return "now"
		}
		return fmt.Sprintf("%dh", hours)
	}
	if days < 7 {
		return fmt.Sprintf("%dd", days)
	}
	if days < 30 {
		return fmt.Sprintf("%dw", days/7)
	}
	if days < 365 {
		return fmt.Sprintf("%dm", days/30)
	}
	return fmt.Sprintf("%dy", days/365)
}

// formatSummary formats the summary statistics
func formatSummary(s Summary) string {
	parts := []string{}

	if s.Protected > 0 {
		parts = append(parts, fmt.Sprintf("%s%d protected%s", colorBlue, s.Protected, colorReset))
	}
	if s.Active > 0 {
		parts = append(parts, fmt.Sprintf("%s%d active%s", colorGreen, s.Active, colorReset))
	}
	if s.Stale > 0 {
		parts = append(parts, fmt.Sprintf("%s%d stale%s", colorYellow, s.Stale, colorReset))
	}
	if s.Merged > 0 {
		parts = append(parts, fmt.Sprintf("%s%d merged%s", colorCyan, s.Merged, colorReset))
	}
	if s.Orphan > 0 {
		parts = append(parts, fmt.Sprintf("%s%d orphan%s", colorRed, s.Orphan, colorReset))
	}

	return fmt.Sprintf("%s%d branches%s (%s)\n",
		colorBold, s.Total, colorReset,
		strings.Join(parts, ", "))
}

// truncate truncates a string to max length with ellipsis
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
