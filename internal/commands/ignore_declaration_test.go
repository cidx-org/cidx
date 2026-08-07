package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// The three states a results directory can be in, and what each one lets a
// reader conclude (#327).
//
// Everything here runs offline, against files written into a temp directory:
// the writer is the real `cidx security vuln ignore` command, the readers are
// the real `catalogueFindings` and `carriedFindings`.

// declaringImage is one catalogue image, pinned the way presets.toml pins them.
const declaringImage = "rust:1.97.0@sha256:" + zeroDigest

// declaringCatalogue is the one-image catalogue the readers are pointed at.
func declaringCatalogue() map[string][]string {
	return map[string][]string{declaringImage: {"cargo-audit"}}
}

// TestTheIgnoreCommandStatesWhatItWrote runs the real command over a file
// holding one live acceptance and two lapsed ones, and reads back what it filed
// next to the results.
//
// It is the whole of the new information, and none of it is new work: the same
// two numbers already went to stderr for the run log, where no reader of the
// artifacts could ever see them.
func TestTheIgnoreCommandStatesWhatItWrote(t *testing.T) {
	dir := t.TempDir()
	vulnFile := filepath.Join(dir, "known-vulnerabilities.toml")
	if err := os.WriteFile(vulnFile, []byte(`
[[vulnerabilities]]
  cve = "CVE-2026-0001"
  repository = "rust"
  severity = "HIGH"
  expires = "2999-01-01"

[[vulnerabilities]]
  cve = "CVE-2026-0002"
  repository = "rust"
  severity = "HIGH"
  expires = "2020-01-01"

[[vulnerabilities]]
  cve = "CVE-2026-0003"
  repository = "rust"
  severity = "HIGH"
  expires = "2020-01-01"
`), 0o600); err != nil {
		t.Fatalf("failed to stage the exception file: %v", err)
	}

	results := filepath.Join(dir, "results")
	if err := NewApp().Run([]string{
		"cidx", "security", "vuln", "ignore",
		"--file", vulnFile, "--results", results, "-o", filepath.Join(dir, ".trivyignore"),
		declaringImage,
	}); err != nil {
		t.Fatalf("cidx security vuln ignore failed: %v", err)
	}

	declaration, stated := readIgnoreDeclaration(results, declaringImage)
	if !stated {
		listed, _ := os.ReadDir(results)
		t.Fatalf("nothing was declared for %s; %s holds %v", declaringImage, results, listed)
	}
	if declaration.Repository != "rust" {
		t.Errorf("repository = %q, want the key exceptions are filed under", declaration.Repository)
	}
	if declaration.Entries != 1 {
		t.Errorf("entries = %d, want the one acceptance still within its date", declaration.Entries)
	}
	if declaration.Expired != 2 {
		t.Errorf("expired = %d, want the two acceptances left out as past their date", declaration.Expired)
	}
}

// TestAnEmptyIgnoreFileMakesAnAbsenceReadable is the case #324's guard cannot
// see and the state the catalogue is actually in: every acceptance is past its
// date, so the file the audit builds is empty, so nothing is ever recorded as
// suppressed. An empty ignore file hid nothing, and the entry whose CVE the
// scan no longer reports has nothing left to waive.
func TestAnEmptyIgnoreFileMakesAnAbsenceReadable(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, declaringImage, map[string]string{"CVE-2026-9999": "HIGH"})
	declare(t, dir, ignoreDeclaration{Image: declaringImage, Repository: "rust", Entries: 0, Expired: 4})

	running, findings, suppressed, evidence := catalogueFindings(declaringCatalogue(), dir)
	if evidence.Sighted {
		t.Fatal("the staged results record a suppression: this test would pass on the old evidence alone")
	}

	verdict := presets.ClassifyException("CVE-2026-0001", "rust", running, findings, suppressed, evidence.SuppressionEvidence)
	if verdict.State != presets.ExceptionObsolete {
		t.Errorf("state = %q (%s), want %q: nothing was filtering, so the absence is an absence",
			verdict.State, verdict.Reason, presets.ExceptionObsolete)
	}
}

// TestADeclaredIgnoreFileWithEntriesStillFailsClosed: the declaration settles an
// absence only when it says nothing went into the file. Entries in it removed
// something, and what that was still has to be on the record.
func TestADeclaredIgnoreFileWithEntriesStillFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, declaringImage, map[string]string{"CVE-2026-9999": "HIGH"})
	declare(t, dir, ignoreDeclaration{Image: declaringImage, Repository: "rust", Entries: 3})

	running, findings, suppressed, evidence := catalogueFindings(declaringCatalogue(), dir)

	verdict := presets.ClassifyException("CVE-2026-0001", "rust", running, findings, suppressed, evidence.SuppressionEvidence)
	if verdict.State != presets.ExceptionUnknown {
		t.Errorf("state = %q (%s), want %q: three entries were filtering and nothing recorded what they removed",
			verdict.State, verdict.Reason, presets.ExceptionUnknown)
	}
}

// TestResultsWithNoDeclarationKeepTheConservativeReading is the compatibility
// case, and the one that keeps #324 intact: a directory produced before any of
// this — or assembled by hand — states nothing, and every absence in it stays
// unreadable. Nothing here is opt-out; the reader concludes only from what the
// results actually say.
func TestResultsWithNoDeclarationKeepTheConservativeReading(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, declaringImage, map[string]string{"CVE-2026-9999": "HIGH"})

	running, findings, suppressed, evidence := catalogueFindings(declaringCatalogue(), dir)
	if len(evidence.Declared) != 0 {
		t.Fatalf("declared = %v, want nothing stated", evidence.Declared)
	}

	verdict := presets.ClassifyException("CVE-2026-0001", "rust", running, findings, suppressed, evidence.SuppressionEvidence)
	if verdict.State != presets.ExceptionUnknown {
		t.Errorf("state = %q (%s), want %q: results saying nothing settle nothing",
			verdict.State, verdict.Reason, presets.ExceptionUnknown)
	}
}

// TestTheBaselineCountsATotalWhenNothingWasFiltered: the same evidence applied
// to a count rather than to a deletion. An accepted finding is missing from its
// own image's report only if the ignore file removed it — with an empty file
// there is nothing to add back, and the number is the total it claims to be.
func TestTheBaselineCountsATotalWhenNothingWasFiltered(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, declaringImage, map[string]string{"CVE-2026-9999": "HIGH"})
	accepted := []Vulnerability{{CVE: "CVE-2026-0001", Repository: "rust", Severity: "HIGH"}}

	if _, accounted := carriedFindings(declaringCatalogue(), dir, accepted); accounted {
		t.Error("results stating nothing are published as a total: the accepted findings cannot be counted back")
	}

	declare(t, dir, ignoreDeclaration{Image: declaringImage, Repository: "rust", Entries: 0, Expired: 1})
	if _, accounted := carriedFindings(declaringCatalogue(), dir, accepted); !accounted {
		t.Error("an empty ignore file removed nothing, so the count is a total and must not be hedged")
	}

	declare(t, dir, ignoreDeclaration{Image: declaringImage, Repository: "rust", Entries: 2})
	if _, accounted := carriedFindings(declaringCatalogue(), dir, accepted); accounted {
		t.Error("two entries were filtering and nothing recorded what they removed: the count is a floor")
	}
}

// TestTheBaselineIgnoresRepositoriesNothingIsAcceptedOn: only the repositories
// carrying an acceptance can have had anything subtracted from the count, so
// only those are asked. Hedging on a repository with an empty exception list
// would make every baseline a floor for ever.
func TestTheBaselineIgnoresRepositoriesNothingIsAcceptedOn(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, declaringImage, map[string]string{"CVE-2026-9999": "HIGH"})

	if _, accounted := carriedFindings(declaringCatalogue(), dir, nil); !accounted {
		t.Error("a catalogue with no acceptance at all is published as a floor")
	}
}

// TestTheAuditStatesWhatItFiltered pins the workflow end of the contract, the
// same way TestTheAuditRecordsWhatItSuppressed pins `--show-suppressed`. Both
// ignore-file steps have to say what they wrote, or the readers fall back to the
// conservative reading and nothing downstream notices the day the flag is
// dropped.
func TestTheAuditStatesWhatItFiltered(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "security-audit.yml"))
	if err != nil {
		t.Fatalf("could not read the audit workflow: %v", err)
	}

	for _, format := range []string{"trivy", "grype"} {
		if !strings.Contains(string(workflow), "--format "+format) {
			t.Fatalf("the audit no longer generates a %s ignore file: this test is looking at the wrong step", format)
		}
		if !strings.Contains(string(workflow), "--format "+format+" --results results") {
			t.Errorf("the %s ignore step drops --results: an empty ignore file would read as missing evidence and no exception could retire (#327)", format)
		}
	}
}

// declare files a declaration the way `cidx security vuln ignore --results`
// files it.
func declare(t *testing.T, dir string, declaration ignoreDeclaration) {
	t.Helper()

	if err := writeIgnoreDeclaration(dir, declaration); err != nil {
		t.Fatalf("failed to state what the ignore file held: %v", err)
	}
}
