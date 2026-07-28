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
