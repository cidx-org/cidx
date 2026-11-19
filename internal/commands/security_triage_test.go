package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

var triageDay = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

const triageImage = "dhi.io/trivy:0.68@sha256:" + zeroDigest

// writeTriageFixture leaves what both scanners said about one image: a finding
// they share (Trivy by CVE, Grype by GHSA with the CVE related), one only Trivy
// reports, one only Grype reports, and one accepted finding the ignore file
// moved aside.
func writeTriageFixture(t *testing.T, dir, image string) {
	t.Helper()

	trivyVuln := func(id, pkg, installed, severity, fixed, status, description string, vendor ...string) map[string]any {
		return map[string]any{
			"VulnerabilityID": id, "PkgName": pkg, "InstalledVersion": installed,
			"Severity": severity, "FixedVersion": fixed, "Status": status,
			"Description": description, "VendorIDs": vendor,
		}
	}
	writeJSON(t, filepath.Join(dir, scanResultFile("trivy", image)), map[string]any{
		"Results": []any{map[string]any{
			"Type": "debian",
			"Vulnerabilities": []any{
				trivyVuln("CVE-2026-0001", "openssl", "3.0.1", "HIGH", "3.0.2", "fixed", "Short.", "GHSA-aaaa-bbbb-cccc"),
				trivyVuln("CVE-2026-0002", "zlib", "1.2.0", "CRITICAL", "", "affected",
					"A buffer overflow\n\tin inflate,\nreachable from a crafted stream."),
				trivyVuln("CVE-2026-0003", "curl", "8.0", "LOW", "", "affected", "below the band"),
			},
			"ExperimentalModifiedFindings": []any{map[string]any{
				"Status":  "ignored",
				"Finding": trivyVuln("CVE-2026-0004", "libxml2", "2.9", "HIGH", "", "affected", "Accepted one."),
			}},
		}},
	})

	grypeMatch := func(id, pkg, version, severity, state, description string, epss float64, kev bool, related ...string) map[string]any {
		vuln := map[string]any{
			"id": id, "severity": severity, "description": description,
			"fix":  map[string]any{"state": state, "versions": []string{}},
			"epss": []any{map[string]any{"epss": epss}},
		}
		if kev {
			vuln["kev"] = []any{map[string]any{"cve": id}}
		}
		var relatedVulns []any
		for _, r := range related {
			relatedVulns = append(relatedVulns, map[string]any{"id": r, "description": "related " + r})
		}
		return map[string]any{
			"vulnerability":          vuln,
			"relatedVulnerabilities": relatedVulns,
			"artifact":               map[string]any{"name": pkg, "version": version, "type": "deb"},
		}
	}
	writeJSON(t, filepath.Join(dir, scanResultFile("grype", image)), map[string]any{
		"matches": []any{
			grypeMatch("CVE-2026-0001", "openssl", "3.0.1", "High", "fixed", "A much longer description of the openssl flaw.", 0.42, true, "GHSA-aaaa-bbbb-cccc"),
			grypeMatch("CVE-2026-0005", "busybox", "1.36", "High", "not-fixed", "", 0, false),
		},
		"ignoredMatches": []any{},
	})
}

func triageFixtureAccepted() []Vulnerability {
	return []Vulnerability{
		{CVE: "CVE-2026-0002", Repository: "dhi.io/trivy", Severity: "CRITICAL", Status: "awaiting-upstream", Expires: "2026-12-31", Notes: "no fix\tupstream"},
		{CVE: "CVE-2026-0004", Repository: "dhi.io/trivy", Severity: "HIGH", Status: "accepted-risk", Expires: "2026-01-01", Notes: "expired one"},
		{CVE: "CVE-2025-9999", Repository: "dhi.io/trivy", Severity: "HIGH", Status: "accepted-risk", Expires: "2027-03-01", Notes: "fixed long ago"},
		{CVE: "CVE-2026-0500", Repository: "other/image", Severity: "HIGH", Expires: "2027-03-01", Notes: "someone else's"},
	}
}

func triageFixtureSheet(t *testing.T) (map[string][]presets.Finding, map[string]triageSheet) {
	t.Helper()
	dir := t.TempDir()
	writeTriageFixture(t, dir, triageImage)

	accepted := triageFixtureAccepted()
	carried, _ := carriedFindings(map[string][]string{triageImage: {"trivy"}}, dir, accepted)
	return carried, triageSheets(carried, accepted, triageDay)
}

func sheetRows(t *testing.T, sheet triageSheet) map[string][]string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(sheet.render(), "\n"), "\n")
	if got := strings.Split(lines[0], "\t"); len(got) != len(triageHeader) || got[0] != "id" {
		t.Fatalf("first line is not the header: %q", lines[0])
	}
	rows := make(map[string][]string)
	for _, line := range lines[1:] {
		cells := strings.Split(line, "\t")
		if len(cells) != len(triageHeader) {
			t.Fatalf("row has %d cells, want %d: %q", len(cells), len(triageHeader), line)
		}
		rows[cells[0]] = cells
	}
	return rows
}

func column(name string) int {
	for i, h := range triageHeader {
		if h == name {
			return i
		}
	}
	panic("no column " + name)
}

// TestTriageSheetHasOneLinePerVulnerabilityAndPackage: both scanners' views of a
// finding are one line, carrying what each knew.
func TestTriageSheetHasOneLinePerVulnerabilityAndPackage(t *testing.T) {
	_, sheets := triageFixtureSheet(t)
	rows := sheetRows(t, sheets[triageImage])

	// 0001, 0002, 0004 (hidden but accepted), 0005 and the stale 9999; 0003 is LOW.
	if len(rows) != 5 {
		t.Fatalf("sheet has %d lines, want 5: %v", len(rows), rows)
	}
	if _, ok := rows["CVE-2026-0003"]; ok {
		t.Error("a LOW finding is on the sheet")
	}

	shared := rows["CVE-2026-0001"]
	for name, want := range map[string]string{
		"aliases":   "GHSA-aaaa-bbbb-cccc",
		"package":   "openssl",
		"installed": "3.0.1",
		"severity":  "HIGH",
		"fix":       "fixed",
		"fixed_in":  "3.0.2",
		"scanners":  "grype,trivy",
		"epss":      "0.42",
		"kev":       "yes",
	} {
		if got := shared[column(name)]; got != want {
			t.Errorf("CVE-2026-0001 %s = %q, want %q", name, got, want)
		}
	}
	if got := shared[column("description")]; got != "A much longer description of the openssl flaw." {
		t.Errorf("the fullest description was not kept: %q", got)
	}

	grypeOnly := rows["CVE-2026-0005"]
	if grypeOnly[column("scanners")] != "grype" || grypeOnly[column("fix")] != "not-fixed" || grypeOnly[column("epss")] != "" || grypeOnly[column("kev")] != "" {
		t.Errorf("a Grype-only finding is misreported: %v", grypeOnly)
	}
}

// TestTriageSheetPutsTheWholeDescriptionOnOneLine: a newline or a tab inside a
// cell would split the row, and truncating would defeat the sheet.
func TestTriageSheetPutsTheWholeDescriptionOnOneLine(t *testing.T) {
	_, sheets := triageFixtureSheet(t)
	rows := sheetRows(t, sheets[triageImage])

	want := "A buffer overflow in inflate, reachable from a crafted stream."
	if got := rows["CVE-2026-0002"][column("description")]; got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
	if got := rows["CVE-2026-0002"][column("accepted_reason")]; got != "no fix upstream" {
		t.Errorf("reason = %q", got)
	}
}

// TestTriageSheetSortsBySeverityThenIdentifier is the order the sheet is read in.
func TestTriageSheetSortsBySeverityThenIdentifier(t *testing.T) {
	_, sheets := triageFixtureSheet(t)

	var ids []string
	for _, r := range sheets[triageImage].Rows {
		ids = append(ids, r.ID)
	}
	want := []string{"CVE-2026-0002", "CVE-2025-9999", "CVE-2026-0001", "CVE-2026-0004", "CVE-2026-0005"}
	if strings.Join(ids, " ") != strings.Join(want, " ") {
		t.Errorf("order = %v, want %v", ids, want)
	}
}

// TestTriageSheetShowsTheCurrentAcceptance: reason and date beside the finding,
// and nothing for a finding nobody accepted — including when the entry waives
// nothing any more.
func TestTriageSheetShowsTheCurrentAcceptance(t *testing.T) {
	_, sheets := triageFixtureSheet(t)
	rows := sheetRows(t, sheets[triageImage])

	active := rows["CVE-2026-0002"]
	if active[column("acceptance")] != "accepted" || active[column("accepted_until")] != "2026-12-31" {
		t.Errorf("active acceptance misreported: %v", active)
	}

	lapsed := rows["CVE-2026-0004"]
	if lapsed[column("acceptance")] != "expired" || lapsed[column("accepted_until")] != "2026-01-01" {
		t.Errorf("an expired acceptance is not marked: %v", lapsed)
	}
	if lapsed[column("package")] != "libxml2" {
		t.Errorf("an accepted finding the ignore file hid is missing from the sheet: %v", lapsed)
	}

	open := rows["CVE-2026-0001"]
	if open[column("acceptance")] != "" || open[column("accepted_reason")] != "" || open[column("accepted_until")] != "" {
		t.Errorf("a finding with no acceptance shows one: %v", open)
	}
}

// TestTriageSheetListsStaleAcceptances: an entry matching nothing carried is on
// the sheet so it can be removed, and an entry on another repository is not.
func TestTriageSheetListsStaleAcceptances(t *testing.T) {
	_, sheets := triageFixtureSheet(t)
	rows := sheetRows(t, sheets[triageImage])

	stale, ok := rows["CVE-2025-9999"]
	if !ok {
		t.Fatalf("the stale acceptance is not on the sheet: %v", rows)
	}
	if stale[column("acceptance")] != "stale" || stale[column("accepted_until")] != "2027-03-01" || stale[column("accepted_reason")] != "fixed long ago" {
		t.Errorf("stale line = %v", stale)
	}
	if _, ok := rows["CVE-2026-0500"]; ok {
		t.Error("an acceptance on another repository is on this image's sheet")
	}
}

// TestTriageSheetMatchesAnAcceptanceByAlias: the entry names the CVE, the scanner
// reported the GHSA.
func TestTriageSheetMatchesAnAcceptanceByAlias(t *testing.T) {
	findings := []presets.Finding{{
		ID: "GHSA-aaaa-bbbb-cccc", Severity: "HIGH", Package: "x", Scanner: "Grype", Aliases: "CVE-2026-0700",
	}}
	entries := []Vulnerability{{CVE: "CVE-2026-0700", Severity: "HIGH", Expires: "2027-01-01", Notes: "n"}}

	rows := buildTriageSheet(findings, entries, triageDay).Rows
	if len(rows) != 1 || rows[0].Acceptance != "accepted" {
		t.Errorf("an acceptance written for the CVE does not cover its GHSA: %+v", rows)
	}
}

// TestTriageIndexSummarisesEachImage pins the counts and their definitions.
func TestTriageIndexSummarisesEachImage(t *testing.T) {
	_, sheets := triageFixtureSheet(t)

	lines := strings.Split(strings.TrimSuffix(renderTriageIndex(sheets, nil), "\n"), "\n")
	if lines[0] != strings.Join(triageIndexHeader, "\t") {
		t.Fatalf("header = %q", lines[0])
	}
	// 4 carried (0001, 0002, 0004, 0005); 2 accepted (0002, 0004); 2 without;
	// 1 fixable (0001); 1 stale (9999); 1 expired (0004).
	want := triageImage + "\t4\t2\t2\t1\t1\t1\t"
	if len(lines) != 2 || lines[1] != want {
		t.Errorf("index = %q, want [header, %q]", lines, want)
	}
}

// TestWriteTriageLeavesOneSheetPerScannedImage: files land where the baseline
// says, an image that left the catalogue loses its sheet, and an unscanned image
// gets none rather than an empty one.
func TestWriteTriageLeavesOneSheetPerScannedImage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "triage")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(dir, "gone_image.tsv")
	keep := filepath.Join(dir, "notes.txt")
	for _, f := range []string{leftover, keep} {
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	carried, _ := triageFixtureSheet(t)
	if n, err := writeTriage(dir, carried, triageFixtureAccepted(), nil, triageDay); err != nil || n != 1 {
		t.Fatalf("writeTriage = %d, %v", n, err)
	}

	if _, err := os.Stat(filepath.Join(dir, triageSheetFile(triageImage))); err != nil {
		t.Errorf("the image has no sheet: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, triageIndexFile)); err != nil {
		t.Errorf("no index: %v", err)
	}
	if _, err := os.Stat(leftover); err == nil {
		t.Error("a sheet from an earlier run survived")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Error("a file that is not a sheet was removed")
	}
}

// TestTriageIsDeterministic: two runs over the same results write the same bytes.
func TestTriageIsDeterministic(t *testing.T) {
	_, first := triageFixtureSheet(t)
	for i := 0; i < 20; i++ {
		_, again := triageFixtureSheet(t)
		if again[triageImage].render() != first[triageImage].render() {
			t.Fatalf("run %d differs from the first", i)
		}
	}
}

// TestTriageDoesNotChangeTheBaseline: the extra fields the sheet reads are
// carried on the findings and weigh nothing, so the committed file is the same
// for the same results.
func TestTriageDoesNotChangeTheBaseline(t *testing.T) {
	carried, _ := triageFixtureSheet(t)

	bare := make(map[string][]presets.Finding)
	for image, findings := range carried {
		for _, f := range findings {
			bare[image] = append(bare[image], presets.Finding{
				ID: f.ID, Severity: f.Severity, Package: f.Package, PackageType: f.PackageType,
				FixedIn: f.FixedIn, EPSS: f.EPSS, KEV: f.KEV, Scanner: f.Scanner,
			})
		}
	}

	catalogue := map[string][]string{triageImage: {"trivy"}}
	accepted := triageFixtureAccepted()
	if rich, plain := renderSecurityBaseline(catalogue, accepted, carried, nil, true), renderSecurityBaseline(catalogue, accepted, bare, nil, true); rich != plain {
		t.Errorf("the baseline depends on the fields only the sheet reads:\n%s\n---\n%s", rich, plain)
	}
}
