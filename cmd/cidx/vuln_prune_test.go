package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// pruneCatalogue is a two-image catalogue pinned by digest, as presets.toml
// writes them (#242).
func pruneCatalogue() map[string][]string {
	return map[string][]string{
		"dhi.io/trivy:0.68@sha256:" + zeroDigest:                     {"trivy"},
		"golangci/golangci-lint:v2.12.2-alpine@sha256:" + zeroDigest: {"golangci-lint"},
	}
}

// TestCatalogueFindingsKeysOnTheDigestFreeReference: exceptions are recorded
// against `repo:tag`, so the findings have to be indexed the same way or every
// lookup misses and every entry reads as unknown.
func TestCatalogueFindingsKeysOnTheDigestFreeReference(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, "dhi.io/trivy:0.68@sha256:"+zeroDigest, map[string]string{"CVE-2026-0001": "HIGH"})

	running, findings := catalogueFindings(pruneCatalogue(), dir)

	if len(running) != 2 || running[0] != "dhi.io/trivy:0.68" {
		t.Fatalf("running = %v, want the digest-free references sorted", running)
	}
	if got := findings["dhi.io/trivy:0.68"]; len(got) != 1 || got[0] != "CVE-2026-0001" {
		t.Errorf("findings = %v, want the CVE indexed under the digest-free reference", findings)
	}
}

// TestCatalogueFindingsLeavesUnscannedImagesAbsent: absent is what makes the
// verdict fail-closed. An unscanned image recorded as "no finding" would make
// every exception look purgeable.
func TestCatalogueFindingsLeavesUnscannedImagesAbsent(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, "dhi.io/trivy:0.68@sha256:"+zeroDigest, nil)

	_, findings := catalogueFindings(pruneCatalogue(), dir)

	if _, scanned := findings["golangci/golangci-lint:v2.12.2-alpine"]; scanned {
		t.Errorf("an image with no result file was recorded as scanned: %v", findings)
	}
	if _, scanned := findings["dhi.io/trivy:0.68"]; !scanned {
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

	_, findings := catalogueFindings(pruneCatalogue(), dir)

	if got := findings["dhi.io/trivy:0.68"]; len(got) != 1 {
		t.Errorf("findings = %v, want the audit's result file to be read", findings)
	}
}

// TestPruneRemovesOnlyObsoleteEntries: the whole point. A carry-over entry
// describes a finding a catalogue image still has, and deleting it would lose
// the justification and turn the next audit red.
func TestPruneRemovesOnlyObsoleteEntries(t *testing.T) {
	entries := []prunedEntry{
		{Vulnerability{CVE: "CVE-1"}, presets.ExceptionVerdict{State: presets.ExceptionObsolete}},
		{Vulnerability{CVE: "CVE-2"}, presets.ExceptionVerdict{State: presets.ExceptionCarryOver}},
		{Vulnerability{CVE: "CVE-3"}, presets.ExceptionVerdict{State: presets.ExceptionUnknown}},
		{Vulnerability{CVE: "CVE-4"}, presets.ExceptionVerdict{State: presets.ExceptionLive}},
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

// TestPruneWritesNothingWhenNothingIsObsolete: an --execute run that found no
// obsolete entry must leave the file exactly as it was, byte for byte — rewriting
// it would produce a diff nobody asked for.
func TestPruneWritesNothingWhenNothingIsObsolete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	if err := os.WriteFile(path, []byte("# untouched\n"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	entries := []prunedEntry{
		{Vulnerability{CVE: "CVE-2"}, presets.ExceptionVerdict{State: presets.ExceptionCarryOver}},
	}
	if err := applyPrune(path, &VulnerabilityFile{}, entries); err != nil {
		t.Fatalf("applyPrune: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(content) != "# untouched\n" {
		t.Errorf("file was rewritten although nothing was obsolete: %q", content)
	}
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
