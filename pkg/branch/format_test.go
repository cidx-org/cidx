package branch

import (
	"strings"
	"testing"
	"time"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"longer", "hello world", 8, "hello..."},
		{"very short max", "hello", 3, "hel"},
		{"max 2", "hello", 2, "he"},
		{"max 1", "hello", 1, "h"},
		{"max 0", "hello", 0, ""},
		{"empty string", "", 5, ""},
		{"max 4 with ellipsis", "hello world", 4, "h..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCalculateColumnWidths_WideTerminal(t *testing.T) {
	branches := []Info{
		{Name: "main", LocalAuthor: "alice", LocalCommitSubject: "initial"},
	}

	widths := calculateColumnWidths(branches, 200)

	if widths.marker != fixedMarker {
		t.Errorf("marker width = %d, want %d", widths.marker, fixedMarker)
	}
	if widths.status != fixedStatus {
		t.Errorf("status width = %d, want %d", widths.status, fixedStatus)
	}
	if widths.pr != fixedPR {
		t.Errorf("pr width = %d, want %d", widths.pr, fixedPR)
	}
	if widths.local != fixedLocal {
		t.Errorf("local width = %d, want %d", widths.local, fixedLocal)
	}
	if widths.remote != fixedRemote {
		t.Errorf("remote width = %d, want %d", widths.remote, fixedRemote)
	}
	// With wide terminal, flexible columns should fit
	if widths.branch < len("BRANCH") {
		t.Errorf("branch width %d too small for header", widths.branch)
	}
}

func TestCalculateColumnWidths_NarrowTerminal(t *testing.T) {
	branches := []Info{
		{Name: "feature/very-long-branch-name-here", LocalAuthor: "developer", LocalCommitSubject: "a long commit subject"},
	}

	widths := calculateColumnWidths(branches, 80)

	// Fixed columns must remain fixed
	if widths.marker != fixedMarker {
		t.Errorf("marker width = %d, want %d", widths.marker, fixedMarker)
	}
	// Branch should have at least minBranch
	if widths.branch < minBranch {
		t.Errorf("branch width %d < minimum %d", widths.branch, minBranch)
	}
	// Author should have at least minAuthor
	if widths.author < minAuthor {
		t.Errorf("author width %d < minimum %d", widths.author, minAuthor)
	}
}

func TestCalculateColumnWidths_EmptyBranches(t *testing.T) {
	widths := calculateColumnWidths(nil, 160)

	// Should still use header lengths as minimums
	if widths.branch < len("BRANCH") {
		t.Errorf("branch width %d < header length", widths.branch)
	}
}

func TestFormatCommitInfo_ZeroTime(t *testing.T) {
	result := formatCommitInfo(time.Time{}, "")
	if !strings.Contains(result, "--") {
		t.Errorf("expected '--' for zero time, got %q", result)
	}
}

func TestFormatCommitInfo_WithTime(t *testing.T) {
	ts := time.Date(2025, time.November, 28, 12, 30, 0, 0, time.UTC)
	result := formatCommitInfo(ts, "abc1234567890")

	// Should contain short hash (7 chars)
	if !strings.Contains(result, "abc1234") {
		t.Errorf("expected short hash in output, got %q", result)
	}
	// Should not contain full hash
	if strings.Contains(result, "abc1234567890") {
		t.Errorf("should truncate hash to 7 chars, got %q", result)
	}
}

func TestFormatCommitInfo_ShortHash(t *testing.T) {
	ts := time.Date(2025, time.June, 15, 10, 0, 0, 0, time.UTC)
	result := formatCommitInfo(ts, "abcd")

	// Short hash should be used as-is
	if !strings.Contains(result, "abcd") {
		t.Errorf("expected short hash preserved, got %q", result)
	}
}

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		color  string
		label  string
	}{
		{"active", StatusActive, colorGreen, "active"},
		{"stale", StatusStale, colorYellow, "stale"},
		{"merged", StatusMerged, colorCyan, "merged"},
		{"protected", StatusProtected, colorBlue, "protected"},
		{"orphan", StatusOrphan, colorRed, "orphan"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatStatus(tt.status)
			if !strings.Contains(result, tt.label) {
				t.Errorf("formatStatus(%q) = %q, should contain %q", tt.status, result, tt.label)
			}
			if !strings.Contains(result, tt.color) {
				t.Errorf("formatStatus(%q) should contain color code", tt.status)
			}
			if !strings.Contains(result, colorReset) {
				t.Errorf("formatStatus(%q) should contain reset code", tt.status)
			}
		})
	}
}

func TestFormatStatus_Unknown(t *testing.T) {
	result := formatStatus(Status("custom"))
	if !strings.Contains(result, "custom") {
		t.Errorf("formatStatus for unknown should contain the status string, got %q", result)
	}
}
