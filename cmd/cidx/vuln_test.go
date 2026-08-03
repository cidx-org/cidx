package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// TestStaleVulnerabilities: an exception recorded against a repository the
// catalogue no longer runs waives nothing, and until #248 nothing ever said so.
// Re-keying by repository (#238) removed the bulk of this — a promotion inside
// the same repository no longer orphans anything — but a repository genuinely
// replaced still leaves entries behind.
func TestStaleVulnerabilities(t *testing.T) {
	running := map[string]bool{
		"golangci/golangci-lint": true,
		"tmknom/prettier":        true,
	}

	entries := []Vulnerability{
		{CVE: "CVE-2025-0001", Repository: "aquasec/trivy"},
		{CVE: "CVE-2025-0002", Repository: "golangci/golangci-lint"},
		{CVE: "CVE-2025-0003", Repository: "tmknom/prettier"},
		{CVE: "CVE-2025-0004", Repository: "docker"},
	}

	stale := staleVulnerabilities(entries, running)

	want := []string{"CVE-2025-0001", "CVE-2025-0004"}
	if len(stale) != len(want) {
		t.Fatalf("stale entries = %v, want %v", stale, want)
	}
	for i, cve := range want {
		if stale[i].CVE != cve {
			t.Errorf("stale[%d].CVE = %q, want %q", i, stale[i].CVE, cve)
		}
	}
}

// TestStaleVulnerabilitiesIgnoresTheTag: the reason the key is the repository.
// The catalogue promoting golangci-lint from v2.6.2 to v2.12.2 changes nothing
// the judgement rested on, and under the old key every entry for it went stale
// on the day of the promotion.
func TestStaleVulnerabilitiesIgnoresTheTag(t *testing.T) {
	running := map[string]bool{"tmknom/prettier": true}

	entries := []Vulnerability{{CVE: "CVE-2025-0003", Repository: "tmknom/prettier"}}

	if stale := staleVulnerabilities(entries, running); len(stale) != 0 {
		t.Errorf("stale entries = %v, want none: neither the tag nor the digest is part of the key", stale)
	}
}

// TestImageRepositoryStripsTagAndDigest: the key exceptions are recorded under,
// derived from whatever form the scanners and the workflows paste back.
func TestImageRepositoryStripsTagAndDigest(t *testing.T) {
	cases := map[string]string{
		"rust:1.97.0-slim@sha256:" + zeroDigest:               "rust",
		"rust:1.97.0":                                         "rust",
		"dhi.io/trivy:0.71":                                   "dhi.io/trivy",
		"ghcr.io/ansible/dev-tools:v26.7.1":                   "ghcr.io/ansible/dev-tools",
		"registry:5000/team/tool":                             "registry:5000/team/tool",
		"registry:5000/team/tool:1.0":                         "registry:5000/team/tool",
		"gcr.io/kaniko-project/executor@sha256:" + zeroDigest: "gcr.io/kaniko-project/executor",
	}

	for ref, want := range cases {
		if got := imageRepository(ref); got != want {
			t.Errorf("imageRepository(%q) = %q, want %q", ref, got, want)
		}
	}
}

// TestLoadMigratesTheOldImageKey: the pre-#297 file keys entries by `repo:tag`.
// The reference becomes context, and the repository is deliberately left empty —
// deriving `golangci/golangci-lint` from `golangci/golangci-lint:v2.6.2` would
// file twelve kernel CVEs against an image that stopped carrying them the day it
// moved to Alpine. Which repository carries the CVE is a question the scan
// results answer, and `vuln prune` is what asks it.
func TestLoadMigratesTheOldImageKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	seed := `[[vulnerabilities]]
  cve = "CVE-2013-7445"
  image = "golangci/golangci-lint:v2.6.2@sha256:` + zeroDigest + `"
  severity = "HIGH"
`
	if err := os.WriteFile(path, []byte(seed), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	vulns, err := loadVulnerabilities(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	got := vulns.Vulnerabilities[0]
	if got.FirstSeen != "golangci/golangci-lint:v2.6.2" {
		t.Errorf("FirstSeen = %q, want the digest-free reference it was recorded against", got.FirstSeen)
	}
	if got.Repository != "" {
		t.Errorf("Repository = %q, want it left for the findings to decide", got.Repository)
	}
	if got.Image != "" {
		t.Errorf("Image = %q, want the legacy key cleared once read", got.Image)
	}
	if key := got.key(); key != "golangci/golangci-lint:v2.6.2" {
		t.Errorf("key() = %q, want the whole reference, which matches no repository", key)
	}
}

// TestSaveIsPurelySubtractive: removing an entry has to produce a diff that
// reads as a removal. The TOML encoder re-indented every kept key, so a purge of
// 101 entries showed up as 538 insertions and 1552 deletions (#289).
func TestSaveIsPurelySubtractive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	all := &VulnerabilityFile{Vulnerabilities: []Vulnerability{
		{CVE: "CVE-1", Repository: "rust", Severity: "HIGH", Status: "third-party", Added: "2026-01-01", Expires: "2026-04-01", Notes: "a"},
		{CVE: "CVE-2", Repository: "rust", Severity: "HIGH", Status: "third-party", Added: "2026-01-01", Expires: "2026-04-01", Notes: "b"},
	}}
	if err := saveVulnerabilities(path, all); err != nil {
		t.Fatalf("save: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	kept := &VulnerabilityFile{Vulnerabilities: []Vulnerability{all.Vulnerabilities[0]}}
	if err := saveVulnerabilities(path, kept); err != nil {
		t.Fatalf("save again: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read again: %v", err)
	}

	if !strings.HasPrefix(string(before), string(after)) {
		t.Errorf("removing the last entry rewrote the lines before it:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestSaveIsDeterministic: two saves of the same content produce the same bytes,
// so a re-run leaves no diff at all.
func TestSaveIsDeterministic(t *testing.T) {
	dir := t.TempDir()
	entries := []Vulnerability{
		{CVE: "CVE-2", Repository: "rust", FirstSeen: "golangci/golangci-lint:v2.6.2", Severity: "HIGH", References: []string{"https://example.test/1"}},
		{CVE: "CVE-1", Repository: "dhi.io/trivy", Severity: "CRITICAL"},
	}

	var previous string
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "run.toml")
		if err := saveVulnerabilities(path, &VulnerabilityFile{Vulnerabilities: entries}); err != nil {
			t.Fatalf("save: %v", err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if i > 0 && string(content) != previous {
			t.Fatalf("run %d differs:\n%s\n---\n%s", i, previous, content)
		}
		previous = string(content)
	}

	if !strings.Contains(previous, `repository = "dhi.io/trivy"`) {
		t.Errorf("the repository key is not written:\n%s", previous)
	}
	if strings.Contains(previous, "image =") {
		t.Errorf("the legacy image key was written back:\n%s", previous)
	}
	if strings.Index(previous, "CVE-1") > strings.Index(previous, "CVE-2") {
		t.Errorf("entries are not sorted by repository:\n%s", previous)
	}
}

// TestSaveRoundTrips: whatever the writer emits, the decoder has to read back
// unchanged — it is a hand-rolled encoder, and that is the whole risk it carries.
func TestSaveRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	original := Vulnerability{
		CVE: "CVE-1", Aliases: []string{"GHSA-xxxx"}, Repository: "rust",
		FirstSeen: "golangci/golangci-lint:v2.6.2", Severity: "HIGH", Status: "third-party",
		Added: "2026-01-01", Expires: "2026-04-01",
		Notes:      `a "quoted" note with a \ backslash`,
		References: []string{"https://example.test/1", "https://example.test/2"},
	}

	if err := saveVulnerabilities(path, &VulnerabilityFile{Vulnerabilities: []Vulnerability{original}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	reloaded, err := loadVulnerabilities(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(reloaded.Vulnerabilities) != 1 {
		t.Fatalf("entries = %d, want 1", len(reloaded.Vulnerabilities))
	}
	got := reloaded.Vulnerabilities[0]
	if got.Notes != original.Notes {
		t.Errorf("Notes = %q, want %q", got.Notes, original.Notes)
	}
	if len(got.References) != 2 || len(got.Aliases) != 1 {
		t.Errorf("lists did not round-trip: %+v", got)
	}
}

// TestRefuseIfFixedUpstream: the policy says never to write an exception for a
// vulnerability the publisher has already fixed — it would record a decision
// where there is only a wait, and waive the finding for the ninety days nobody
// spends bumping the image. There is no --force: the correct action is a repin.
func TestRefuseIfFixedUpstream(t *testing.T) {
	dir := t.TempDir()
	image := "ghcr.io/ansible/dev-tools:v26.7.1@sha256:" + zeroDigest
	writeTrivyResultAs(t, dir, "trivy-"+imageFileName.Replace(image)+".json", nil)
	writeJSON(t, filepath.Join(dir, "grype-"+imageFileName.Replace(image)+".json"), map[string]any{
		"matches": []any{map[string]any{
			"vulnerability": map[string]any{
				"id": "CVE-2025-52881", "severity": "High",
				"fix": map[string]any{"versions": []any{"1.2.8"}},
			},
			"artifact": map[string]any{"name": "github.com/opencontainers/runc", "type": "go-module"},
		}},
	})

	err := refuseIfFixedUpstream("CVE-2025-52881", image, dir)
	if err == nil {
		t.Fatal("an exception was accepted for a CVE that is fixed upstream")
	}
	if !strings.Contains(err.Error(), "1.2.8") {
		t.Errorf("the refusal does not name the fix: %v", err)
	}

	if err := refuseIfFixedUpstream("CVE-2026-9999", image, dir); err != nil {
		t.Errorf("an exception for a CVE with no fix was refused: %v", err)
	}
}

// TestRefuseIfFixedUpstreamCannotCheckWithoutResults: refusing on missing
// evidence would make the command unusable wherever the audit's artifacts are
// not to hand. Nothing is being promoted or deleted here, and the entry is
// argued again at its expiry.
func TestRefuseIfFixedUpstreamCannotCheckWithoutResults(t *testing.T) {
	if err := refuseIfFixedUpstream("CVE-2025-52881", "rust:1.97.0", t.TempDir()); err != nil {
		t.Errorf("the exception was refused although nothing was known: %v", err)
	}
}

// TestVulnerabilityJSONContract pins the field names security-audit.yml reads
// with jq out of `cidx security vuln check --json`. Renaming one here breaks the
// workflow summary, not the build — and the legacy `image` key must not come
// back through the JSON, since it is cleared on load and always empty.
func TestVulnerabilityJSONContract(t *testing.T) {
	encoded, err := json.Marshal(Vulnerability{
		CVE: "CVE-2026-0001", Repository: "rust", Severity: "HIGH", Expires: "2026-09-01",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	for _, field := range []string{"CVE", "Repository", "Severity", "Expires"} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("field %q missing: security-audit.yml reads it", field)
		}
	}
	if _, ok := decoded["Image"]; ok {
		t.Errorf("the legacy image key is still serialised, and it is always empty")
	}
}

// The expiry date decides what the ignore file carries (#303)
//
// `generateTrivyIgnore` emitted every entry's CVE with no date check at all, so
// the eighteen acceptances that lapsed on 2026-03-02 went on removing their
// findings from the audit's own scan results — the JSON is written after the
// filter, so nothing downstream could see them. The `expires` field was
// decorative, and it was the mechanism meant to force the review.

// TestTheIgnoreFileDropsAnExpiredAcceptance runs the two generators over the
// entries the command actually hands them, which is the whole of the change:
// a live acceptance still suppresses its finding, a lapsed one suppresses
// nothing.
func TestTheIgnoreFileDropsAnExpiredAcceptance(t *testing.T) {
	today := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

	entries := []Vulnerability{
		{CVE: "CVE-2026-0001", Repository: "rust", Expires: "2026-12-01"},
		{CVE: "CVE-2026-0002", Repository: "rust", Expires: "2026-03-02"},
	}

	live := waiving(entries, today)
	if len(live) != 1 || live[0].CVE != "CVE-2026-0001" {
		t.Fatalf("the entries still waiving something are %v, want CVE-2026-0001 alone", live)
	}

	trivy := generateTrivyIgnore(live)
	if !strings.Contains(trivy, "CVE-2026-0001") {
		t.Error("the live acceptance is missing from the trivy ignore file")
	}
	if strings.Contains(trivy, "CVE-2026-0002") {
		t.Error("the expired acceptance is still filtering its finding out of the trivy scan")
	}

	grype := generateGrypeIgnore(live)
	if !strings.Contains(grype, "CVE-2026-0001") {
		t.Error("the live acceptance is missing from the grype ignore file")
	}
	if strings.Contains(grype, "CVE-2026-0002") {
		t.Error("the expired acceptance is still filtering its finding out of the grype scan")
	}
}

// TestTheExpiryDayIsIncluded pins the boundary, which is the one thing a date
// check can get wrong quietly: "expires on 2026-03-02" is a deadline, not a
// cut-off, so the entry waives its finding all through that day and stops the
// next morning.
func TestTheExpiryDayIsIncluded(t *testing.T) {
	entry := []Vulnerability{{CVE: "CVE-2026-0001", Repository: "rust", Expires: "2026-03-02"}}

	cases := map[string]bool{
		"2026-03-01": true,  // the day before
		"2026-03-02": true,  // the day named — still covered
		"2026-03-03": false, // the day after — lapsed
	}

	for date, want := range cases {
		day, err := time.Parse(time.DateOnly, date)
		if err != nil {
			t.Fatalf("parse %q: %v", date, err)
		}
		if got := len(waiving(entry, day)) == 1; got != want {
			t.Errorf("on %s the acceptance waives = %v, want %v", date, got, want)
		}
	}
}

// TestTheExpiryDayIsReadInUTC: a scan runs on a runner in whatever zone GitHub
// gives it, and an acceptance is dated to the day. An instant late on the last
// day, read in a zone ahead of UTC, must not retire the entry early.
func TestTheExpiryDayIsReadInUTC(t *testing.T) {
	entry := []Vulnerability{{CVE: "CVE-2026-0001", Repository: "rust", Expires: "2026-03-02"}}

	late := time.Date(2026, 3, 2, 23, 30, 0, 0, time.FixedZone("UTC+11", 11*3600))
	if len(waiving(entry, late)) != 1 {
		t.Error("the acceptance stopped waiving before its day was over in UTC")
	}
}

// TestAnAcceptanceWithNoDateWaivesNothing: fail-closed, the posture an
// unresolvable digest, an undatable candidate and an unreadable scan all take.
// An acceptance is a dated judgement; with no readable date there is nothing
// saying it is still one somebody has taken. `cidx security vuln check` names
// the malformed date, and the finding reaches the audit like any other.
func TestAnAcceptanceWithNoDateWaivesNothing(t *testing.T) {
	today := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)

	for _, expires := range []string{"", "soon", "2026-13-45", "02/03/2026"} {
		entry := []Vulnerability{{CVE: "CVE-2026-0001", Repository: "rust", Expires: expires}}
		if got := waiving(entry, today); len(got) != 0 {
			t.Errorf("an entry dated %q still waives its finding", expires)
		}
	}
}

// TestTheIgnoreFileAndTheTabAgreeOnTheBoundary: the file drops the entry on the
// day the Security tab starts calling it expired, never a day either side.
// Two answers to "has this acceptance lapsed" is the one bug neither view could
// be trusted after.
func TestTheIgnoreFileAndTheTabAgreeOnTheBoundary(t *testing.T) {
	entry := Vulnerability{CVE: "CVE-2026-0001", Repository: "rust", Expires: "2026-03-02"}
	exception := []presets.Exception{{CVE: entry.CVE, Repository: entry.Repository, Expires: entry.Expires}}

	for day := time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC); day.Before(time.Date(2026, 3, 7, 0, 0, 0, 0, time.UTC)); day = day.AddDate(0, 0, 1) {
		filters := len(waiving([]Vulnerability{entry}, day)) == 1
		expired := len(presets.ExpiredExceptions(exception, day)) == 1
		if filters == expired {
			t.Errorf("on %s the ignore file filters = %v and the tab reports expired = %v",
				day.Format(time.DateOnly), filters, expired)
		}
	}
}
