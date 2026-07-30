package main

import (
	"regexp"
	"strings"
	"testing"
)

func baselineCatalogue() map[string][]string {
	return map[string][]string{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest:                 {"trivy"},
		"commitizen/commitizen:4.16.5@sha256:" + zeroDigest:      {"commitizen"},
		"dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest: {"go-test", "go-build", "gofmt"},
	}
}

// TestSecurityBaselineIsDeterministic: the file is committed so that its diff
// says what changed. Map iteration order has already produced flapping output
// twice in this repository (#230, #233), and a baseline that reordered itself
// every run would make the diff worthless.
func TestSecurityBaselineIsDeterministic(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0002", Image: "dhi.io/trivy:0.68", Severity: "HIGH", Status: "awaiting-upstream", Expires: "2026-09-01", Notes: "b"},
		{CVE: "CVE-2026-0001", Image: "dhi.io/trivy:0.68", Severity: "CRITICAL", Status: "accepted-risk", Expires: "2026-09-01", Notes: "a"},
		{CVE: "CVE-2026-0003", Image: "commitizen/commitizen:4.16.5", Severity: "HIGH", Status: "third-party", Expires: "2026-09-01", Notes: "c"},
	}

	first := renderSecurityBaseline(baselineCatalogue(), accepted)
	for i := 0; i < 20; i++ {
		if got := renderSecurityBaseline(baselineCatalogue(), accepted); got != first {
			t.Fatalf("generation %d differs from the first:\n%s", i, got)
		}
	}
}

// isoDate matches the only dates the baseline is allowed to carry: the expiry of
// an accepted finding, which is data. A generation timestamp would change the
// file on every run, so every release would carry a diff that says nothing.
var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func TestSecurityBaselineCarriesNoGenerationDate(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil)

	if date := isoDate.FindString(out); date != "" {
		t.Errorf("baseline of a catalogue with no accepted finding carries the date %q", date)
	}
}

// TestSecurityBaselineSortsCriticalFirst: the table is read top-down, and the
// worst finding on an image has to be the first one seen.
func TestSecurityBaselineSortsCriticalFirst(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0002", Image: "dhi.io/trivy:0.68", Severity: "HIGH", Expires: "2026-09-01"},
		{CVE: "CVE-2026-0001", Image: "dhi.io/trivy:0.68", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted)
	critical, high := strings.Index(out, "CVE-2026-0001"), strings.Index(out, "CVE-2026-0002")

	if critical < 0 || high < 0 {
		t.Fatalf("both findings should be listed:\n%s", out)
	}
	if critical > high {
		t.Errorf("CRITICAL listed after HIGH for the same image")
	}
}

// TestSecurityBaselineIgnoresExceptionsForImagesNoLongerRun: publishing an entry
// recorded against a tag the catalogue has moved past would claim an acceptance
// that waives nothing — the exact fiction the baseline exists to remove.
func TestSecurityBaselineIgnoresExceptionsForImagesNoLongerRun(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Image: "aquasec/trivy:0.67.2", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted)

	if strings.Contains(out, "CVE-2026-0001") {
		t.Errorf("an exception for an image the catalogue no longer runs is published as accepted:\n%s", out)
	}
	if !strings.Contains(out, "No HIGH/CRITICAL finding is accepted") {
		t.Errorf("baseline should state that nothing is accepted:\n%s", out)
	}
}

// TestSecurityBaselineMatchesOnTheDigestFreeReference: exceptions are recorded
// against `repo:tag` while the catalogue is pinned `repo:tag@sha256:...` (#242).
// Matching on the full reference would publish an empty baseline for ever.
func TestSecurityBaselineMatchesOnTheDigestFreeReference(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Image: "dhi.io/trivy:0.68", Severity: "CRITICAL", Expires: "2026-09-01"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted)

	if !strings.Contains(out, "CVE-2026-0001") {
		t.Errorf("the digest kept the exception from matching its own image:\n%s", out)
	}
}

// TestSecurityBaselineKeepsAJustificationInItsOwnRow: notes are free text, and a
// pipe in one would silently split the row it is written in.
func TestSecurityBaselineKeepsAJustificationInItsOwnRow(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Image: "dhi.io/trivy:0.68", Severity: "HIGH", Expires: "2026-09-01",
			Notes: "fixed upstream | not yet released"},
	}

	out := renderSecurityBaseline(baselineCatalogue(), accepted)

	if !strings.Contains(out, `fixed upstream \| not yet released`) {
		t.Errorf("a pipe in the justification was not escaped:\n%s", out)
	}
}

// TestSecurityBaselineListsEveryCatalogueImage: an image with nothing accepted
// on it is the majority case and the most important one — the table has to say
// "we ship this, and nothing is waived on it".
func TestSecurityBaselineListsEveryCatalogueImage(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil)

	for image := range baselineCatalogue() {
		if !strings.Contains(out, image) {
			t.Errorf("catalogue image %s is missing from the baseline", image)
		}
	}
}
