package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

func baselineCatalogue() map[string][]string {
	return map[string][]string{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest:                 {"trivy"},
		"commitizen/commitizen:4.16.5@sha256:" + zeroDigest:      {"commitizen"},
		"dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest: {"go-test", "go-build", "gofmt"},
	}
}

// noScanResults is the state where the baseline has no evidence of what the
// images carry. It must not print a zero: "not scanned" and "carries nothing"
// are the two answers this file exists to keep apart.
func noScanResults() map[string][]presets.Finding { return nil }

// baselineBases is what the scanners reported as the base of each catalogue
// image, and noBases is the state where nothing was scanned.
func baselineBases() map[string]presets.BaseOS {
	return map[string]presets.BaseOS{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest:                 {Family: "debian", Version: "13.6"},
		"commitizen/commitizen:4.16.5@sha256:" + zeroDigest:      {Family: "alpine", Version: "3.24.1"},
		"dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest: {Family: "alpine", Version: "3.21.4"},
	}
}

func noBases() map[string]presets.BaseOS { return nil }

// TestSecurityBaselineStatesTheBaseOfEveryImage: the base decides whether the
// carried findings can ever go away, and it was the one thing the scans already
// knew and the record threw away (#303).
func TestSecurityBaselineStatesTheBaseOfEveryImage(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), baselineBases())

	for _, want := range []string{"debian 13.6", "alpine 3.24.1", "alpine 3.21.4"} {
		if !strings.Contains(out, want) {
			t.Errorf("baseline does not state the base %q:\n%s", want, out)
		}
	}
}

// TestSecurityBaselineNeverPrintsAnUnscannedBaseAsAbsent: an image nothing
// scanned has an unknown base, and an image built on scratch has none. Printing
// them the same way is the confusion this file exists to remove — the same
// distinction the carried column makes between "not scanned" and 0.
func TestSecurityBaselineNeverPrintsAnUnscannedBaseAsAbsent(t *testing.T) {
	scratch := map[string]presets.BaseOS{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest: {},
	}

	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), scratch)

	if !strings.Contains(out, "| none |") {
		t.Errorf("an image with no distribution base should read `none`:\n%s", out)
	}
	if strings.Count(out, "not scanned") < 2 {
		t.Errorf("the two images nobody scanned should read `not scanned`:\n%s", out)
	}
}

// TestSecurityBaselineCarriesNoEndOfSupportDate: the date support ends is
// relative to the day it is read and comes from a third party, so it would
// churn this committed file without anything changing about the catalogue —
// exactly what keeping the generation date out prevents. The base is a fact
// about what we ship; the countdown is not.
func TestSecurityBaselineCarriesNoEndOfSupportDate(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), baselineBases())

	if date := isoDate.FindString(out); date != "" {
		t.Errorf("baseline carries the date %q, which no catalogue change would move", date)
	}
}

// TestSecurityBaselineIsDeterministic: the file is committed so that its diff
// says what changed. Map iteration order has already produced flapping output
// twice in this repository (#230, #233), and a baseline that reordered itself
// every run would make the diff worthless.
func TestSecurityBaselineIsDeterministic(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0002", Repository: "dhi.io/trivy", Severity: "HIGH", Status: "awaiting-upstream", Expires: "2026-09-01", Notes: "b"},
		{CVE: "CVE-2026-0001", Repository: "dhi.io/trivy", Severity: "CRITICAL", Status: "accepted-risk", Expires: "2026-09-01", Notes: "a"},
		{CVE: "CVE-2026-0003", Repository: "commitizen/commitizen", Severity: "HIGH", Status: "third-party", Expires: "2026-09-01", Notes: "c"},
	}
	carried := map[string][]presets.Finding{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest: {
			{ID: "CVE-2026-0001", Severity: "CRITICAL", Package: "stdlib", PackageType: "gobinary"},
			{ID: "CVE-2026-0002", Severity: "HIGH", Package: "openssl", PackageType: "debian", FixedIn: "3.0.1"},
		},
		"commitizen/commitizen:4.16.5@sha256:" + zeroDigest:      {{ID: "CVE-2026-0003", Severity: "HIGH", Package: "libxml2", PackageType: "debian"}},
		"dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest: nil,
	}

	first := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases())
	for i := 0; i < 20; i++ {
		if got := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases()); got != first {
			t.Fatalf("generation %d differs from the first:\n%s", i, got)
		}
	}
}

// isoDate matches the only dates the baseline is allowed to carry: the expiry of
// an accepted finding, which is data. A generation timestamp would change the
// file on every run, so every release would carry a diff that says nothing.
var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func TestSecurityBaselineCarriesNoGenerationDate(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases())

	if date := isoDate.FindString(out); date != "" {
		t.Errorf("baseline of a catalogue with no accepted finding carries the date %q", date)
	}
}

// TestSecurityBaselineSortsCriticalFirst: the table is read top-down, and the
// worst finding on an image has to be the first one seen.
func TestSecurityBaselineSortsCriticalFirst(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0002", Repository: "dhi.io/trivy", Severity: "HIGH", Expires: "2026-09-01"},
		{CVE: "CVE-2026-0001", Repository: "dhi.io/trivy", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases())
	critical, high := strings.Index(out, "CVE-2026-0001"), strings.Index(out, "CVE-2026-0002")

	if critical < 0 || high < 0 {
		t.Fatalf("both findings should be listed:\n%s", out)
	}
	if critical > high {
		t.Errorf("CRITICAL listed after HIGH for the same image")
	}
}

// TestSecurityBaselineIgnoresExceptionsForRepositoriesNoLongerRun: publishing an
// entry recorded against a repository the catalogue has moved past would claim an
// acceptance that waives nothing — the exact fiction the baseline exists to
// remove.
func TestSecurityBaselineIgnoresExceptionsForRepositoriesNoLongerRun(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Repository: "aquasec/trivy", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases())

	if strings.Contains(out, "CVE-2026-0001") {
		t.Errorf("an exception for a repository the catalogue no longer runs is published as accepted:\n%s", out)
	}
	if !strings.Contains(out, "No HIGH/CRITICAL finding is accepted") {
		t.Errorf("baseline should state that nothing is accepted:\n%s", out)
	}
}

// TestSecurityBaselineMatchesOnTheRepository: exceptions are keyed by repository
// (#238) while the catalogue is pinned `repo:tag@sha256:...` (#242). Matching on
// the full reference would publish an empty baseline for ever.
func TestSecurityBaselineMatchesOnTheRepository(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Repository: "dhi.io/trivy", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases())

	if !strings.Contains(out, "CVE-2026-0001") {
		t.Errorf("the tag and digest kept the exception from matching its own image:\n%s", out)
	}
}

// TestSecurityBaselineKeepsAJustificationInItsOwnRow: notes are free text, and a
// pipe in one would silently split the row it is written in.
func TestSecurityBaselineKeepsAJustificationInItsOwnRow(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Repository: "dhi.io/trivy", Severity: "HIGH", Expires: "2026-09-01",
			Notes: "fixed upstream | not yet released"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases())

	if !strings.Contains(out, `fixed upstream \| not yet released`) {
		t.Errorf("a pipe in the justification was not escaped:\n%s", out)
	}
}

// TestSecurityBaselineListsEveryCatalogueImage: an image with nothing accepted
// on it is the majority case and the most important one — the table has to say
// "we ship this, and nothing is waived on it".
func TestSecurityBaselineListsEveryCatalogueImage(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases())

	for image := range baselineCatalogue() {
		if !strings.Contains(out, image) {
			t.Errorf("catalogue image %s is missing from the baseline", image)
		}
	}
}

// TestSecurityBaselineStatesCarriedAndAcceptedSeparately: the whole point of
// #238. Publishing only what is accepted is how the file came to read "0
// accepted findings" while the catalogue carried 596, and neither number can
// stand for the other.
func TestSecurityBaselineStatesCarriedAndAcceptedSeparately(t *testing.T) {
	carried := map[string][]presets.Finding{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest: {
			{ID: "CVE-2026-0010", Severity: "HIGH", Package: "openssl", PackageType: "debian", FixedIn: "3.0.1"},
			{ID: "CVE-2026-0011", Severity: "HIGH", Package: "stdlib", PackageType: "gobinary"},
			{ID: "CVE-2026-0012", Severity: "HIGH", Package: "linux-libc-dev", PackageType: "debian"},
			{ID: "CVE-2026-0013", Severity: "CRITICAL", Package: "libxml2", PackageType: "debian"},
		},
		"commitizen/commitizen:4.16.5@sha256:" + zeroDigest:      nil,
		"dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest: nil,
	}

	out := renderSecurityBaseline(baselineCatalogue(), nil, carried, baselineBases())

	for _, want := range []string{
		"carries **4** HIGH/CRITICAL",
		"| Fixed upstream | 1 |",
		"| Go stdlib in a CLI binary | 1 |",
		"| Kernel headers | 1 |",
		"| **Needing triage** | **1** |",
		"No HIGH/CRITICAL finding is accepted",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("baseline does not state %q:\n%s", want, out)
		}
	}
}

// TestSecurityBaselineNeverPrintsAnUnscannedImageAsClean: an absent number is
// not a zero, and the fiction this file removed must not come back through the
// carried column.
func TestSecurityBaselineNeverPrintsAnUnscannedImageAsClean(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases())

	if strings.Contains(out, "| 0 |") {
		t.Errorf("an image nobody scanned is reported as carrying nothing:\n%s", out)
	}
	if !strings.Contains(out, "not scanned") {
		t.Errorf("baseline should say the images were not scanned:\n%s", out)
	}
	if !strings.Contains(out, "An absent number is not a\nzero") {
		t.Errorf("baseline should say why the carried counts are missing:\n%s", out)
	}
}
