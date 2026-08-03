package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// candidateRef and runningRef are the two references every verdict weighs
// against each other: what the catalogue runs today, and what the cooldown has
// already cleared for promotion.
const (
	runningRef   = "tmknom/prettier:3.6.2@sha256:" + zeroDigest
	candidateRef = "tmknom/prettier:3.7.0@sha256:" + zeroDigest
)

// writeTrivyResult writes the scanner output the monitor's Trivy job uploads,
// under the name the promote job looks it up by. No Docker, no network — the
// file is the whole interface.
func writeTrivyResult(t *testing.T, dir, image string, findings map[string]string) {
	t.Helper()

	type vuln struct {
		VulnerabilityID string
		Severity        string
	}
	var vulns []vuln
	for id, severity := range findings {
		vulns = append(vulns, vuln{VulnerabilityID: id, Severity: severity})
	}

	report := map[string]any{
		"ArtifactName": image,
		"Results":      []any{map[string]any{"Vulnerabilities": vulns}},
	}
	writeJSON(t, filepath.Join(dir, scanResultFile("trivy", image)), report)
}

// writeGrypeResult does the same for Grype, whose document shape and severity
// spelling differ from Trivy's.
func writeGrypeResult(t *testing.T, dir, image string, findings map[string]string) {
	t.Helper()

	var matches []any
	for id, severity := range findings {
		matches = append(matches, map[string]any{
			"vulnerability": map[string]any{"id": id, "severity": severity},
		})
	}
	writeJSON(t, filepath.Join(dir, scanResultFile("grype", image)), map[string]any{"matches": matches})
}

func writeJSON(t *testing.T, path string, document any) {
	t.Helper()

	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("failed to encode the scanner result: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// promotableTarget is a candidate the cooldown has already cleared — the only
// kind the scan gate has anything to say about.
func promotableTarget() scanTarget {
	return scanTarget{
		CurrentImage:   runningRef,
		ScanImage:      candidateRef,
		CandidateImage: candidateRef,
		IsUpdate:       true,
		Presets:        []string{"prettier"},
		PolicyReason:   "published 20 days ago, past the 14-day cooldown",
	}
}

func onlyVerdict(t *testing.T, verdicts []promotionVerdict) promotionVerdict {
	t.Helper()
	if len(verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(verdicts))
	}
	return verdicts[0]
}

// TestPromotionVerdictClearsACleanCandidate: both scanners ran, neither found
// anything, the promotion goes ahead.
func TestPromotionVerdictClearsACleanCandidate(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, nil)
	writeGrypeResult(t, dir, candidateRef, nil)

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, nil))

	if !verdict.Promote {
		t.Fatalf("a clean candidate should be promoted, got: %s", verdict.Reason)
	}
	if verdict.NewImage != candidateRef {
		t.Errorf("NewImage = %q, want the pinned candidate %q", verdict.NewImage, candidateRef)
	}
	if verdict.PolicyReason == "" {
		t.Error("PolicyReason is empty: the promotion PR still has to state the cooldown verdict")
	}
}

// TestPromotionVerdictClearsAnInheritedFinding is the differential case: the
// candidate carries a vulnerability the running image already carries and that
// is on record, so the update is not a regression and must not be blocked
// (#247).
func TestPromotionVerdictClearsAnInheritedFinding(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0001": "HIGH"})
	writeGrypeResult(t, dir, candidateRef, map[string]string{"CVE-2026-0001": "High"})

	accepted := map[string][]string{"tmknom/prettier": {"CVE-2026-0001"}}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, accepted))

	if !verdict.Promote {
		t.Fatalf("a finding inherited from the running image must not block, got: %s", verdict.Reason)
	}
	if len(verdict.Introduces) != 0 {
		t.Errorf("Introduces = %v, want none", verdict.Introduces)
	}
}

// TestPromotionVerdictHoldsANewFinding: what the gate is for. The candidate is
// otherwise legitimate — past the cooldown, pinned by digest — and is still held.
func TestPromotionVerdictHoldsANewFinding(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{
		"CVE-2026-0001": "HIGH",
		"CVE-2026-0002": "CRITICAL",
	})
	writeGrypeResult(t, dir, candidateRef, nil)

	accepted := map[string][]string{"tmknom/prettier": {"CVE-2026-0001"}}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, accepted))

	if verdict.Promote {
		t.Fatalf("a candidate introducing CVE-2026-0002 must be held, got: %s", verdict.Reason)
	}
	if len(verdict.Introduces) != 1 || verdict.Introduces[0] != "CVE-2026-0002" {
		t.Errorf("Introduces = %v, want CVE-2026-0002 alone", verdict.Introduces)
	}
	if !strings.Contains(verdict.Reason, "CVE-2026-0002") {
		t.Errorf("Reason = %q, want it to name the finding that held the promotion", verdict.Reason)
	}
}

// TestPromotionVerdictClearsAFindingAcceptedOnTheRepository: the candidate is a
// newer tag of the repository the catalogue already runs, so the exception
// written for that repository covers it without being re-filed first. Under the
// old `repo:tag` key it did not, and the gate held promotions on findings that
// had been reviewed months earlier (#238).
func TestPromotionVerdictClearsAFindingAcceptedOnTheRepository(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0002": "HIGH"})

	accepted := map[string][]string{"tmknom/prettier": {"CVE-2026-0002"}}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, accepted))

	if !verdict.Promote {
		t.Fatalf("an accepted finding must not hold the promotion, got: %s", verdict.Reason)
	}
}

// TestPromotionVerdictIgnoresLowSeverityFindings: Grype reports every severity
// whatever --fail-on says, and the policy acts on HIGH/CRITICAL only.
func TestPromotionVerdictIgnoresLowSeverityFindings(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0003": "MEDIUM"})
	writeGrypeResult(t, dir, candidateRef, map[string]string{"CVE-2026-0004": "Negligible"})

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, nil))

	if !verdict.Promote {
		t.Fatalf("only HIGH/CRITICAL findings gate a promotion, got: %s", verdict.Reason)
	}
}

// TestPromotionVerdictReadsBothScanners: a finding only Grype knows about still
// holds the candidate, or running two scanners buys nothing.
func TestPromotionVerdictReadsBothScanners(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, nil)
	writeGrypeResult(t, dir, candidateRef, map[string]string{"GHSA-cgrx-mc8f-2prm": "Critical"})

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, nil))

	if verdict.Promote {
		t.Fatal("a finding reported by Grype alone must hold the candidate")
	}
	if len(verdict.Introduces) != 1 || verdict.Introduces[0] != "GHSA-cgrx-mc8f-2prm" {
		t.Errorf("Introduces = %v, want the Grype finding", verdict.Introduces)
	}
}

// TestPromotionVerdictNamesTheScannersItRead: the promotion PR states that both
// scanners looked, so a verdict reached on one of them has to say so. The very
// first end-to-end run of this gate lost a Trivy job to a flaky registry login,
// which is exactly the case that would otherwise be claimed as a clean sweep.
func TestPromotionVerdictNamesTheScannersItRead(t *testing.T) {
	both := t.TempDir()
	writeTrivyResult(t, both, candidateRef, nil)
	writeGrypeResult(t, both, candidateRef, nil)

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, both, nil))
	if !strings.Contains(verdict.Reason, "scanned by Trivy and Grype") {
		t.Errorf("Reason = %q, want both scanners named", verdict.Reason)
	}
	if len(verdict.ScannedBy) != 2 {
		t.Errorf("ScannedBy = %v, want both scanners", verdict.ScannedBy)
	}

	grypeOnly := t.TempDir()
	writeGrypeResult(t, grypeOnly, candidateRef, nil)

	verdict = onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, grypeOnly, nil))
	if !verdict.Promote {
		t.Fatalf("one scanner's evidence still promotes, got: %s", verdict.Reason)
	}
	if strings.Contains(verdict.Reason, "Trivy") {
		t.Errorf("Reason = %q, want it not to claim a Trivy scan that never ran", verdict.Reason)
	}
	if len(verdict.ScannedBy) != 1 || verdict.ScannedBy[0] != "Grype" {
		t.Errorf("ScannedBy = %v, want Grype alone", verdict.ScannedBy)
	}
}

// TestPromotionVerdictHoldsWithoutScanResults is the fail-closed case: no
// evidence, no promotion. It is the state a failed pull or a skipped scan job
// leaves behind, and before #247 it promoted regardless.
func TestPromotionVerdictHoldsWithoutScanResults(t *testing.T) {
	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, t.TempDir(), nil))

	if verdict.Promote {
		t.Fatal("a candidate nobody scanned must not be promoted")
	}
	if !strings.Contains(verdict.Reason, "no scanner result") {
		t.Errorf("Reason = %q, want it to say no result was produced", verdict.Reason)
	}
}

// TestPromotionVerdictHoldsOnUnreadableResults: an empty file is what a scanner
// that failed to pull the image leaves behind, and it must not read as a clean
// scan.
func TestPromotionVerdictHoldsOnUnreadableResults(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, scanResultFile("trivy", candidateRef)), nil, 0o600); err != nil {
		t.Fatalf("failed to write the empty result: %v", err)
	}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{promotableTarget()}, dir, nil))

	if verdict.Promote {
		t.Fatal("an unreadable scanner result must not be taken for a clean image")
	}
	if !strings.Contains(verdict.Reason, "could not be read") {
		t.Errorf("Reason = %q, want it to report the unreadable result", verdict.Reason)
	}
}

// TestPromotionVerdictSkipsWhatTheCooldownHeld pins the order of the two gates:
// the cooldown runs first, in `cidx preset scan-targets`, and a candidate it
// held is not scanned as a candidate at all. The scan gate has nothing to judge
// and must not report it a second time.
func TestPromotionVerdictSkipsWhatTheCooldownHeld(t *testing.T) {
	held := scanTarget{
		CurrentImage:   runningRef,
		ScanImage:      runningRef, // the cooldown left the running image in place
		CandidateImage: candidateRef,
		IsUpdate:       false,
		Presets:        []string{"prettier"},
		PolicyReason:   "held: published 3 days ago, 11 days of the 14-day cooldown left",
	}

	if verdicts := buildPromotionVerdicts([]scanTarget{held}, t.TempDir(), nil); len(verdicts) != 0 {
		t.Errorf("got %d verdicts, want none: the cooldown already held this candidate", len(verdicts))
	}
}

// TestPromotionVerdictDoesNotLetAWaiverExcuseANewFinding: the two gates are
// cumulative. Rule 3 waives the *cooldown* when the running image is knowingly
// vulnerable (#242); it says nothing about what the candidate itself brings, and
// a candidate promoted for fixing one CVE must not smuggle in another.
func TestPromotionVerdictDoesNotLetAWaiverExcuseANewFinding(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0009": "CRITICAL"})

	waived := promotableTarget()
	waived.PolicyReason = "14-day cooldown waived: the running image is affected by CVE-2026-0001"
	waived.CVEWaiver = []string{"CVE-2026-0001"}

	accepted := map[string][]string{"tmknom/prettier": {"CVE-2026-0001"}}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{waived}, dir, accepted))

	if verdict.Promote {
		t.Fatalf("a cooldown waiver must not excuse a new finding, got: %s", verdict.Reason)
	}
	if len(verdict.CVEWaiver) != 1 {
		t.Errorf("CVEWaiver = %v, want the cooldown waiver carried through for the report", verdict.CVEWaiver)
	}
}

// TestPromotionVerdictClearsAWaivedCandidateThatFixesTheCVE is the same
// interaction the other way round, and the case rule 3 exists for: the candidate
// skipped the cooldown because we are knowingly vulnerable, and it brings
// nothing new, so it ships.
func TestPromotionVerdictClearsAWaivedCandidateThatFixesTheCVE(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, nil)
	writeGrypeResult(t, dir, candidateRef, nil)

	waived := promotableTarget()
	waived.PolicyReason = "14-day cooldown waived: the running image is affected by CVE-2026-0001"
	waived.CVEWaiver = []string{"CVE-2026-0001"}

	accepted := map[string][]string{"tmknom/prettier": {"CVE-2026-0001"}}

	verdict := onlyVerdict(t, buildPromotionVerdicts([]scanTarget{waived}, dir, accepted))

	if !verdict.Promote {
		t.Fatalf("the fix we waived the cooldown for should be promoted, got: %s", verdict.Reason)
	}
}

// TestScanResultFileMatchesTheWorkflowConvention pins the one coupling between
// the scan jobs and the promote job: `tr '/:@' '___'` in container-monitor.yml
// has to produce the name looked up here.
func TestScanResultFileMatchesTheWorkflowConvention(t *testing.T) {
	got := scanResultFile("trivy", "dhi.io/golang:1.24-alpine3.21@sha256:"+zeroDigest)
	want := "trivy-dhi.io_golang_1.24-alpine3.21_sha256_" + zeroDigest + ".json"

	if got != want {
		t.Errorf("scanResultFile = %q, want %q", got, want)
	}
}

// grypeWithAnIgnoredMatch is the document Grype writes for an image with an
// exception on file, reduced to the fields that are read. The audit passes it
// the ignore file `cidx security vuln ignore` generates, and the suppressed
// match is *moved* to `ignoredMatches` rather than dropped — fix included, which
// is what makes #312 answerable at all.
const grypeWithAnIgnoredMatch = `{
  "matches": [
    {"vulnerability": {"id": "CVE-2026-0002", "severity": "High"},
     "artifact": {"name": "libxml2", "type": "deb"}}
  ],
  "ignoredMatches": [
    {"vulnerability": {"id": "GHSA-cgrx-mc8f-2prm", "severity": "High",
                       "fix": {"versions": ["1.2.8"]}},
     "artifact": {"name": "github.com/opencontainers/runc", "type": "go-module"},
     "appliedIgnoreRules": [{"vulnerability": "GHSA-cgrx-mc8f-2prm"}]}
  ]
}`

// TestGrypeFindingsIgnoreWhatTheIgnoreFileSuppressed: the scan gate and the
// baseline count what the scanners actually report, and an accepted finding is
// deliberately not one of them. Reading `ignoredMatches` here would put every
// exception back into the numbers it was written to take out of them.
func TestGrypeFindingsIgnoreWhatTheIgnoreFileSuppressed(t *testing.T) {
	found, err := grypeFindings([]byte(grypeWithAnIgnoredMatch))
	if err != nil {
		t.Fatalf("grypeFindings: %v", err)
	}
	if len(found) != 1 || found[0].ID != "CVE-2026-0002" {
		t.Errorf("findings = %+v, want the reported match alone", found)
	}
}

// TestGrypeSuppressedFindingsReadTheIgnoredMatches: the other half. This is the
// only evidence there is about a live entry, because the ignore file that hid
// the finding was built from that entry.
func TestGrypeSuppressedFindingsReadTheIgnoredMatches(t *testing.T) {
	found, err := grypeSuppressedFindings([]byte(grypeWithAnIgnoredMatch))
	if err != nil {
		t.Fatalf("grypeSuppressedFindings: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("suppressed = %+v, want the ignored match alone", found)
	}
	if found[0].ID != "GHSA-cgrx-mc8f-2prm" || found[0].FixedIn != "1.2.8" {
		t.Errorf("suppressed = %+v, want the identifier and the fix Grype recorded", found[0])
	}
}

// TestSuppressedFindingsAreSilentWithoutARecordedSuppression: a report showing
// nothing suppressed says nothing was suppressed as far as this can tell. It is
// not "no fix" and not "the CVE is gone" — what settles the latter is whether
// the image produced a scan result at all, which `scanFindings` answers.
func TestSuppressedFindingsAreSilentWithoutARecordedSuppression(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0002": "HIGH"})

	if found := suppressedFindings(dir, candidateRef); len(found) != 0 {
		t.Errorf("suppressed = %+v, want nothing: the report records no suppressed finding", found)
	}
}

// trivyWithASuppressedFinding is what `trivy image --show-suppressed` writes for
// an image with an exception on file, reduced to the fields that are read.
//
// Measured against Trivy 0.72.0: the suppressed finding is *moved* to
// `ExperimentalModifiedFindings` with `Status: "ignored"`, the source of the
// rule, and the whole finding underneath — `FixedVersion` included. `--severity`
// still applies to it, so the band matches the visible findings. A CVE in the
// ignore file that the image does not carry produces no entry at all, which is
// exactly the signal #311 needed.
const trivyWithASuppressedFinding = `{
  "ArtifactName": "example.io/tool:1.0",
  "Results": [
    {"Type": "alpine",
     "Vulnerabilities": [
       {"VulnerabilityID": "CVE-2026-0002", "PkgName": "libxml2", "Severity": "HIGH"}
     ],
     "ExperimentalModifiedFindings": [
       {"Status": "ignored", "Source": "/root/.trivyignore",
        "Finding": {"VulnerabilityID": "CVE-2026-0001", "PkgName": "nghttp2-libs",
                    "Severity": "HIGH", "FixedVersion": "1.68.1"}},
       {"Status": "ignored", "Source": "/root/.trivyignore",
        "Finding": {"VulnerabilityID": "CVE-2026-0003", "PkgName": "busybox",
                    "Severity": "MEDIUM"}},
       {"Status": "not_affected", "Source": "vex.json",
        "Finding": {"VulnerabilityID": "CVE-2026-0004", "PkgName": "openssl",
                    "Severity": "HIGH"}}
     ]}
  ]
}`

// TestTrivyFindingsIgnoreWhatWasSuppressed: the scan gate and the baseline count
// what the report shows, and an accepted finding is deliberately not one of
// them. Reading the suppressed list here would put every exception back into the
// numbers it was written to take out of.
func TestTrivyFindingsIgnoreWhatWasSuppressed(t *testing.T) {
	found, err := trivyFindings([]byte(trivyWithASuppressedFinding))
	if err != nil {
		t.Fatalf("trivyFindings: %v", err)
	}
	if len(found) != 1 || found[0].ID != "CVE-2026-0002" {
		t.Errorf("findings = %+v, want the reported vulnerability alone", found)
	}
}

// TestTrivySuppressedFindingsReadTheModifiedFindings is the half #311 adds. The
// ignore file is generated from the entries themselves, so this is where an
// accepted CVE has to be looked for before it can be called gone.
func TestTrivySuppressedFindingsReadTheModifiedFindings(t *testing.T) {
	found, err := trivySuppressedFindings([]byte(trivyWithASuppressedFinding))
	if err != nil {
		t.Fatalf("trivySuppressedFindings: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("suppressed = %+v, want the ignored HIGH finding alone", found)
	}
	if found[0].ID != "CVE-2026-0001" || found[0].FixedIn != "1.68.1" {
		t.Errorf("suppressed = %+v, want the identifier and the fix Trivy recorded", found[0])
	}
	if found[0].PackageType != "alpine" {
		t.Errorf("PackageType = %q, want the ecosystem of the enclosing result", found[0].PackageType)
	}
}

// TestTrivySuppressedFindingsIgnoreVEXStatuses: `ExperimentalModifiedFindings`
// also carries Trivy's VEX statuses, which say a finding was assessed rather
// than waived by our ignore file. Reading `not_affected` as "the image carries
// this" would keep an entry alive on somebody else's assessment.
func TestTrivySuppressedFindingsIgnoreVEXStatuses(t *testing.T) {
	found, err := trivySuppressedFindings([]byte(trivyWithASuppressedFinding))
	if err != nil {
		t.Fatalf("trivySuppressedFindings: %v", err)
	}
	for _, f := range found {
		if f.ID == "CVE-2026-0004" {
			t.Errorf("suppressed = %+v, want only the findings the ignore file removed", found)
		}
	}
}

// TestSuppressedFindingsReadBothScanners: the union is what an image is known to
// carry, and either scanner may be the only one that saw a given CVE. Two of the
// six ansible entries are Trivy-only, and they were invisible here until the
// audit started passing `--show-suppressed` (#311, #315).
func TestSuppressedFindingsReadBothScanners(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, scanResultFile("trivy", candidateRef)), json.RawMessage(trivyWithASuppressedFinding))
	writeJSON(t, filepath.Join(dir, scanResultFile("grype", candidateRef)), json.RawMessage(grypeWithAnIgnoredMatch))

	found := suppressedFindings(dir, candidateRef)

	ids := make(map[string]bool, len(found))
	for _, f := range found {
		ids[f.ID] = true
	}
	if !ids["CVE-2026-0001"] || !ids["GHSA-cgrx-mc8f-2prm"] {
		t.Errorf("suppressed = %+v, want what both scanners recorded", found)
	}
}

// TestTheAuditRecordsWhatItSuppressed pins the other end of the evidence
// contract. `ClassifyException` calls an accepted CVE gone when neither the
// visible findings nor the suppressed record mention it, and Trivy only keeps
// that record under `--show-suppressed`. Its report carries no trace of whether
// the flag was passed — `ExperimentalModifiedFindings` is simply omitted when a
// result suppressed nothing — so nothing downstream can notice the day this
// flag is dropped from security-audit.yml. This test is what notices.
func TestTheAuditRecordsWhatItSuppressed(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "security-audit.yml"))
	if err != nil {
		t.Fatalf("could not read the audit workflow: %v", err)
	}

	scan, _, found := strings.Cut(string(workflow), "results/trivy-${SAFE_NAME}.json")
	if !found {
		t.Fatal("the audit no longer writes results/trivy-${SAFE_NAME}.json: this test is looking at the wrong step")
	}
	if !strings.Contains(scan, "--show-suppressed") {
		t.Error("the audit's JSON scan drops --show-suppressed: an accepted CVE would read as gone and `vuln prune -x` would delete the exception waiving it (#311)")
	}
}

// TestPromotionVerdictJSONContract pins the field names container-monitor.yml
// reads with jq. Renaming one here breaks the workflow, not the build.
func TestPromotionVerdictJSONContract(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, candidateRef, map[string]string{"CVE-2026-0002": "HIGH"})

	waived := promotableTarget()
	waived.CVEWaiver = []string{"CVE-2026-0001"}

	verdicts := buildPromotionVerdicts([]scanTarget{waived}, dir, nil)

	encoded, err := json.Marshal(verdicts[0])
	if err != nil {
		t.Fatalf("failed to encode the verdict: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to decode the verdict: %v", err)
	}

	for _, field := range []string{
		"current_image", "new_image", "presets", "promote", "reason",
		"introduces", "scanned_by", "policy_reason", "cve_waiver",
	} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("field %q missing from scan-verdicts JSON: container-monitor.yml reads it", field)
		}
	}

	if got := decoded["promote"]; got != false {
		t.Errorf("promote = %v, want false", got)
	}
}

// TestReadScanTargetsRoundTripsTheWorkflowHandover: the promote job pipes the
// scan-targets output straight in, so the two commands have to agree on the
// document.
func TestReadScanTargetsRoundTripsTheWorkflowHandover(t *testing.T) {
	now := scanNow(t)
	stubRegistry(t, "3.7.0", now.AddDate(0, 0, -20), nil)

	targets := buildScanTargets(map[string][]string{runningRef: {"prettier"}}, nil, now)
	encoded, err := json.Marshal(targets)
	if err != nil {
		t.Fatalf("failed to encode the targets: %v", err)
	}

	path := filepath.Join(t.TempDir(), "targets.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("failed to write the targets: %v", err)
	}

	got, err := readScanTargets(path)
	if err != nil {
		t.Fatalf("readScanTargets: %v", err)
	}
	if len(got) != 1 || !got[0].IsUpdate || got[0].ScanImage != candidateRef {
		t.Errorf("readScanTargets = %+v, want the promotable candidate back", got)
	}
}
