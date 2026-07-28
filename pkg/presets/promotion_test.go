package presets

import (
	"strings"
	"testing"
	"time"
)

// fixedNow is a fixed point so the tests state ages, not dates.
var fixedNow = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)

func daysAgo(n int) time.Time {
	return fixedNow.AddDate(0, 0, -n)
}

// TestEvaluatePromotionServesTheCooldown: rule 2 in its ordinary form — a
// version has to have been public long enough for a compromise to have been
// noticed.
func TestEvaluatePromotionServesTheCooldown(t *testing.T) {
	tests := []struct {
		name        string
		published   time.Time
		wantPromote bool
		wantAgeDays int
	}{
		{"twenty days old is promoted", daysAgo(20), true, 20},
		{"exactly the cooldown is promoted", daysAgo(14), true, 14},
		{"one day short is held", daysAgo(13), false, 13},
		{"three days old is held", daysAgo(3), false, 3},
		{"published today is held", fixedNow, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EvaluatePromotion(tt.published, fixedNow, nil)

			if got.Promote != tt.wantPromote {
				t.Errorf("Promote = %v, want %v (reason: %s)", got.Promote, tt.wantPromote, got.Reason)
			}
			if got.AgeDays == nil {
				t.Fatalf("AgeDays = nil, want %d", tt.wantAgeDays)
			}
			if *got.AgeDays != tt.wantAgeDays {
				t.Errorf("AgeDays = %d, want %d", *got.AgeDays, tt.wantAgeDays)
			}
			if got.Reason == "" {
				t.Error("Reason is empty; a decision that cannot be explained cannot be reviewed")
			}
			if len(got.WaivedFor) != 0 {
				t.Errorf("WaivedFor = %v, want none: no vulnerability was in play", got.WaivedFor)
			}
		})
	}
}

// TestEvaluatePromotionHeldReasonStatesTheRemainingWait: the monitor runs
// weekly, so "held" without a horizon is an invitation to forget the candidate.
func TestEvaluatePromotionHeldReasonStatesTheRemainingWait(t *testing.T) {
	got := EvaluatePromotion(daysAgo(3), fixedNow, nil)

	if got.Promote {
		t.Fatal("a 3-day-old version should not be promoted")
	}
	if !strings.Contains(got.Reason, "11 days") {
		t.Errorf("Reason = %q, want it to state the 11 days left", got.Reason)
	}
}

// TestEvaluatePromotionFailsClosedWithoutADate: same posture as rule 1's
// unresolvable digest — no promotion on an assumption, and the reason is
// reported rather than the candidate silently dropped.
func TestEvaluatePromotionFailsClosedWithoutADate(t *testing.T) {
	got := EvaluatePromotion(time.Time{}, fixedNow, nil)

	if got.Promote {
		t.Error("an undatable candidate must not be promoted")
	}
	if got.AgeDays != nil {
		t.Errorf("AgeDays = %d, want nil: no date means no age", *got.AgeDays)
	}
	if !strings.Contains(got.Reason, "no publication date") {
		t.Errorf("Reason = %q, want it to name the missing date", got.Reason)
	}
}

// TestEvaluatePromotionFutureDateIsHeld: a date ahead of the clock is an
// anomaly, and an anomaly must not read as "old enough".
func TestEvaluatePromotionFutureDateIsHeld(t *testing.T) {
	if got := EvaluatePromotion(fixedNow.AddDate(0, 0, 2), fixedNow, nil); got.Promote {
		t.Errorf("a version published in the future was promoted: %s", got.Reason)
	}
}

// TestEvaluatePromotionWaivesForVulnerabilitiesAffectingUs: rule 3 — staying on
// a knowingly vulnerable image to guard against a hypothetical one is the worse
// trade.
func TestEvaluatePromotionWaivesForVulnerabilitiesAffectingUs(t *testing.T) {
	affecting := []string{"CVE-2026-0001", "CVE-2026-0002"}

	got := EvaluatePromotion(daysAgo(3), fixedNow, affecting)

	if !got.Promote {
		t.Fatalf("a fix for a vulnerability affecting us should be promoted, got: %s", got.Reason)
	}
	if len(got.WaivedFor) != 2 || got.WaivedFor[0] != "CVE-2026-0001" {
		t.Errorf("WaivedFor = %v, want %v", got.WaivedFor, affecting)
	}
	for _, cve := range affecting {
		if !strings.Contains(got.Reason, cve) {
			t.Errorf("Reason = %q, want it to name %s: the PR states which CVE bought the waiver", got.Reason, cve)
		}
	}
}

// TestEvaluatePromotionWaivesWithoutADate: waiting out a date that will never
// arrive would leave the vulnerability in place for good.
func TestEvaluatePromotionWaivesWithoutADate(t *testing.T) {
	got := EvaluatePromotion(time.Time{}, fixedNow, []string{"CVE-2026-0001"})

	if !got.Promote {
		t.Fatalf("an undatable fix for a vulnerability affecting us should be promoted, got: %s", got.Reason)
	}
	if len(got.WaivedFor) != 1 {
		t.Errorf("WaivedFor = %v, want the waiver to be reported", got.WaivedFor)
	}
}

// TestEvaluatePromotionClaimsNoWaiverItDidNotNeed: a waiver line in the PR must
// mean the cooldown was actually bypassed, or the record stops being readable.
func TestEvaluatePromotionClaimsNoWaiverItDidNotNeed(t *testing.T) {
	got := EvaluatePromotion(daysAgo(20), fixedNow, []string{"CVE-2026-0001"})

	if !got.Promote {
		t.Fatal("a version past the cooldown should be promoted")
	}
	if len(got.WaivedFor) != 0 {
		t.Errorf("WaivedFor = %v, want none: the cooldown was served, not waived", got.WaivedFor)
	}
	if strings.Contains(got.Reason, "waived") {
		t.Errorf("Reason = %q, want it not to claim a waiver", got.Reason)
	}
}

// TestPromotionCooldownIsTheDocumentedWindow pins the constant against the
// written policy (docs/core-concepts/security.md): the two must not drift.
func TestPromotionCooldownIsTheDocumentedWindow(t *testing.T) {
	if got := wholeDays(PromotionCooldown); got != 14 {
		t.Errorf("PromotionCooldown = %d days, want 14 as documented", got)
	}
}

// TestEvaluateScanPromotesACleanCandidate: nothing found, nothing to weigh.
func TestEvaluateScanPromotesACleanCandidate(t *testing.T) {
	got := EvaluateScan(nil, nil)

	if !got.Promote {
		t.Fatalf("a candidate with no finding should be promoted, got: %s", got.Reason)
	}
	if len(got.Introduces) != 0 {
		t.Errorf("Introduces = %v, want none", got.Introduces)
	}
	if !strings.Contains(got.Reason, "no HIGH/CRITICAL finding") {
		t.Errorf("Reason = %q, want it to state that nothing was found", got.Reason)
	}
}

// TestEvaluateScanPromotesAnInheritedFinding is the point of the whole gate
// being differential: the candidate carries what the running image already
// carries, so it is not a regression and must not block an otherwise legitimate
// update (#247).
func TestEvaluateScanPromotesAnInheritedFinding(t *testing.T) {
	got := EvaluateScan([]string{"CVE-2026-0001"}, []string{"CVE-2026-0001"})

	if !got.Promote {
		t.Fatalf("a finding the running image already has must not block, got: %s", got.Reason)
	}
	if len(got.Introduces) != 0 {
		t.Errorf("Introduces = %v, want none: the finding was inherited, not introduced", got.Introduces)
	}
	if !strings.Contains(got.Reason, "already accepted") {
		t.Errorf("Reason = %q, want it to say the findings were already accepted", got.Reason)
	}
}

// TestEvaluateScanHoldsANewFinding: the case the gate exists for.
func TestEvaluateScanHoldsANewFinding(t *testing.T) {
	got := EvaluateScan([]string{"CVE-2026-0001", "CVE-2026-0002"}, []string{"CVE-2026-0001"})

	if got.Promote {
		t.Fatalf("a candidate introducing a new finding must be held, got: %s", got.Reason)
	}
	if len(got.Introduces) != 1 || got.Introduces[0] != "CVE-2026-0002" {
		t.Errorf("Introduces = %v, want only the new CVE-2026-0002", got.Introduces)
	}
	if !strings.Contains(got.Reason, "CVE-2026-0002") {
		t.Errorf("Reason = %q, want it to name the finding that held the promotion", got.Reason)
	}
	if strings.Contains(got.Reason, "CVE-2026-0001") {
		t.Errorf("Reason = %q, want it not to blame an inherited finding", got.Reason)
	}
}

// TestEvaluateScanPromotesAnAcceptedNewFinding: a finding absent from the
// running image but on record — reviewed and accepted ahead of the promotion —
// is not what the gate is for.
func TestEvaluateScanPromotesAnAcceptedNewFinding(t *testing.T) {
	got := EvaluateScan([]string{"CVE-2026-0009"}, []string{"CVE-2026-0009"})

	if !got.Promote {
		t.Fatalf("an accepted finding must not hold a promotion, got: %s", got.Reason)
	}
}

// TestEvaluateScanCountsTheSameCVEOnce: Trivy and Grype both report it, and a
// duplicate must neither be listed twice nor inflate the count.
func TestEvaluateScanCountsTheSameCVEOnce(t *testing.T) {
	held := EvaluateScan([]string{"CVE-2026-0002", "cve-2026-0002"}, nil)
	if len(held.Introduces) != 1 {
		t.Errorf("Introduces = %v, want the CVE listed once", held.Introduces)
	}

	passed := EvaluateScan([]string{"CVE-2026-0001", "CVE-2026-0001"}, []string{"cve-2026-0001"})
	if !passed.Promote {
		t.Fatalf("a case-different match on record must still count as accepted, got: %s", passed.Reason)
	}
	if !strings.Contains(passed.Reason, "1 HIGH/CRITICAL") {
		t.Errorf("Reason = %q, want one finding counted, not two", passed.Reason)
	}
}

// TestEvaluateScanReportsFindingsInAStableOrder: the reason string lands in a
// PR body and a workflow summary, where a set iterated at random would produce
// a different text on every run.
func TestEvaluateScanReportsFindingsInAStableOrder(t *testing.T) {
	got := EvaluateScan([]string{"CVE-2026-0009", "CVE-2026-0002", "CVE-2026-0005"}, nil)

	want := []string{"CVE-2026-0002", "CVE-2026-0005", "CVE-2026-0009"}
	for i, id := range want {
		if got.Introduces[i] != id {
			t.Fatalf("Introduces = %v, want %v in order", got.Introduces, want)
		}
	}
}
