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

	first := renderSecurityBaseline(baselineCatalogue(), accepted, carried)
	for i := 0; i < 20; i++ {
		if got := renderSecurityBaseline(baselineCatalogue(), accepted, carried); got != first {
			t.Fatalf("generation %d differs from the first:\n%s", i, got)
		}
	}
}

// isoDate matches the only dates the baseline is allowed to carry: the expiry of
// an accepted finding, which is data. A generation timestamp would change the
// file on every run, so every release would carry a diff that says nothing.
var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func TestSecurityBaselineCarriesNoGenerationDate(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults())

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults())
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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults())

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults())

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults())

	if !strings.Contains(out, `fixed upstream \| not yet released`) {
		t.Errorf("a pipe in the justification was not escaped:\n%s", out)
	}
}

// TestSecurityBaselineListsEveryCatalogueImage: an image with nothing accepted
// on it is the majority case and the most important one — the table has to say
// "we ship this, and nothing is waived on it".
func TestSecurityBaselineListsEveryCatalogueImage(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults())

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

	out := renderSecurityBaseline(baselineCatalogue(), nil, carried)

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
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults())

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
