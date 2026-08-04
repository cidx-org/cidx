package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), baselineBases(), true)

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

	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), scratch, true)

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
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), baselineBases(), true)

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

	first := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases(), true)
	for i := 0; i < 20; i++ {
		if got := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases(), true); got != first {
			t.Fatalf("generation %d differs from the first:\n%s", i, got)
		}
	}
}

// isoDate matches the only dates the baseline is allowed to carry: the expiry of
// an accepted finding, which is data. A generation timestamp would change the
// file on every run, so every release would carry a diff that says nothing.
var isoDate = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func TestSecurityBaselineCarriesNoGenerationDate(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases(), true)

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases(), true)
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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases(), true)

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases(), true)

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

	out := renderSecurityBaseline(baselineCatalogue(), accepted, noScanResults(), noBases(), true)

	if !strings.Contains(out, `fixed upstream \| not yet released`) {
		t.Errorf("a pipe in the justification was not escaped:\n%s", out)
	}
}

// TestSecurityBaselineListsEveryCatalogueImage: an image with nothing accepted
// on it is the majority case and the most important one — the table has to say
// "we ship this, and nothing is waived on it".
func TestSecurityBaselineListsEveryCatalogueImage(t *testing.T) {
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases(), true)

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

	out := renderSecurityBaseline(baselineCatalogue(), nil, carried, baselineBases(), true)

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
	out := renderSecurityBaseline(baselineCatalogue(), nil, noScanResults(), noBases(), true)

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

// TestSecurityBaselineCountsWhatTheIgnoreFileHid is the first half of #310.
//
// The audit generates each image's ignore file out of the entries accepted on
// that image's repository, so an accepted finding is deleted from that image's
// own visible results (#238). Reading only the visible half published "what the
// catalogue carries" with the carried findings we had already argued about
// subtracted from it — the one population the file's readers are most likely to
// ask about. Both scanners keep the other half now (#311), so the number can
// finally be the whole one.
//
// Only the accepted identifiers, though. Grype ships a default rule dropping
// indirect `linux-libc-dev` matches — 188 of them on the Rust image — and
// counting a scanner's own defaults would move this number without anything
// changing about the catalogue, which is the one thing this committed file may
// never do.
func TestSecurityBaselineCountsWhatTheIgnoreFileHid(t *testing.T) {
	dir := t.TempDir()
	image := "dhi.io/trivy:0.68@sha256:" + zeroDigest

	writeTrivyResult(t, dir, image, map[string]string{"CVE-2026-0001": "HIGH"})
	writeSuppressedTrivyResult(t, dir, image,
		map[string]string{"CVE-2026-0001": "HIGH"},
		map[string]string{"CVE-2026-0002": "CRITICAL"})
	// Grype suppressed a kernel-header match nobody accepted, under its own rule.
	writeGrypeResultWithIgnored(t, dir, image,
		map[string]string{"CVE-2026-0001": "High"},
		map[string]string{"CVE-2026-0500": "High"})

	carried, recorded := carriedFindings(map[string][]string{image: {"trivy"}}, dir,
		[]Vulnerability{{CVE: "CVE-2026-0002", Repository: "dhi.io/trivy", Severity: "CRITICAL"}})

	if !recorded {
		t.Error("a Trivy result recording a suppressed finding was not recognised as evidence")
	}
	if got := presets.Summarise(carried[image]).Carried; got != 2 {
		t.Errorf("carried = %d, want 2: the accepted CVE the ignore file removed is carried, the scanner's own default suppression is not", got)
	}
	for _, f := range carried[image] {
		if f.ID == "CVE-2026-0500" {
			t.Error("a finding the scanner suppressed under its own default rule is counted as carried")
		}
	}
}

// writeGrypeResultWithIgnored writes a Grype document whose ignore rules moved
// some matches aside — ours, and its own.
func writeGrypeResultWithIgnored(t *testing.T, dir, image string, visible, ignored map[string]string) {
	t.Helper()

	match := func(findings map[string]string) []any {
		var matches []any
		for id, severity := range findings {
			matches = append(matches, map[string]any{
				"vulnerability": map[string]any{"id": id, "severity": severity},
			})
		}
		return matches
	}

	writeJSON(t, filepath.Join(dir, scanResultFile("grype", image)), map[string]any{
		"matches":        match(visible),
		"ignoredMatches": match(ignored),
	})
}

// TestSecurityBaselineNeverPublishesAFloorAsATotal keeps the count consistent
// with the posture #324 settled on: `ExperimentalModifiedFindings` is
// `omitempty`, so a Trivy report says nothing about whether `--show-suppressed`
// was passed. On results that record no suppression at all the accepted
// findings are missing from the count and cannot be added back — which the file
// has to say, rather than publish the number as the total.
func TestSecurityBaselineNeverPublishesAFloorAsATotal(t *testing.T) {
	accepted := []Vulnerability{
		{CVE: "CVE-2026-0001", Repository: "dhi.io/trivy", Severity: "HIGH", Expires: "2026-09-01"},
	}
	carried := map[string][]presets.Finding{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest: {{ID: "CVE-2026-0009", Severity: "HIGH", Package: "zlib", PackageType: "debian"}},
	}

	unrecorded := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases(), false)
	if !strings.Contains(unrecorded, "Read it as a floor.") {
		t.Errorf("results recording no suppression are published as a total:\n%s", unrecorded)
	}

	recorded := renderSecurityBaseline(baselineCatalogue(), accepted, carried, baselineBases(), true)
	if strings.Contains(recorded, "Read it as a floor.") {
		t.Errorf("results that do record what they suppressed are hedged anyway:\n%s", recorded)
	}
}

// writeSuppressedTrivyResult writes the document `--show-suppressed` produces:
// the findings the report still shows, plus the ones the ignore file moved to
// `ExperimentalModifiedFindings` rather than dropped.
func writeSuppressedTrivyResult(t *testing.T, dir, image string, visible, hidden map[string]string) {
	t.Helper()

	type vuln struct {
		VulnerabilityID string
		Severity        string
	}
	var vulns []vuln
	for id, severity := range visible {
		vulns = append(vulns, vuln{VulnerabilityID: id, Severity: severity})
	}
	var modified []any
	for id, severity := range hidden {
		modified = append(modified, map[string]any{
			"Status":  "ignored",
			"Finding": vuln{VulnerabilityID: id, Severity: severity},
		})
	}

	writeJSON(t, filepath.Join(dir, scanResultFile("trivy", image)), map[string]any{
		"ArtifactName": image,
		"Results": []any{map[string]any{
			"Vulnerabilities":              vulns,
			"ExperimentalModifiedFindings": modified,
		}},
	})
}

// TestSecurityBaselineIsCurrent is the second half of #310, and the standing
// guard that keeps it from happening again.
//
// SECURITY-BASELINE.md is committed so that its diff is the history of what the
// catalogue delivers. Nothing regenerated it: it was written by a hand-run
// command and committed when someone remembered, so the file on main had no
// `Base` column months after #305 started emitting one, and a release attached
// it as an asset all the same. A file that is not an output of its own
// generator publishes a catalogue that does not exist.
//
// The check that would settle it in one line — regenerate, compare bytes — is
// not available to a test. Half of this file is measured by scanners whose
// databases move without this repository moving, and the only job holding that
// evidence is the daily audit, whose artifacts live for a day. A gate needing
// them would be red every time a CVE was published and green every time the
// download flaked, which is what #247, #271 and #272 each turned out to be.
//
// So the guard covers the half this repository decides — the document's shape,
// the images, the presets on them, and every acceptance published against them —
// which is also the half that actually rotted. It is byte-exact on every line
// that two different measurements of the same catalogue agree on, which is how
// the measured cells exclude themselves without this test having to know which
// they are. The numbers are reported by the audit instead, next to the evidence
// that would fix them.
func TestSecurityBaselineIsCurrent(t *testing.T) {
	committed, err := os.ReadFile(filepath.Join("..", "..", defaultBaselineFile))
	if err != nil {
		t.Fatalf("could not read the committed baseline: %v", err)
	}

	imagePresets, err := catalogueImages()
	if err != nil {
		t.Fatalf("could not load the catalogue: %v", err)
	}
	vulns, err := loadVulnerabilities(filepath.Join("..", "..", defaultVulnFile))
	if err != nil {
		t.Fatalf("could not read the accepted findings: %v", err)
	}

	// Two renderings of the same catalogue under measurements that disagree
	// about every cell a scanner decides: the counts, the four populations, the
	// bases, KEV and EPSS. They are built to keep the same line structure, so a
	// line the two share is one no scan result could have moved.
	firstCarried, firstBases := baselineMeasurement(imagePresets, 0)
	secondCarried, secondBases := baselineMeasurement(imagePresets, 1)
	first := strings.Split(renderSecurityBaseline(imagePresets, vulns.Vulnerabilities, firstCarried, firstBases, true), "\n")
	second := strings.Split(renderSecurityBaseline(imagePresets, vulns.Vulnerabilities, secondCarried, secondBases, true), "\n")

	if len(first) != len(second) {
		t.Fatalf("the two renderings are %d and %d lines long: this test can no longer tell a measured line from a decided one",
			len(first), len(second))
	}

	got := strings.Split(string(committed), "\n")
	at := 0
	for i, line := range first {
		if strings.TrimSpace(line) == "" || second[i] != line {
			continue // blank, or a line the scan results decide
		}

		matched := false
		for ; at < len(got); at++ {
			if got[at] == line {
				at++
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("%s is not an output of the current generator: it is missing\n\n\t%s\n\n"+
				"Regenerate it — `cidx security baseline --results <the audit's artifacts>` — and commit the result (#310).",
				defaultBaselineFile, line)
		}
	}

	// The image rows are the one thing the loop above cannot check: two of
	// their five cells are measured, so the two renderings never agree on a
	// whole row. What the catalogue ships is checked against them directly.
	rows := baselineImageRows(t, string(committed))
	if len(rows) != len(imagePresets) {
		t.Errorf("%s lists %d images, the catalogue ships %d", defaultBaselineFile, len(rows), len(imagePresets))
	}
	for image, names := range imagePresets {
		sorted := append([]string(nil), names...)
		sort.Strings(sorted)
		prefix := fmt.Sprintf("| `%s` | %s | ", image, strings.Join(sorted, ", "))

		listed := false
		for _, row := range rows {
			listed = listed || strings.HasPrefix(row, prefix)
		}
		if !listed {
			t.Errorf("%s does not list %s against the presets using it (%s) — regenerate it (#310)",
				defaultBaselineFile, image, strings.Join(sorted, ", "))
		}
	}
}

// baselineImageRows returns the rows of the committed file's image table, which
// is the block following its header up to the first blank line.
func baselineImageRows(t *testing.T, committed string) []string {
	t.Helper()

	_, table, found := strings.Cut(committed, "| Image | Presets | Base | Carried HIGH/CRITICAL | Accepted |")
	if !found {
		t.Fatalf("%s carries no image table with the columns the generator emits (#310)", defaultBaselineFile)
	}
	table, _, _ = strings.Cut(table, "\n\n")

	var rows []string
	for _, line := range strings.Split(table, "\n") {
		if strings.HasPrefix(line, "| `") {
			rows = append(rows, line)
		}
	}
	return rows
}

// baselineMeasurement fabricates what a scan of the whole catalogue would have
// said, in two variants that disagree on every measured cell while producing the
// same lines in the same places. Both carry a KEV finding and both leave every
// image scanned, because either of those changes how many lines the carried
// section is — and an alignment that shifted would silently stop checking the
// rest of the file.
func baselineMeasurement(imagePresets map[string][]string, variant int) (map[string][]presets.Finding, map[string]presets.BaseOS) {
	findings := [][]presets.Finding{{
		{ID: "CVE-2026-9000", Severity: "HIGH", Package: "openssl", PackageType: "debian", KEV: true, EPSS: 0.11},
		{ID: "CVE-2026-9001", Severity: "HIGH", Package: "curl", PackageType: "debian"},
	}, {
		{ID: "CVE-2026-9100", Severity: "CRITICAL", Package: "libxml2", PackageType: "debian", KEV: true, EPSS: 0.42},
		{ID: "CVE-2026-9101", Severity: "HIGH", Package: "stdlib", PackageType: "gobinary"},
		{ID: "CVE-2026-9102", Severity: "HIGH", Package: "linux-libc-dev", PackageType: "debian"},
		{ID: "CVE-2026-9103", Severity: "HIGH", Package: "zlib", PackageType: "debian", FixedIn: "1.3.1"},
	}}
	base := []presets.BaseOS{{Family: "debian", Version: "13.6"}, {Family: "alpine", Version: "3.24.1"}}

	carried := make(map[string][]presets.Finding, len(imagePresets))
	bases := make(map[string]presets.BaseOS, len(imagePresets))
	for image := range imagePresets {
		carried[image] = findings[variant]
		bases[image] = base[variant]
	}
	return carried, bases
}
