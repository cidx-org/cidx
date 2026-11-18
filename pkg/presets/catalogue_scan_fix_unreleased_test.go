package presets

import (
	"testing"
	"time"
)

func fixUnreleasedScan(day string, decision FixUnreleased) []ImageScan {
	on, _ := time.Parse(time.DateOnly, day)
	return []ImageScan{{
		Image: scanTestImage,
		Findings: []Finding{
			{ID: "CVE-2026-111497", Severity: "HIGH", Package: "brotli-libs", FixedIn: "1.2.1-r0", Scanner: "Trivy"},
			{ID: "CVE-2026-999", Severity: "HIGH", Package: "curl", FixedIn: "8.9.0", Scanner: "Trivy"},
		},
		FixUnreleased: []FixUnreleased{decision},
		On:            on,
	}}
}

func propertiesOf(t *testing.T, scans []ImageScan, advisory string) map[string]any {
	t.Helper()
	for _, r := range scanTestResults(t, scanTestLog(t, scans, nil)) {
		if r["ruleId"] == advisory {
			return r["properties"].(map[string]any)
		}
	}
	t.Fatalf("no result for %s", advisory)
	return nil
}

var brotliDecision = FixUnreleased{
	ID:         "brotli-fix-unreleased",
	Repository: repositoryOf(scanTestImage),
	Reason:     "Fixed in the distribution, shipped by no image.",
	ReviewBy:   "2026-12-18",
	Advisories: []string{"CVE-2026-111497"},
}

// TestScanMarksTheFindingsAStandingFixUnreleasedDecisionCovers: same repository,
// same advisory, review date not passed.
func TestScanMarksTheFindingsAStandingFixUnreleasedDecisionCovers(t *testing.T) {
	props := propertiesOf(t, fixUnreleasedScan("2026-11-18", brotliDecision), "CVE-2026-111497")
	if props["exceptionCategory"] != "fix-unreleased" {
		t.Errorf("exceptionCategory = %v, want fix-unreleased", props["exceptionCategory"])
	}
	if props["exceptionReason"] != brotliDecision.Reason {
		t.Errorf("exceptionReason = %v, want the decision's reason", props["exceptionReason"])
	}
	if props["fixedVersion"] != "1.2.1-r0" {
		t.Errorf("the marked result lost its own properties: %v", props)
	}

	// The last day of the review is still inside it.
	props = propertiesOf(t, fixUnreleasedScan("2026-12-18", brotliDecision), "CVE-2026-111497")
	if props["exceptionCategory"] != "fix-unreleased" {
		t.Errorf("review_by day itself: exceptionCategory = %v", props["exceptionCategory"])
	}
}

// TestScanFallsBackToTheDecisionIDWithoutAReason: the reason is the id when the
// decision has none.
func TestScanFallsBackToTheDecisionIDWithoutAReason(t *testing.T) {
	d := brotliDecision
	d.Reason = ""
	props := propertiesOf(t, fixUnreleasedScan("2026-11-18", d), "CVE-2026-111497")
	if props["exceptionReason"] != d.ID {
		t.Errorf("exceptionReason = %v, want %s", props["exceptionReason"], d.ID)
	}
}

// TestScanDoesNotMarkAnotherAdvisoryOnTheSameImage: coverage is by advisory.
func TestScanDoesNotMarkAnotherAdvisoryOnTheSameImage(t *testing.T) {
	props := propertiesOf(t, fixUnreleasedScan("2026-11-18", brotliDecision), "CVE-2026-999")
	for _, key := range []string{"exceptionCategory", "exceptionReason"} {
		if _, has := props[key]; has {
			t.Errorf("a fixable finding of another advisory carries %s: %v", key, props)
		}
	}
	if props["fixedVersion"] != "8.9.0" {
		t.Errorf("the unmarked result changed: %v", props)
	}
}

// TestScanDoesNotMarkWithAnExpiredOrForeignDecision: a lapsed review marks
// nothing, nor does a decision about another repository or one naming no advisory.
func TestScanDoesNotMarkWithAnExpiredOrForeignDecision(t *testing.T) {
	expired := brotliDecision
	other := brotliDecision
	other.Repository = "dhi.io/other"
	unnamed := brotliDecision
	unnamed.Advisories = nil

	for name, scans := range map[string][]ImageScan{
		"expired": fixUnreleasedScan("2026-12-19", expired),
		"foreign": fixUnreleasedScan("2026-11-18", other),
		"unnamed": fixUnreleasedScan("2026-11-18", unnamed),
	} {
		props := propertiesOf(t, scans, "CVE-2026-111497")
		if _, has := props["exceptionCategory"]; has {
			t.Errorf("%s decision marked the result: %v", name, props)
		}
	}
}
