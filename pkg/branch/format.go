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

// Status icons
const (
	iconActive    = "🔄"
	iconStale     = "💤"
	iconMerged    = "✅"
	iconProtected = "🔒"
	iconOrphan    = "👻"
	iconLocal     = "📍"
	iconRemote    = "☁️"
	iconBoth      = "🔄"
	iconPROpen    = "🟢"
	iconPRMerged  = "🟣"
	iconPRClosed  = "🔴"
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
	sb.WriteString(fmt.Sprintf("\n%s%-40s %-12s %-8s %-14s %-14s %-18s%s\n",
		colorBold,
		"BRANCH",
		"STATUS",
		"PR",
		"LOCAL",
		"REMOTE",
		"AUTHOR",
		colorReset,
	))
	sb.WriteString(strings.Repeat("─", 110) + "\n")

	// Branches
	for _, b := range result.Branches {
		sb.WriteString(formatBranchLine(b))
	}

	// Footer
	sb.WriteString(strings.Repeat("─", 110) + "\n")
	sb.WriteString(formatSummary(result.Summary))

	// Suggestions
	if result.Summary.Merged > 0 {
		sb.WriteString(fmt.Sprintf("\n%s💡 Run 'cidx branch cleanup' to remove %d merged branch(es)%s\n",
			colorDim, result.Summary.Merged, colorReset))
	}
	if result.Summary.Stale > 0 {
		sb.WriteString(fmt.Sprintf("%s💡 Run 'cidx branch stale' to see %d stale branch(es)%s\n",
			colorDim, result.Summary.Stale, colorReset))
	}

	return sb.String()
}

// formatBranchLine formats a single branch line
func formatBranchLine(b Info) string {
	// Branch name with location icon
	locationIcon := getLocationIcon(b.Location)
	name := truncate(b.Name, 37)

	// Status with icon and color
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

	return fmt.Sprintf("%s %-37s %-12s %-8s %-14s %-14s %-16s\n",
		locationIcon, name, status, pr, localInfo, remoteInfo, author)
}

// formatCommitInfo formats commit date and hash
func formatCommitInfo(t time.Time, hash string) string {
	if t.IsZero() {
		return fmt.Sprintf("%s-%s", colorDim, colorReset)
	}

	age := formatAge(t)
	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}

	return fmt.Sprintf("%s %s%s%s", age, colorDim, shortHash, colorReset)
}

// getLocationIcon returns the icon for branch location
func getLocationIcon(loc Location) string {
	switch loc {
	case LocationLocal:
		return iconLocal
	case LocationRemote:
		return iconRemote
	case LocationBoth:
		return iconBoth
	default:
		return " "
	}
}

// formatStatus formats the branch status with icon and color
func formatStatus(s Status) string {
	switch s {
	case StatusActive:
		return fmt.Sprintf("%s%s active%s", colorGreen, iconActive, colorReset)
	case StatusStale:
		return fmt.Sprintf("%s%s stale%s", colorYellow, iconStale, colorReset)
	case StatusMerged:
		return fmt.Sprintf("%s%s merged%s", colorCyan, iconMerged, colorReset)
	case StatusProtected:
		return fmt.Sprintf("%s%s protect%s", colorBlue, iconProtected, colorReset)
	case StatusOrphan:
		return fmt.Sprintf("%s%s orphan%s", colorRed, iconOrphan, colorReset)
	default:
		return string(s)
	}
}

// formatPR formats the PR number and status
func formatPR(number int, status PRStatus) string {
	if number == 0 {
		return fmt.Sprintf("%s-%s", colorDim, colorReset)
	}

	var icon string
	switch status {
	case PRStatusOpen:
		icon = iconPROpen
	case PRStatusMerged:
		icon = iconPRMerged
	case PRStatusClosed:
		icon = iconPRClosed
	}

	return fmt.Sprintf("%s#%d", icon, number)
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
