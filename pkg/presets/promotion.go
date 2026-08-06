package presets

import (
	"fmt"
	"sort"
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

// ScanDecision is the verdict on what the monitor's scanners found on one
// candidate, in the words the workflow summary and the promotion PR print
// verbatim.
type ScanDecision struct {
	// Promote reports whether the findings leave the candidate promotable.
	Promote bool

	// Reason states why — for a pass as much as for a hold. A candidate the
	// scan gate holds has to say so somewhere, or the promotion silently
	// swallows it, which is the failure mode #247 was about in the first place.
	Reason string

	// Introduces names the findings that blocked the promotion: on the
	// candidate, on neither the running image's record nor the candidate's own,
	// and therefore new. Empty on a pass, so a promotion never implies findings
	// it does not have.
	Introduces []string
}

// EvaluateScan decides whether what the scanners found on a candidate blocks
// its promotion.
//
// found are the HIGH/CRITICAL vulnerabilities the monitor's scanners reported
// against the candidate. accepted are the ones a human has argued
// (known-vulnerabilities.toml, the file the security audit maintains). carried
// are the ones the same run's scanners reported against the image the
// catalogue runs today.
//
// accepted and carried are two records of "what we already run", and the gate
// needs both (issue #379). It used to consult the file alone, assuming a green
// audit kept the file identical to reality; with the audit red, every CVE the
// database had learned since the file was written read as "introduced" — the
// rust:1.97.1-slim candidate was held for 80 CVEs, 80 of which the running
// image reported the same day, and no candidate promoted for months. The file
// answers for what has been judged; the same-day scan answers for what is
// actually carried; only a finding on neither is the candidate's own.
//
// The verdict is differential on purpose. Several catalogue images are
// knowingly vulnerable — that is exactly what known-vulnerabilities.toml
// records — so "the candidate has findings" would hold every one of them for
// ever, and a gate that never passes is worth as little as the one that never
// failed (#247). What blocks a promotion is a finding that is *new*: reported
// on the candidate, not already accepted on what we run today.
//
// It follows that a candidate carrying the same vulnerabilities as the running
// image is promotable. It is not a regression, and refusing it would strand the
// catalogue on an older image over a finding the candidate merely inherited —
// while the update it carries goes unapplied.
//
// Comparison is case-insensitive: Trivy spells severities and identifiers in
// upper case, Grype does not, and the same CVE reported by both must count once.
func EvaluateScan(found, accepted, carried []string) ScanDecision {
	onRecord := make(map[string]bool, len(accepted)+len(carried))
	for _, id := range accepted {
		onRecord[strings.ToUpper(id)] = true
	}
	for _, id := range carried {
		onRecord[strings.ToUpper(id)] = true
	}

	var introduces []string
	distinct := make(map[string]bool, len(found))
	for _, id := range found {
		key := strings.ToUpper(id)
		if distinct[key] {
			continue
		}
		distinct[key] = true
		if !onRecord[key] {
			introduces = append(introduces, id)
		}
	}
	sort.Strings(introduces)

	if len(introduces) == 0 {
		return ScanDecision{Promote: true, Reason: scanPassReason(len(distinct))}
	}

	return ScanDecision{
		Reason: fmt.Sprintf("held: introduces %s, %s neither carried nor accepted by the image we run today",
			strings.Join(introduces, ", "), isAre(len(introduces))),
		Introduces: introduces,
	}
}

// scanPassReason distinguishes the two ways a candidate clears the gate, because
// they are not the same news: nothing was found, or everything found was already
// on our own record.
//
// Which scanners were consulted is deliberately not stated here — this function
// weighs findings and does not know where they came from. The caller appends the
// provenance, so a verdict never implies a scanner that did not run.
func scanPassReason(found int) string {
	if found == 0 {
		return "no HIGH/CRITICAL finding reported"
	}
	return fmt.Sprintf("%d HIGH/CRITICAL finding(s), all already carried or accepted by the image we run today", found)
}

func isAre(n int) string {
	if n == 1 {
		return "which is"
	}
	return "which are"
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
