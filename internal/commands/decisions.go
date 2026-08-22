package commands

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// A Decision is one reviewed exposure judgement about a repository, under the
// aggregate context of every preset consuming it. Members reference it by ID
// instead of cloning its status, date and prose — the file the discussion
// measured held 119 word-identical notes, which is one judgement stored 119
// times (docs/discussions/vulnerability-management-reset.md).
type Decision struct {
	ID         string `toml:"id"`
	Repository string `toml:"repository"`
	Model      string `toml:"model"`

	// Capabilities is the mechanical context the review argued against,
	// derived from the consumer presets' declarations at review time. Any
	// difference from the context derived today invalidates the decision.
	Capabilities []string `toml:"capabilities"`

	// Requires names the semantic predicates the exposure model depends on,
	// such as "bounded-input". A predicate the current context has not
	// established is unknown, and unknown never satisfies a requirement.
	Requires []string `toml:"requires,omitempty"`

	Treatment  string   `toml:"treatment"`
	Reviewed   string   `toml:"reviewed"`
	ReviewBy   string   `toml:"review_by"`
	Reason     string   `toml:"reason"`
	References []string `toml:"references"`
}

// DecisionContext is what is true of a repository's consumers when waivers are
// resolved: the mechanical capabilities derived from the preset declarations,
// and the semantic predicates a review has established.
type DecisionContext struct {
	Capabilities []string
	Semantics    []string
}

// A MemberVerdict says whether one entry still waives its finding, and why not
// when it does not. The reason is for the reviewer: it names the date, the
// capability or the predicate that stopped the waiver.
type MemberVerdict struct {
	CVE    string
	Waived bool
	Reason string
}

// ResolveWaivers judges every entry of the file on `day`.
//
// A legacy entry — no decision reference — answers with its own expiry,
// exactly as before the decisions table existed: migration changes nothing it
// has not reviewed. A member answers with its decision, and the decision must
// still stand entirely — date, mechanical context, semantic predicates.
// Anything it rested on that cannot be shown today waives nothing.
func ResolveWaivers(file *VulnerabilityFile, contexts map[string]DecisionContext, day time.Time) []MemberVerdict {
	byID := make(map[string]Decision, len(file.Decisions))
	for _, d := range file.Decisions {
		byID[d.ID] = d
	}

	verdicts := make([]MemberVerdict, 0, len(file.Vulnerabilities))
	for _, v := range file.Vulnerabilities {
		if v.Decision == "" {
			verdicts = append(verdicts, MemberVerdict{CVE: v.CVE, Waived: presets.Waives(v.Expires, day)})
			continue
		}
		verdicts = append(verdicts, memberVerdict(v, byID[v.Decision], contexts, day))
	}
	return verdicts
}

func memberVerdict(v Vulnerability, d Decision, contexts map[string]DecisionContext, day time.Time) MemberVerdict {
	if !presets.Waives(d.ReviewBy, day) {
		return MemberVerdict{CVE: v.CVE, Reason: fmt.Sprintf("decision %s needs review: review_by %s has passed", d.ID, d.ReviewBy)}
	}
	ctx, known := contexts[d.Repository]
	if !known {
		return MemberVerdict{CVE: v.CVE, Reason: fmt.Sprintf("decision %s: no context derived for %s", d.ID, d.Repository)}
	}
	if diff := capabilityDiff(d.Capabilities, ctx.Capabilities); diff != "" {
		return MemberVerdict{CVE: v.CVE, Reason: fmt.Sprintf("decision %s: context changed since review: %s", d.ID, diff)}
	}
	for _, predicate := range d.Requires {
		if !slices.Contains(ctx.Semantics, predicate) {
			return MemberVerdict{CVE: v.CVE, Reason: fmt.Sprintf("decision %s: %s is unproven for %s", d.ID, predicate, d.Repository)}
		}
	}
	return MemberVerdict{CVE: v.CVE, Waived: true}
}

// capabilityDiff names what appeared or disappeared, because a reviewer must be
// able to see why a context change invalidated a group — an opaque mismatch
// would be a hash with error formatting.
func capabilityDiff(reviewed, current []string) string {
	var changes []string
	for _, c := range current {
		if !slices.Contains(reviewed, c) {
			changes = append(changes, c+" appeared")
		}
	}
	for _, c := range reviewed {
		if !slices.Contains(current, c) {
			changes = append(changes, c+" disappeared")
		}
	}
	sort.Strings(changes)
	return strings.Join(changes, ", ")
}

// A FixObservation is suppressed scanner evidence that a fixed version exists
// for a CVE: the version, and the day the evidence was first seen. It travels
// through #312's channel — what the scanners recorded as suppressed — and is
// never inferred from a finding's absence.
type FixObservation struct {
	CVE          string
	FixedVersion string
	Observed     string // YYYY-MM-DD
}

// A FixTransition queues one member to leave its acceptance for remediation.
// The fixable-age clock starts on the day the fix was observed, not on the day
// somebody got around to looking.
type FixTransition struct {
	CVE          string
	Decision     string
	FixedVersion string
	ClockStart   string
}

// FixTransitions reports which members of a decision have a fix on evidence.
// It reports and changes nothing — the convention `vuln prune` holds: the
// default run is the one that changes nothing, and one member gaining a fix
// says nothing about its neighbours.
func FixTransitions(file *VulnerabilityFile, observations []FixObservation) []FixTransition {
	byCVE := make(map[string]FixObservation, len(observations))
	for _, obs := range observations {
		byCVE[obs.CVE] = obs
	}

	var transitions []FixTransition
	for _, v := range file.Vulnerabilities {
		if v.Decision == "" {
			continue // legacy entries are `vuln prune`'s Fixed-upstream report
		}
		if obs, seen := byCVE[v.CVE]; seen {
			transitions = append(transitions, FixTransition{
				CVE:          v.CVE,
				Decision:     v.Decision,
				FixedVersion: obs.FixedVersion,
				ClockStart:   obs.Observed,
			})
		}
	}
	return transitions
}

// validateDecisionRefs refuses a file whose member references a decision that
// does not exist. Half-loading would let the member fall back to legacy
// semantics it no longer carries; refusing names both halves of the broken
// reference, so the fix is one edit away.
func validateDecisionRefs(file *VulnerabilityFile) error {
	ids := make(map[string]bool, len(file.Decisions))
	for _, d := range file.Decisions {
		ids[d.ID] = true
	}
	for _, v := range file.Vulnerabilities {
		if v.Decision != "" && !ids[v.Decision] {
			return fmt.Errorf("%s references unknown decision %q", v.CVE, v.Decision)
		}
	}
	return nil
}
