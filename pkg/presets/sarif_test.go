package presets

import (
	"strings"
	"testing"
)

// The two references the ansible repin moves between: same repository, new tag,
// new digest. Every test below is about what may and what may not change with
// them.
const (
	ansibleBefore = "ghcr.io/ansible/community-ansible-dev-tools:v26.7.1@sha256:4d4db3e75c48ce64763d26adbca58ff3f8b93a8ddae785373ac973b4f20a7d92"
	ansibleAfter  = "ghcr.io/ansible/community-ansible-dev-tools:v26.8.0@sha256:0000000000000000000000000000000000000000000000000000000000000000"
)

// unfixed is a finding needing triage: no fix at any version, not exempt by
// class. The only population TriageAlerts publishes.
func unfixed(id string) []Finding {
	return []Finding{{ID: id, Severity: "HIGH", Package: "github.com/opencontainers/runc", PackageType: "gobinary", Scanner: "Grype"}}
}

func onlyAlert(t *testing.T, alerts []Alert) Alert {
	t.Helper()
	if len(alerts) != 1 {
		t.Fatalf("alerts = %d, want exactly one", len(alerts))
	}
	return alerts[0]
}

// TestTriageAlertIdentitySurvivesARepin is the standing guard behind #313.
//
// GitHub matches an alert across runs on its rule identifier and its
// fingerprint. Both were keyed on the pinned reference, digest included, so a
// repin changed the identity of *every* alert on that image: the tab closed all
// of them and opened near-identical replacements, including for CVEs that never
// went away — which destroys the one thing the Security tab was added for.
//
// A fingerprint derived from the reference again fails here, which is the whole
// point of the test: the property has to hold on the day of the next repin, not
// only on the day the fix was written.
func TestTriageAlertIdentitySurvivesARepin(t *testing.T) {
	before := onlyAlert(t, TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 172))
	after := onlyAlert(t, TriageAlerts(ansibleAfter, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 174))

	if before.Fingerprint != after.Fingerprint {
		t.Errorf("the repin changed the alert's fingerprint:\n  before %s\n  after  %s\n\n"+
			"GitHub would close the alert and open a near-identical one for a CVE that never went away (#313).\n"+
			"Key the fingerprint on the repository, as the rule identifier and known-vulnerabilities.toml do.",
			before.Fingerprint, after.Fingerprint)
	}
	if before.RuleID != after.RuleID {
		t.Errorf("RuleID = %q then %q, want the repository-keyed identifier either side of a repin",
			before.RuleID, after.RuleID)
	}

	// The test would pass just as well on two alerts that are identical in every
	// respect, and that is not what is being claimed: the reference did move, and
	// the alert says so where a reader needs it.
	if !strings.Contains(after.Message, ansibleAfter) {
		t.Errorf("Message = %q, want the pinned reference the finding was seen on", after.Message)
	}
	if before.Line == after.Line {
		t.Fatalf("both alerts point at line %d, so the repin was not staged", before.Line)
	}
}

// TestTriageAlertPointsAtTheLinePinningTheImage: the identity survives the
// repin, the location follows it. The alert is actionable because it links to
// the line that has to change, and a repin rewrites that line in place.
func TestTriageAlertPointsAtTheLinePinningTheImage(t *testing.T) {
	alert := onlyAlert(t, TriageAlerts(ansibleAfter, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 174))

	if alert.File != CatalogueFile {
		t.Errorf("File = %q, want %q", alert.File, CatalogueFile)
	}
	if alert.Line != 174 {
		t.Errorf("Line = %d, want the line pinning the image the finding was seen on", alert.Line)
	}
}

// TestTriageAlertFingerprintsStayDistinct: dropping the reference from the key
// must not collapse alerts that answer different questions. Two repositories
// carrying one CVE are two repins, and the baseline counts them as two.
func TestTriageAlertFingerprintsStayDistinct(t *testing.T) {
	ansible := onlyAlert(t, TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 172))
	rust := onlyAlert(t, TriageAlerts("rust:1.97.0-slim@sha256:686a437ead83701e8f871e66e838c3ec55f46b5fc235b025756396ac823bdc51",
		[]string{"cargo-build"}, unfixed("CVE-2025-31133"), 909))
	other := onlyAlert(t, TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-52565"), 172))

	if ansible.Fingerprint == rust.Fingerprint {
		t.Errorf("the same CVE on two repositories shares a fingerprint (%s), so one of the two repins would never be shown",
			ansible.Fingerprint)
	}
	if ansible.Fingerprint == other.Fingerprint {
		t.Errorf("two CVEs on one repository share a fingerprint (%s)", ansible.Fingerprint)
	}
}

// TestExpiredExceptionAlertIdentityIsKeyedOnTheRepository: the alerts the
// triage ones now match. An acceptance is keyed by repository and CVE, and the
// alert reporting its expiry has always been keyed the same way — that is the
// consistency #313 asks the triage alerts to join.
func TestExpiredExceptionAlertIdentityIsKeyedOnTheRepository(t *testing.T) {
	expired := []Exception{{
		CVE:        "CVE-2025-31133",
		Repository: "ghcr.io/ansible/community-ansible-dev-tools",
		Severity:   "HIGH",
		Status:     "third-party",
		Added:      "2025-12-02",
		Expires:    "2026-03-02",
		Line:       36,
	}}

	alert := onlyAlert(t, ExpiredExceptionAlerts(expired, day("2026-08-02")))
	triage := onlyAlert(t, TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 172))

	if alert.File != ExceptionsFile || alert.Line != 36 {
		t.Errorf("the expired alert points at %s:%d, want the entry to re-argue", alert.File, alert.Line)
	}
	// Same repository, same CVE, two different things to do about it: the two
	// alerts must not share an identity either.
	if alert.Fingerprint == triage.Fingerprint {
		t.Errorf("the expired-exception alert and the triage alert share a fingerprint (%s)", alert.Fingerprint)
	}
}

// One alert per repository and CVE (#303)
//
// The two families could not collide before the ignore file started honouring
// `expires`: a lapsed acceptance filtered its finding out of the audit's own
// results, so TriageAlerts never saw it. Now it does, and the same CVE on the
// same repository has one alert from each.

// TestAnExpiredAcceptanceSupersedesTheTriageAlert: one CVE, one repository, one
// decision — and the alert that survives is the one naming the judgement that
// lapsed, because it points at the entry a human edits rather than at the image
// pin, and it does not offer `vuln add` for a CVE that already has an entry.
func TestAnExpiredAcceptanceSupersedesTheTriageAlert(t *testing.T) {
	triage := TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 172)
	expired := ExpiredExceptionAlerts([]Exception{{
		CVE:        "CVE-2025-31133",
		Repository: "ghcr.io/ansible/community-ansible-dev-tools",
		Severity:   "HIGH",
		Expires:    "2026-03-02",
		Line:       36,
	}}, day("2026-08-02"))

	alert := onlyAlert(t, MergeAlerts(triage, expired))
	if alert.File != ExceptionsFile {
		t.Errorf("the surviving alert points at %s, want the entry that expired", alert.File)
	}
	if alert.Fingerprint != expired[0].Fingerprint {
		t.Error("the surviving alert is not the one code scanning has been showing since #301")
	}
}

// TestAnExpiredAcceptanceOnlySupersedesItsOwnCVE: the suppression is keyed on
// the repository and the identifier, exactly as everything else here is. A
// lapsed acceptance must not silence the rest of an image's queue.
func TestAnExpiredAcceptanceOnlySupersedesItsOwnCVE(t *testing.T) {
	triage := TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-52565"), 172)
	expired := ExpiredExceptionAlerts([]Exception{{
		CVE:        "CVE-2025-31133",
		Repository: "ghcr.io/ansible/community-ansible-dev-tools",
		Severity:   "HIGH",
		Expires:    "2026-03-02",
	}}, day("2026-08-02"))

	merged := MergeAlerts(triage, expired)
	if len(merged) != 2 {
		t.Fatalf("merged alerts = %d, want the triage alert and the expired one", len(merged))
	}
}

// TestAnAcceptanceStillWithinItsDateSupersedesNothing: it is still filtering,
// so there is no triage alert to collide with — and nothing may be dropped on
// its account either.
func TestAnAcceptanceStillWithinItsDateSupersedesNothing(t *testing.T) {
	triage := TriageAlerts(ansibleBefore, []string{"ansible-lint"}, unfixed("CVE-2025-31133"), 172)
	expired := ExpiredExceptionAlerts([]Exception{{
		CVE:        "CVE-2025-31133",
		Repository: "ghcr.io/ansible/community-ansible-dev-tools",
		Severity:   "HIGH",
		Expires:    "2999-01-01",
	}}, day("2026-08-02"))

	alert := onlyAlert(t, MergeAlerts(triage, expired))
	if alert.File != CatalogueFile {
		t.Errorf("the surviving alert points at %s, want the line pinning the image", alert.File)
	}
}
