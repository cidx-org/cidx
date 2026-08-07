package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// pruneCatalogue is a two-image catalogue pinned by digest, as presets.toml
// writes them (#242).
func pruneCatalogue() map[string][]string {
	return map[string][]string{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest:                     {"trivy"},
		"golangci/golangci-lint:v2.12.2-alpine@sha256:" + zeroDigest: {"golangci-lint"},
	}
}

// TestCatalogueFindingsKeysOnTheRepository: exceptions are keyed by repository
// (#238), so the findings have to be indexed the same way or every lookup
// misses and every entry reads as unknown.
func TestCatalogueFindingsKeysOnTheRepository(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, "dhi.io/trivy:0.68@sha256:"+zeroDigest, map[string]string{"CVE-2026-0001": "HIGH"})
	writeTrivyResult(t, dir, "golangci/golangci-lint:v2.12.2-alpine@sha256:"+zeroDigest, nil)

	running, findings, _, _ := catalogueFindings(pruneCatalogue(), dir)

	if len(running) != 2 || running[0] != "dhi.io/trivy" {
		t.Fatalf("running = %v, want the repositories sorted", running)
	}
	got := presets.FindingIDs(findings["dhi.io/trivy"])
	if len(got) != 1 || got[0] != "CVE-2026-0001" {
		t.Errorf("findings = %v, want the CVE indexed under the repository", got)
	}
}

// TestCatalogueFindingsMergesTheTagsOfOneRepository: the catalogue runs two tags
// of `rust`, and an exception written for the repository covers both. The
// findings of both have to be behind the one key, or a CVE present on the slim
// image alone would read as gone.
func TestCatalogueFindingsMergesTheTagsOfOneRepository(t *testing.T) {
	dir := t.TempDir()
	catalogue := map[string][]string{
		"rust:1.97.0@sha256:" + zeroDigest:      {"cargo-audit"},
		"rust:1.97.0-slim@sha256:" + zeroDigest: {"cargo-build"},
	}
	writeTrivyResult(t, dir, "rust:1.97.0@sha256:"+zeroDigest, map[string]string{"CVE-2026-0001": "HIGH"})
	writeTrivyResult(t, dir, "rust:1.97.0-slim@sha256:"+zeroDigest, map[string]string{"CVE-2026-0002": "HIGH"})

	running, findings, _, _ := catalogueFindings(catalogue, dir)

	if len(running) != 1 || running[0] != "rust" {
		t.Fatalf("running = %v, want one repository", running)
	}
	if got := presets.FindingIDs(findings["rust"]); len(got) != 2 {
		t.Errorf("findings = %v, want both tags' findings under the repository", got)
	}
}

// TestCatalogueFindingsCollectsWhatTheIgnoreFileSuppressed: the evidence a live
// entry is judged on (#312). The audit's ignore file is generated from the
// entries themselves, so an accepted CVE is missing from its own repository's
// results — Grype's record of what it suppressed is the only place left that can
// say whether it has since been fixed upstream.
func TestCatalogueFindingsCollectsWhatTheIgnoreFileSuppressed(t *testing.T) {
	dir := t.TempDir()
	image := "dhi.io/trivy:0.68@sha256:" + zeroDigest
	writeTrivyResult(t, dir, image, nil)
	writeJSON(t, filepath.Join(dir, scanResultFile("grype", image)), map[string]any{
		"matches": []any{},
		"ignoredMatches": []any{map[string]any{
			"vulnerability": map[string]any{
				"id": "CVE-2026-0001", "severity": "High",
				"fix": map[string]any{"versions": []string{"0.71.1"}},
			},
		}},
	})
	writeTrivyResult(t, dir, "golangci/golangci-lint:v2.12.2-alpine@sha256:"+zeroDigest, nil)

	_, findings, suppressed, _ := catalogueFindings(pruneCatalogue(), dir)

	if got := presets.FindingIDs(findings["dhi.io/trivy"]); len(got) != 0 {
		t.Errorf("findings = %v, want the accepted CVE absent from the results it filtered itself out of", got)
	}
	if got := presets.FixVersion(suppressed["dhi.io/trivy"], "CVE-2026-0001"); got != "0.71.1" {
		t.Errorf("suppressed fix = %q, want the version Grype recorded against the ignored match", got)
	}
}

// TestCatalogueFindingsLeavesPartlyScannedRepositoriesAbsent: absent is what
// makes the verdict fail-closed. One unscanned tag is enough to leave the
// question open — the CVE could be on exactly that one — so the repository must
// not appear as answered.
func TestCatalogueFindingsLeavesPartlyScannedRepositoriesAbsent(t *testing.T) {
	dir := t.TempDir()
	catalogue := map[string][]string{
		"rust:1.97.0@sha256:" + zeroDigest:      {"cargo-audit"},
		"rust:1.97.0-slim@sha256:" + zeroDigest: {"cargo-build"},
	}
	writeTrivyResult(t, dir, "rust:1.97.0@sha256:"+zeroDigest, nil)

	_, findings, _, _ := catalogueFindings(catalogue, dir)

	if _, scanned := findings["rust"]; scanned {
		t.Errorf("a repository with an unscanned tag was recorded as scanned: %v", findings)
	}
}

// TestCatalogueFindingsLeavesUnscannedImagesAbsent: an unscanned image recorded
// as "no finding" would make every exception look purgeable.
func TestCatalogueFindingsLeavesUnscannedImagesAbsent(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, "dhi.io/trivy:0.68@sha256:"+zeroDigest, nil)

	_, findings, _, _ := catalogueFindings(pruneCatalogue(), dir)

	if _, scanned := findings["golangci/golangci-lint"]; scanned {
		t.Errorf("an image with no result file was recorded as scanned: %v", findings)
	}
	if _, scanned := findings["dhi.io/trivy"]; !scanned {
		t.Errorf("an image with a clean result file must count as scanned: %v", findings)
	}
}

// TestCatalogueFindingsReadsTheAuditFileNames: security-audit.yml flattens image
// references with `tr '/:' '__'` and leaves the `@` in place, where
// container-monitor.yml runs `tr '/:@' '___'`. The audit scans every catalogue
// image daily, so its artifacts are the natural source here — and were unreadable
// under the monitor's name alone.
func TestCatalogueFindingsReadsTheAuditFileNames(t *testing.T) {
	dir := t.TempDir()
	image := "dhi.io/trivy:0.68@sha256:" + zeroDigest
	name := "trivy-" + auditFileName.Replace(image) + ".json"
	writeTrivyResultAs(t, dir, name, map[string]string{"CVE-2026-0001": "HIGH"})

	_, findings, _, _ := catalogueFindings(pruneCatalogue(), dir)

	if got := presets.FindingIDs(findings["dhi.io/trivy"]); len(got) != 1 {
		t.Errorf("findings = %v, want the audit's result file to be read", got)
	}
}

// TestPruneRemovesOnlyObsoleteEntries: the whole point. A carry-over entry
// describes a finding a catalogue image still has, and deleting it would lose
// the justification and turn the next audit red.
func TestPruneRemovesOnlyObsoleteEntries(t *testing.T) {
	entries := []prunedEntry{
		{Vulnerability{CVE: "CVE-1", Repository: "gone/one"}, presets.ExceptionVerdict{State: presets.ExceptionObsolete}},
		{Vulnerability{CVE: "CVE-2", Repository: "gone/two"}, presets.ExceptionVerdict{State: presets.ExceptionCarryOver, StillOn: "live/one"}},
		{Vulnerability{CVE: "CVE-3", Repository: "gone/three"}, presets.ExceptionVerdict{State: presets.ExceptionUnknown}},
		{Vulnerability{CVE: "CVE-4", Repository: "live/two"}, presets.ExceptionVerdict{State: presets.ExceptionLive}},
	}

	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	vulns := &VulnerabilityFile{}
	for _, e := range entries {
		vulns.Vulnerabilities = append(vulns.Vulnerabilities, e.Vulnerability)
	}

	if err := applyPrune(path, vulns, entries); err != nil {
		t.Fatalf("applyPrune: %v", err)
	}

	saved, err := loadVulnerabilities(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	kept := map[string]bool{}
	for _, v := range saved.Vulnerabilities {
		kept[v.CVE] = true
	}
	if kept["CVE-1"] {
		t.Errorf("the obsolete entry was kept")
	}
	for _, cve := range []string{"CVE-2", "CVE-3", "CVE-4"} {
		if !kept[cve] {
			t.Errorf("%s was removed although only obsolete entries may be", cve)
		}
	}
}

// TestPruneRefilesCarryOverOntoTheCarryingRepository: the justification and the
// dates it was argued under survive the move; only the key changes, and the
// reference it came from is kept as context.
func TestPruneRefilesCarryOverOntoTheCarryingRepository(t *testing.T) {
	entries := []prunedEntry{
		{
			Vulnerability{CVE: "CVE-2013-7445", Repository: "", FirstSeen: "golangci/golangci-lint:v2.6.2",
				Severity: "HIGH", Status: "third-party", Added: "2025-12-02", Expires: "2026-03-02", Notes: "kernel headers"},
			presets.ExceptionVerdict{State: presets.ExceptionCarryOver, StillOn: "rust"},
		},
	}

	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	vulns := &VulnerabilityFile{Vulnerabilities: []Vulnerability{entries[0].Vulnerability}}

	if err := applyPrune(path, vulns, entries); err != nil {
		t.Fatalf("applyPrune: %v", err)
	}

	saved, err := loadVulnerabilities(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(saved.Vulnerabilities) != 1 {
		t.Fatalf("entries = %d, want the carry-over entry re-filed, not dropped", len(saved.Vulnerabilities))
	}

	got := saved.Vulnerabilities[0]
	if got.Repository != "rust" {
		t.Errorf("Repository = %q, want the repository that carries the CVE", got.Repository)
	}
	if got.FirstSeen != "golangci/golangci-lint:v2.6.2" {
		t.Errorf("FirstSeen = %q, want the reference it was recorded against", got.FirstSeen)
	}
	if got.Notes != "kernel headers" || got.Added != "2025-12-02" || got.Expires != "2026-03-02" {
		t.Errorf("re-filing lost the justification or the dates: %+v", got)
	}
}

// TestRefilingPrefersTheEntryAlreadyWrittenAboutTheRepository: four entries for
// the same CVE, recorded against four images the catalogue has replaced, all land
// on one repository. Only one can be the exception, and the one that already
// described that repository is the judgement made about this image rather than
// about a neighbour.
func TestRefilingPrefersTheEntryAlreadyWrittenAboutTheRepository(t *testing.T) {
	kept := resolveRefiled([]Vulnerability{
		{CVE: "CVE-2024-25621", Repository: "ghcr.io/ansible/dev-tools", FirstSeen: "docker:29.0.4", Notes: "about docker"},
		{CVE: "CVE-2024-25621", Repository: "ghcr.io/ansible/dev-tools", FirstSeen: "ghcr.io/ansible/dev-tools:v25.11.0", Notes: "about ansible"},
		{CVE: "CVE-2024-25621", Repository: "ghcr.io/ansible/dev-tools", FirstSeen: "aquasec/trivy:0.67.2", Notes: "about trivy"},
	})

	if len(kept) != 1 {
		t.Fatalf("entries = %d, want the three collapsed onto one key", len(kept))
	}
	if kept[0].Notes != "about ansible" {
		t.Errorf("kept %q, want the entry already written about that repository", kept[0].Notes)
	}
}

// TestRefilingIsStableWhenNothingMatches: with no entry written about the target
// repository, file order decides — so two runs produce the same file.
func TestRefilingIsStableWhenNothingMatches(t *testing.T) {
	entries := []Vulnerability{
		{CVE: "CVE-1", Repository: "rust", FirstSeen: "golangci/golangci-lint:v2.6.2", Notes: "first"},
		{CVE: "CVE-1", Repository: "rust", FirstSeen: "docker:29.0.4", Notes: "second"},
	}

	for i := 0; i < 5; i++ {
		kept := resolveRefiled(entries)
		if len(kept) != 1 || kept[0].Notes != "first" {
			t.Fatalf("run %d kept %v, want the first in file order", i, kept)
		}
	}
}

// TestPruneWritesNothingWhenThereIsNothingToApply: an --execute run that found
// nothing to remove and nothing to re-file must leave the file exactly as it
// was, byte for byte — rewriting it would produce a diff nobody asked for (#289).
func TestPruneWritesNothingWhenThereIsNothingToApply(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	if err := os.WriteFile(path, []byte("# untouched\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	entries := []prunedEntry{
		{Vulnerability{CVE: "CVE-2"}, presets.ExceptionVerdict{State: presets.ExceptionUnknown}},
	}
	if err := applyPrune(path, &VulnerabilityFile{}, entries); err != nil {
		t.Fatalf("applyPrune: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(content) != "# untouched\n" {
		t.Errorf("file was rewritten although there was nothing to apply: %q", content)
	}
}

// TestPruneReportNamesTheFindingsThatAreFixedUpstream: the policy says never to
// write an exception for a vulnerability that has a fix. An entry that turns out
// to have one has to be named, or the file goes on carrying a decision where a
// repin was the answer.
func TestPruneReportNamesTheFindingsThatAreFixedUpstream(t *testing.T) {
	entries := []prunedEntry{
		{
			Vulnerability{CVE: "CVE-2025-52881", FirstSeen: "docker:29.0.4"},
			presets.ExceptionVerdict{State: presets.ExceptionCarryOver, StillOn: "ghcr.io/ansible/dev-tools", FixedIn: "1.2.8"},
		},
	}

	out := captureStdout(t, func() {
		printPruneReport(entries, 1, []string{"ghcr.io/ansible/dev-tools"}, "scan-results", sightedEvidence())
	})

	for _, want := range []string{"FIXED UPSTREAM (1)", "fixed in 1.2.8", "never to write an exception"} {
		if !strings.Contains(out, want) {
			t.Errorf("report does not state %q:\n%s", want, out)
		}
	}
}

// TestPruneReportSaysWhenTheResultsKeptNoReceipt: with nothing recorded as
// suppressed, no entry can be shown obsolete — and a report that stayed silent
// about why would read as "nothing to purge", which is a different claim.
func TestPruneReportSaysWhenTheResultsKeptNoReceipt(t *testing.T) {
	entries := []prunedEntry{
		{
			Vulnerability{CVE: "CVE-2025-52881", Repository: "ghcr.io/ansible/dev-tools"},
			presets.ExceptionVerdict{State: presets.ExceptionUnknown, Reason: "nothing was recorded"},
		},
	}

	out := captureStdout(t, func() {
		printPruneReport(entries, 1, []string{"ghcr.io/ansible/dev-tools"}, "scan-results", *newIgnoreEvidence())
	})

	if !strings.Contains(out, "--show-suppressed") {
		t.Errorf("report does not say what evidence is missing or where to get it:\n%s", out)
	}
}

// TestPruneReportSaysWhatTheAuditStatedItFiltered is the other half, and the one
// the report was silent about: results that record no suppression because the
// ignore file was empty are results nothing was hidden from, and the report has
// to say so rather than print the caveat that fits the opposite case. Since #303
// that is the state of every catalogue repository (#327).
func TestPruneReportSaysWhatTheAuditStatedItFiltered(t *testing.T) {
	entries := []prunedEntry{
		{
			Vulnerability{CVE: "CVE-2025-52881", Repository: "ghcr.io/ansible/dev-tools"},
			presets.ExceptionVerdict{State: presets.ExceptionObsolete, Reason: "nothing carries it"},
		},
	}

	evidence := newIgnoreEvidence()
	evidence.Declared["ghcr.io/ansible/dev-tools"] = 0
	evidence.expired["ghcr.io/ansible/dev-tools"] = 4

	out := captureStdout(t, func() {
		printPruneReport(entries, 1, []string{"ghcr.io/ansible/dev-tools"}, "scan-results", *evidence)
	})

	for _, want := range []string{"empty ignore file for 1", "4 acceptance(s)", "an absence there is an absence"} {
		if !strings.Contains(out, want) {
			t.Errorf("report does not state %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--show-suppressed") {
		t.Errorf("report hedges results that state they filtered nothing:\n%s", out)
	}
}

// sightedEvidence is a directory whose scanners kept the record of what their
// ignore file removed — what security-audit.yml produces under
// `--show-suppressed`.
func sightedEvidence() ignoreEvidence {
	evidence := newIgnoreEvidence()
	evidence.Sighted = true
	return *evidence
}

// writeTrivyResultAs writes a Trivy report under an explicit file name, for the
// cases where the name itself is what is under test.
func writeTrivyResultAs(t *testing.T, dir, name string, findings map[string]string) {
	t.Helper()

	var vulns []any
	for id, severity := range findings {
		vulns = append(vulns, map[string]any{"VulnerabilityID": id, "Severity": severity})
	}

	writeJSON(t, filepath.Join(dir, name), map[string]any{
		"Results": []any{map[string]any{"Vulnerabilities": vulns}},
	})
}

// captureStdout runs fn with os.Stdout redirected, so a report that prints
// rather than returns can still be asserted on.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = write

	done := make(chan string)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := read.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	fn()
	_ = write.Close()
	os.Stdout = original
	return <-done
}
