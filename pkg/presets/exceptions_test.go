package presets

import "testing"

var catalogueRefs = []string{
	"dhi.io/trivy:0.68",
	"golangci/golangci-lint:v2.12.2-alpine",
}

// scannedClean is the evidence state where every catalogue image was scanned and
// none of them reported anything. Stated explicitly, because the difference
// between "scanned, nothing found" and "not scanned" is the whole point.
func scannedClean() map[string][]string {
	findings := make(map[string][]string, len(catalogueRefs))
	for _, ref := range catalogueRefs {
		findings[ref] = nil
	}
	return findings
}

// TestExceptionOnARunningImageIsLive: an exception covering what the catalogue
// runs today is never in question, whatever the scanners say.
func TestExceptionOnARunningImageIsLive(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "dhi.io/trivy:0.68", catalogueRefs, scannedClean())

	if verdict.State != ExceptionLive {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionLive)
	}
}

// TestExceptionSurvivingThePromotionIsCarriedOver: the trap this command exists
// for. The tag moved, so the entry matches nothing — but the CVE came along to
// the new image. Purging it would lose the justification and leave the next scan
// with an unexplained HIGH/CRITICAL.
func TestExceptionSurvivingThePromotionIsCarriedOver(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy:0.68"] = []string{"CVE-2026-0001"}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
	if verdict.StillOn != "dhi.io/trivy:0.68" {
		t.Errorf("StillOn = %q, want the image that still carries it", verdict.StillOn)
	}
}

// TestCarryOverIsCaseInsensitive: Trivy shouts identifiers, Grype capitalises
// them, and an exception must not read as obsolete because the scanner that
// found it spelled it differently.
func TestCarryOverIsCaseInsensitive(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy:0.68"] = []string{"ghsa-cgrx-mc8f-2prm"}

	verdict := ClassifyException("GHSA-cgrx-mc8f-2prm", "aquasec/trivy:0.67.2", catalogueRefs, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestExceptionWhoseCVEIsGoneIsObsolete: the image was replaced and the finding
// went with it. Nothing left to waive.
func TestExceptionWhoseCVEIsGoneIsObsolete(t *testing.T) {
	findings := scannedClean()
	findings["dhi.io/trivy:0.68"] = []string{"CVE-2026-9999"}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, findings)

	if verdict.State != ExceptionObsolete {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionObsolete)
	}
}

// TestExceptionWithoutScanEvidenceIsUnknown: a CVE cannot be shown absent from
// an image nobody scanned. Fail-closed, like the cooldown on an undatable
// candidate and the scan gate on an unreadable result.
func TestExceptionWithoutScanEvidenceIsUnknown(t *testing.T) {
	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, nil)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestPartialScanEvidenceIsUnknown: one image missing from the results is enough
// to leave the question open — the CVE could be on exactly that one.
func TestPartialScanEvidenceIsUnknown(t *testing.T) {
	findings := map[string][]string{"dhi.io/trivy:0.68": nil}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, findings)

	if verdict.State != ExceptionUnknown {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionUnknown)
	}
}

// TestCarryOverBeatsMissingEvidence: a CVE found on a scanned image is an answer
// on its own — no amount of unscanned images makes it less true.
func TestCarryOverBeatsMissingEvidence(t *testing.T) {
	findings := map[string][]string{"dhi.io/trivy:0.68": {"CVE-2026-0001"}}

	verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, findings)

	if verdict.State != ExceptionCarryOver {
		t.Fatalf("state = %q (%s), want %q", verdict.State, verdict.Reason, ExceptionCarryOver)
	}
}

// TestEveryVerdictStatesAReason: an entry the report leaves in place has to say
// what it is waiting for, or the file goes quiet exactly the way it did before.
func TestEveryVerdictStatesAReason(t *testing.T) {
	cases := []struct {
		name     string
		findings map[string][]string
	}{
		{"live", scannedClean()},
		{"obsolete", scannedClean()},
		{"unknown", nil},
	}

	for _, tc := range cases {
		verdict := ClassifyException("CVE-2026-0001", "aquasec/trivy:0.67.2", catalogueRefs, tc.findings)
		if verdict.Reason == "" {
			t.Errorf("%s: verdict %q states no reason", tc.name, verdict.State)
		}
	}
}
