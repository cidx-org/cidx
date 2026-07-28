package presets

import (
	"fmt"
	"strings"
	"time"
)

// Rules 2 and 3 of the image supply-chain policy
// (docs/core-concepts/security.md, issue #242).
//
// Rule 1 made every catalogue reference immutable. That closes the "same tag,
// different content" vector but not the one it exists for: a version published
// with a backdoor is perfectly immutable too, and carries no CVE until someone
// finds it. The only defence against a delay measured in weeks is to wait.

// PromotionCooldown is how long a newly published version must have been
// publicly available before the catalogue promotes it. The monitor runs weekly,
// so this is roughly two cycles.
//
// It is deliberately not a user-facing setting: the policy governs cidx's own
// catalogue, not the projects that use cidx, which pin whatever they want in
// their own cidx.toml (guardrail 5). Shortening the window is a decision to
// argue for in a commit, not to bury in a config file.
const PromotionCooldown = 14 * 24 * time.Hour

// PromotionDecision is the verdict on one candidate version, in the words the
// workflow summary and the promotion PR print verbatim.
type PromotionDecision struct {
	// Promote reports whether the candidate may replace the running image.
	Promote bool

	// Reason states why. Always set, for promotions as much as for holds — a
	// candidate held for another week has to say so somewhere, or the policy
	// silently swallows it.
	Reason string

	// WaivedFor names the vulnerabilities that bought the candidate its way
	// past the cooldown. Empty when no waiver was needed, so a promotion never
	// claims a waiver that did nothing.
	WaivedFor []string

	// AgeDays is how long the candidate has been public, in whole days. Nil
	// when the registry gave no date.
	AgeDays *int
}

// EvaluatePromotion applies the cooldown and its exception to one candidate.
//
// published is when the candidate became publicly available; the zero time
// means the registry would not say. affectingUs are the HIGH/CRITICAL
// vulnerabilities already recorded against the image the catalogue runs today.
//
// An undatable candidate is held. That mirrors rule 1's treatment of an
// unresolvable digest: the promotion is skipped rather than taken on an
// assumption, and the reason is reported so it does not vanish quietly.
//
// The exception then overrides the hold — for a young candidate and an
// undatable one alike. Waiting out a date that will never arrive would just
// leave a known vulnerability in place, and deliberately running a
// known-vulnerable image to guard against a hypothetical one is the worse
// trade. A candidate that has served the cooldown claims no waiver: it did not
// need one.
func EvaluatePromotion(published, now time.Time, affectingUs []string) PromotionDecision {
	cooldownDays := wholeDays(PromotionCooldown)

	if !published.IsZero() {
		age := now.Sub(published)
		ageDays := wholeDays(age)

		if age >= PromotionCooldown {
			return PromotionDecision{
				Promote: true,
				Reason:  fmt.Sprintf("published %s ago, past the %d-day cooldown", inDays(ageDays), cooldownDays),
				AgeDays: &ageDays,
			}
		}

		if len(affectingUs) > 0 {
			return PromotionDecision{
				Promote:   true,
				Reason:    waiverReason(cooldownDays, affectingUs),
				WaivedFor: affectingUs,
				AgeDays:   &ageDays,
			}
		}

		return PromotionDecision{
			Reason: fmt.Sprintf("held: published %s ago, %s of the %d-day cooldown left",
				inDays(ageDays), inDays(cooldownDays-ageDays), cooldownDays),
			AgeDays: &ageDays,
		}
	}

	if len(affectingUs) > 0 {
		return PromotionDecision{
			Promote:   true,
			Reason:    waiverReason(cooldownDays, affectingUs),
			WaivedFor: affectingUs,
		}
	}

	return PromotionDecision{
		Reason: fmt.Sprintf("held: no publication date available, so the %d-day cooldown cannot be shown to have elapsed", cooldownDays),
	}
}

// waiverReason spells out rule 3 the way the promotion PR has to state it:
// which vulnerabilities, on the image we run today, bought the wait off.
func waiverReason(cooldownDays int, affectingUs []string) string {
	return fmt.Sprintf("%d-day cooldown waived: the running image is affected by %s",
		cooldownDays, strings.Join(affectingUs, ", "))
}

// wholeDays renders a duration in the unit the policy is written and reviewed
// in. Partial days round down, so a candidate is never called old enough a few
// hours early.
func wholeDays(d time.Duration) int {
	return int(d / (24 * time.Hour))
}

// inDays spells a day count out for the humans reading the promotion PR and the
// workflow summary, where "1 days" would be the only thing they notice.
func inDays(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}
