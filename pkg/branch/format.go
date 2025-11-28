package branch

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
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

// Column configuration
type columnWidths struct {
	marker  int // Fixed: 3
	branch  int // Flexible
	status  int // Fixed: 9
	pr      int // Fixed: 6
	local   int // Fixed: 20
	remote  int // Fixed: 20
	author  int // Flexible
	subject int // Flexible (lowest priority)
}

// Fixed column widths
const (
	fixedMarker = 3
	fixedStatus = 9
	fixedPR     = 6
	fixedLocal  = 20
	fixedRemote = 20
	minBranch   = 15
	minAuthor   = 10
	minSubject  = 0 // Can be hidden entirely
)

// getTerminalWidth returns the terminal width or a default value
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 160 // Default for non-terminal or error
	}
	return width
}

// calculateColumnWidths determines optimal column widths based on content and terminal width
func calculateColumnWidths(branches []Info, termWidth int) columnWidths {
	// Find max content lengths
	maxBranch := len("BRANCH")
	maxAuthor := len("LAST AUTHOR")
	maxSubject := len("SUBJECT")

	for _, b := range branches {
		if len(b.Name) > maxBranch {
			maxBranch = len(b.Name)
		}

		author := b.LocalAuthor
		if author == "" {
			author = b.RemoteAuthor
		}
		if len(author) > maxAuthor {
			maxAuthor = len(author)
		}

		subject := b.LocalCommitSubject
		if subject == "" {
			subject = b.RemoteCommitSubject
		}
		if len(subject) > maxSubject {
			maxSubject = len(subject)
		}
	}

	// Calculate fixed space needed (including separators)
	// marker(3) + space + branch + space + status(9) + space + pr(6) + space + local(20) + space + remote(20) + space + author + space + subject
	fixedSpace := fixedMarker + fixedStatus + fixedPR + fixedLocal + fixedRemote + 7 // 7 spaces between columns

	// Available space for flexible columns
	availableForFlex := termWidth - fixedSpace

	// Ideal widths for flexible columns
	idealBranch := maxBranch
	idealAuthor := maxAuthor
	idealSubject := maxSubject
	totalIdeal := idealBranch + idealAuthor + idealSubject

	var widths columnWidths
	widths.marker = fixedMarker
	widths.status = fixedStatus
	widths.pr = fixedPR
	widths.local = fixedLocal
	widths.remote = fixedRemote

	if totalIdeal <= availableForFlex {
		// Everything fits - use ideal widths
		widths.branch = idealBranch
		widths.author = idealAuthor
		widths.subject = idealSubject
	} else {
		// Need to truncate - prioritize: branch > author > subject
		remaining := availableForFlex

		// Allocate branch (up to ideal, min minBranch)
		widths.branch = min(idealBranch, max(minBranch, remaining/3))
		remaining -= widths.branch

		// Allocate author (up to ideal, min minAuthor)
		widths.author = min(idealAuthor, max(minAuthor, remaining/2))
		remaining -= widths.author

		// Subject gets the rest (can be 0)
		widths.subject = max(0, remaining)
	}

	return widths
}

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

	termWidth := getTerminalWidth()
	widths := calculateColumnWidths(result.Branches, termWidth)

	var sb strings.Builder

	// Calculate total table width
	tableWidth := widths.marker + 1 + widths.branch + 1 + widths.status + 1 + widths.pr + 1 + widths.local + 1 + widths.remote + 1 + widths.author
	if widths.subject > 0 {
		tableWidth += 1 + widths.subject
	}

	// Header
	sb.WriteString(fmt.Sprintf("\n%s%-*s %-*s %-*s %-*s %-*s %-*s %-*s",
		colorBold,
		widths.marker, "",
		widths.branch, "BRANCH",
		widths.status, "STATUS",
		widths.pr, "PR",
		widths.local, "LOCAL",
		widths.remote, "REMOTE",
		widths.author, "LAST AUTHOR",
	))
	if widths.subject > 0 {
		sb.WriteString(fmt.Sprintf(" %-*s", widths.subject, "SUBJECT"))
	}
	sb.WriteString(fmt.Sprintf("%s\n", colorReset))
	sb.WriteString(strings.Repeat("─", tableWidth) + "\n")

	// Branches
	for _, b := range result.Branches {
		sb.WriteString(formatBranchLine(b, widths))
	}

	// Footer
	sb.WriteString(strings.Repeat("─", tableWidth) + "\n")
	sb.WriteString(formatSummary(result.Summary))

	// Legend
	sb.WriteString(formatLegend())

	// Warning if no GitHub token
	if !result.HasGitHubToken {
		sb.WriteString(fmt.Sprintf("\n%s⚠ No GitHub token: PR info unavailable (set GITHUB_TOKEN or run 'gh auth login')%s\n",
			colorYellow, colorReset))
	}

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
func formatBranchLine(b Info, widths columnWidths) string {
	// Location marker
	marker := getLocationMarker(b.Location)

	// Branch name (truncate if needed)
	name := truncate(b.Name, widths.branch)

	// Status with color
	status := formatStatus(b.Status)

	// PR info
	pr := formatPR(b.PRNumber, b.PRStatus)

	// Local info (date + hash)
	localInfo := formatCommitInfo(b.LocalCommitDate, b.LocalCommitHash)

	// Remote info (date + hash)
	remoteInfo := formatCommitInfo(b.RemoteCommitDate, b.RemoteCommitHash)

	// Author (prefer local, fallback to remote)
	author := b.LocalAuthor
	if author == "" {
		author = b.RemoteAuthor
	}
	author = truncate(author, widths.author)

	// Subject (prefer local, fallback to remote)
	subject := b.LocalCommitSubject
	if subject == "" {
		subject = b.RemoteCommitSubject
	}

	// Build the line
	line := fmt.Sprintf("%s %-*s %s %-*s %-*s %-*s %-*s",
		marker,
		widths.branch, name,
		status,
		widths.pr, pr,
		widths.local, localInfo,
		widths.remote, remoteInfo,
		widths.author, author,
	)

	if widths.subject > 0 {
		subject = truncate(subject, widths.subject)
		line += fmt.Sprintf(" %s", subject)
	}

	return line + "\n"
}

// formatCommitInfo formats commit date and hash
// Returns a string of exactly 20 visible characters (date + space + hash)
func formatCommitInfo(t time.Time, hash string) string {
	if t.IsZero() {
		// Pad to 20 chars to match "Nov 28 12:30 1234567" format
		return fmt.Sprintf("%s%-20s%s", colorDim, "--", colorReset)
	}

	// Format as local date/time: "Nov 28 12:30"
	dateStr := t.Local().Format("Jan 02 15:04")
	shortHash := hash
	if len(shortHash) > 7 {
		shortHash = shortHash[:7]
	}

	return fmt.Sprintf("%s %s", dateStr, shortHash)
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

// formatStatus formats the branch status with color (fixed width: 9 chars)
func formatStatus(s Status) string {
	// All status strings are padded to 9 chars (length of "protected")
	switch s {
	case StatusActive:
		return fmt.Sprintf("%s%-9s%s", colorGreen, "active", colorReset)
	case StatusStale:
		return fmt.Sprintf("%s%-9s%s", colorYellow, "stale", colorReset)
	case StatusMerged:
		return fmt.Sprintf("%s%-9s%s", colorCyan, "merged", colorReset)
	case StatusProtected:
		return fmt.Sprintf("%s%-9s%s", colorBlue, "protected", colorReset)
	case StatusOrphan:
		return fmt.Sprintf("%s%-9s%s", colorRed, "orphan", colorReset)
	default:
		return fmt.Sprintf("%-9s", string(s))
	}
}

// formatPR formats the PR number and status
func formatPR(number int, status PRStatus) string {
	if number == 0 {
		// Pad to match typical PR width
		return fmt.Sprintf("%s%-6s%s", colorDim, "--", colorReset)
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

	return fmt.Sprintf("%s#%-5d%s", color, number, colorReset)
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

// formatLegend formats the legend explaining markers and colors
func formatLegend() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%sLocation:%s [L]=local  [R]=remote  [B]=both    ", colorDim, colorReset))
	sb.WriteString(fmt.Sprintf("%sStatus:%s %sactive%s %sstale%s %smerged%s %sprotected%s %sorphan%s\n",
		colorDim, colorReset,
		colorGreen, colorReset,
		colorYellow, colorReset,
		colorCyan, colorReset,
		colorBlue, colorReset,
		colorRed, colorReset,
	))

	return sb.String()
}

// truncate truncates a string to max length with ellipsis
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
