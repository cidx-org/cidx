package presets

import "testing"

// catalogueRepos is what the catalogue runs, as exceptions key it: repository,
// no tag, no digest.
var catalogueRepos = []string{
	"dhi.io/trivy",
	"golangci/golangci-lint",
}

// scannedClean is the evidence state where every catalogue repository was
// scanned and none of them reported anything. Stated explicitly, because the
// difference between "scanned, nothing found" and "not scanned" is the whole
// point.
func scannedClean() map[string][]Finding {
	findings := make(map[string][]Finding, len(catalogueRepos))
	for _, repo := range catalogueRepos {
		findings[repo] = nil
	}
	return findings
}

// TestExceptionOnARunningRepositoryIsLive: an exception covering what the
// catalogue runs today is never in question, whatever the scanners say.
//
// It is checked before the findings on purpose. The audit builds its ignore file
// from these very entries, so a CVE accepted on a running repository is filtered
// out of that repository's own scan results; reading its absence as "gone" would
// purge every exception that is doing its job.
func TestExceptionOnARunningRepositoryIsLive(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean())

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestExceptionSurvivesATagMove: the reason the key is the repository. The
// catalogue promoting `dhi.io/trivy:0.68` to `0.71` changes none of what the
// judgement rests on, and under the old `repo:tag` key every entry for it
// stopped matching anything on the day of the promotion.
func TestExceptionSurvivesATagMove(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy", catalogueRepos, scannedClean())

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q: the tag is context, not identity", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestAnUnmigratedEntryMatchesNoRepository: an entry still keyed the old way
// carries a whole `repo:tag` where a repository belongs. It equals no
// repository, so it is judged on its CVE alone — which is exactly what re-keying
// it requires, and needs no special case.
func TestAnUnmigratedEntryMatchesNoRepository(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "golangci/golangci-lint:v2.6.2", catalogueRepos, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy" {
		t.Errorf("StillOn = %q, want the repository that carries it", verdict.StillOn)
	}
}

// TestExceptionSurvivingThePromotionIsCarriedOver: the trap this command exists
// for. The repository was replaced, so the entry matches nothing — but the CVE
// came along to the new image. Purging it would lose the justification and leave
// the next scan with an unexplained HIGH/CRITICAL.
func TestExceptionSurvivingThePromotionIsCarriedOver(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy" {
		t.Errorf("StillOn = %q, want the repository that still carries it", verdict.StillOn)
	}
}

// TestCarryOverNamesTheFixWhenThereIsOne: a carried-over CVE that is fixed
// upstream is not exception territory at all — the policy says never to write
// one for it — so the verdict has to say so rather than let the entry be
// re-filed in silence.
func TestCarryOverNamesTheFixWhenThereIsOne(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-0001", Severity: "HIGH", FixedIn: "1.2.8"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings)

	if verdict.FixedIn != "1.2.8" {
		t.Errorf("FixedIn = %q, want the version the scanners reported", verdict.FixedIn)
	}
}

// TestCarryOverIsCaseInsensitive: Trivy shouts identifiers, Grype capitalises
// them, and an exception must not read as obsolete because the scanner that
// found it spelled it differently.
func TestCarryOverIsCaseInsensitive(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "ghsa-cgrx-mc8f-2prm", Severity: "HIGH"}}

	verdict := ClassifyException("GHSA-cgrx-mc8f-2prm", "aquasec/trivy", catalogueRepos, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestExceptionWhoseCVEIsGoneIsObsolete: the repository was replaced and the
// finding went with it. Nothing left to waive.
func TestExceptionWhoseCVEIsGoneIsObsolete(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy"] = []Finding{{ID: "CVE-2026-9999", Severity: "HIGH"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings)

	if verdict.State != ExceptionObsolete {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionObsolete)
	}
}

// TestExceptionWithoutScanEvidenceIsUnknown: a CVE cannot be shown absent from
// an image nobody scanned. Fail-closed, like the cooldown on an undatable
// candidate and the scan gate on an unreadable result.
func TestExceptionWithoutScanEvidenceIsUnknown(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, nil)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestPartialScanEvidenceIsUnknown: one repository missing from the results is
// enough to leave the question open — the CVE could be on exactly that one.
func TestPartialScanEvidenceIsUnknown(t *testing.T) {
	findings := map[string][]Finding{"dhi.io/trivy": nil}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestCarryOverBeatsMissingEvidence: a CVE found on a scanned repository is an
// answer on its own — no amount of unscanned images makes it less true.
func TestCarryOverBeatsMissingEvidence(t *testing.T) {
	findings := map[string][]Finding{"dhi.io/trivy": {{ID: "CVE-2026-0001", Severity: "HIGH"}}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestEveryVerdictStatesAReason: an entry the report leaves in place has to say
// what it is waiting for, or the file goes quiet exactly the way it did before.
func TestEveryVerdictStatesAReason(t *testing.T) {
	cases := []struct {
		name     string
		findings map[string][]Finding
	}{
		{"live", scannedClean()},
		{"obsolete", scannedClean()},
		{"unknown", nil},
	}

	for _, tc := range cases {
		verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy", catalogueRepos, tc.findings)
		if verdict.Reason == "" {
			t.Errorf("%s: verdict %q states no reason", tc.name, verdict.State)
		}
	}
}
