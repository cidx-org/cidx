package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// The assembly half of `cidx security summary`: which evidence it reads and
// what it leaves out. What the page *says* is specified in
// features/security/vulnerability_status_summary.feature; these cover the
// wiring the scenarios cannot reach, and none of them touches the network —
// endoflife.date goes through the eolCyclesFunc seam.

func summaryCatalogue() map[string][]string {
	return map[string][]string{
		runningRef:   {"prettier"},
		candidateRef: {"prettier-next"},
	}
}

func summaryDay() time.Time { return time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC) }

// TestSummaryNamesTheImagesNobodyScanned: an image with no result carries no
// findings on the page, and the page has to say so. A scan leg that died must
// never read as a catalogue that improved overnight — the same distinction
// SECURITY-BASELINE.md prints as `not scanned` rather than `0`.
func TestSummaryNamesTheImagesNobodyScanned(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) { return alpineFeed(), nil })

	dir := t.TempDir()
	writeTrivyResult(t, dir, runningRef, map[string]string{"CVE-2026-1000": "HIGH"})

	summary := buildCatalogueSummary(summaryCatalogue(), dir, nil, summaryDay())

	if len(summary.Unscanned) != 1 || summary.Unscanned[0] != candidateRef {
		t.Fatalf("unscanned = %v, want exactly %s", summary.Unscanned, candidateRef)
	}
	if summary.Scanned() != 1 {
		t.Errorf("scanned = %d, want 1", summary.Scanned())
	}

	page := presets.RenderSummary(summary)
	if !strings.Contains(page, "produced no scan result") || !strings.Contains(page, candidateRef) {
		t.Errorf("the page does not name the unscanned image:\n%s", page)
	}
}

// TestSummaryReadsTheFindingsBothScannersReported: the page counts a
// vulnerability once however many scanners saw it, because it reads the same
// triage the baseline and the Security tab read. Two counts of one catalogue is
// the one thing this page must not introduce.
func TestSummaryReadsTheFindingsBothScannersReported(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) { return alpineFeed(), nil })

	dir := t.TempDir()
	for _, image := range []string{runningRef, candidateRef} {
		writeTrivyResult(t, dir, image, map[string]string{"CVE-2026-1001": "HIGH"})
		writeGrypeResult(t, dir, image, map[string]string{"CVE-2026-1001": "High"})
	}

	summary := buildCatalogueSummary(summaryCatalogue(), dir, nil, summaryDay())

	// One vulnerability, two images: two findings, because each one is its own
	// repin. Not four, because each image saw it twice.
	if summary.Triage.Carried != 2 {
		t.Errorf("carried = %d, want 2 (one per image, not one per scanner)", summary.Triage.Carried)
	}
	if summary.Triage.Actionable != 2 {
		t.Errorf("needing triage = %d, want 2", summary.Triage.Actionable)
	}
}

// TestSummaryCountsOnlyTheAcceptancesPastTheirDate: an acceptance still within
// its date is doing its job and is not a decision waiting for anybody.
func TestSummaryCountsOnlyTheAcceptancesPastTheirDate(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) { return alpineFeed(), nil })

	accepted := []Vulnerability{
		{CVE: "CVE-2026-1002", Repository: "tmknom/prettier", Severity: "HIGH", Expires: "2020-01-01"},
		{CVE: "CVE-2026-1003", Repository: "tmknom/prettier", Severity: "HIGH", Expires: "2999-01-01"},
		// Out of the band the policy acts on, so out of the page.
		{CVE: "CVE-2026-1004", Repository: "tmknom/prettier", Severity: "MEDIUM", Expires: "2020-01-01"},
	}

	summary := buildCatalogueSummary(summaryCatalogue(), t.TempDir(), accepted, summaryDay())

	if len(summary.Expired) != 1 || !strings.EqualFold(summary.Expired[0].CVE, "CVE-2026-1002") {
		t.Fatalf("expired = %+v, want only CVE-2026-1002", summary.Expired)
	}

	page := presets.RenderSummary(summary)
	if !strings.Contains(page, "CVE-2026-1002") {
		t.Errorf("the page does not name the lapsed acceptance:\n%s", page)
	}
	if strings.Contains(page, "CVE-2026-1003") {
		t.Errorf("the page names an acceptance still within its date:\n%s", page)
	}
}

// TestSummaryKeepsOnlyTheBasesNeedingADecision: twenty supported bases printed
// daily is how a section teaches its reader to skip it. An unchecked one is
// kept, because an endoflife.date outage must never read as a checked base.
func TestSummaryKeepsOnlyTheBasesNeedingADecision(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) { return alpineFeed(), nil })

	dir := t.TempDir()
	// 3.20 ended on 2026-04-01, 3.24 runs to 2028: one decision, one silence.
	writeTrivyBase(t, dir, runningRef, "alpine", "3.20.10")
	writeTrivyBase(t, dir, candidateRef, "alpine", "3.24.1")

	summary := buildCatalogueSummary(summaryCatalogue(), dir, nil, summaryDay())

	if len(summary.Bases) != 1 || summary.Bases[0].Image != runningRef {
		t.Fatalf("bases = %+v, want only %s", summary.Bases, runningRef)
	}
	if summary.Bases[0].State != presets.BaseEnded {
		t.Errorf("base state = %q, want %q", summary.Bases[0].State, presets.BaseEnded)
	}
	if got := summary.Digest().BasesEnded; got != 1 {
		t.Errorf("digest reports %d base(s) past end of support, want 1", got)
	}
}

// TestSummaryReportsAnEndOfLifeOutageRatherThanSilence: the audit already
// refuses to let a third-party outage change a verdict, and the page has to
// carry the same posture — `unchecked` stated, never omitted.
func TestSummaryReportsAnEndOfLifeOutageRatherThanSilence(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) {
		return nil, errNoScanResults // any error: the page only says the check did not happen
	})

	dir := t.TempDir()
	writeTrivyBase(t, dir, runningRef, "alpine", "3.20.10")
	writeTrivyBase(t, dir, candidateRef, "alpine", "3.24.1")

	summary := buildCatalogueSummary(summaryCatalogue(), dir, nil, summaryDay())

	if got := summary.Digest().BasesUnchecked; got != 2 {
		t.Errorf("digest reports %d unchecked base(s), want 2", got)
	}
	if summary.Digest().BasesEnded != 0 {
		t.Error("an unchecked base was counted as one past its end of support")
	}
}

// TestSummaryLinksComeFromTheRunOrNowhere: the page carries no repository of its
// own. In a workflow run it reads the one GitHub sets; anywhere else it renders
// the same text with no hyperlinks, because a guessed repository is a link that
// goes somewhere else.
func TestSummaryLinksComeFromTheRunOrNowhere(t *testing.T) {
	t.Setenv("GITHUB_SERVER_URL", "https://github.com")
	t.Setenv("GITHUB_REPOSITORY", "cidx-org/cidx")
	t.Setenv("GITHUB_RUN_ID", "42")

	links := summaryLinks()
	if links.Repo != "https://github.com/cidx-org/cidx" {
		t.Errorf("repo = %q", links.Repo)
	}
	if links.Run != "https://github.com/cidx-org/cidx/actions/runs/42" {
		t.Errorf("run = %q", links.Run)
	}

	t.Setenv("GITHUB_REPOSITORY", "")
	if links := summaryLinks(); links.Repo != "" || links.Run != "" {
		t.Errorf("links outside a run = %+v, want empty", links)
	}
}

// TestSummaryRendersWithoutLinksOutsideARun: the degraded form still has to be
// a readable page, or nobody checks it before pushing it.
func TestSummaryRendersWithoutLinksOutsideARun(t *testing.T) {
	page := presets.RenderSummary(presets.CatalogueSummary{Images: 1, Day: summaryDay()})

	if strings.Contains(page, "]()") || strings.Contains(page, "](") {
		t.Errorf("the page renders an empty or partial link outside a run:\n%s", page)
	}
	if !strings.Contains(page, "the Security tab") {
		t.Errorf("the page drops the text along with the link:\n%s", page)
	}
}

// TestSummaryIsWrittenWhereItIsAsked: the workflow hands the body to
// `gh issue edit --body-file`, so the file is the interface.
func TestSummaryIsWrittenWhereItIsAsked(t *testing.T) {
	stubEOL(t, func(string) ([]presets.EOLCycle, error) { return alpineFeed(), nil })

	out := filepath.Join(t.TempDir(), "status.md")
	if err := newApp().Run([]string{"cidx", "security", "summary", "--results", t.TempDir(), "-o", out}); err != nil {
		t.Fatalf("cidx security summary: %v", err)
	}

	page, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the page was not written: %v", err)
	}
	if !strings.HasPrefix(string(page), "# Vulnerability status") {
		t.Errorf("unexpected page:\n%s", page)
	}
}
