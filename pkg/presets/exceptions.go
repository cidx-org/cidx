package presets

import (
	"fmt"
	"sort"
	"strings"
)

// The lifecycle of a vulnerability exception
// (docs/core-concepts/security.md, issue #238).
//
// An exception records a judgement about a CVE in an image, in our usage — and
// that judgement survives a version bump, because none of what it rests on
// changed. Keying it to `repo:tag` was the original design and it was wrong:
// every promotion left the entries recorded against the replaced version behind,
// matching nothing and waiving nothing, and nothing ever asked whether they
// still had an object. All 155 entries reached that state.
//
// So an exception is keyed by **repository and CVE**; the tag it was first seen
// on is context, not identity. It dies when its CVE is no longer carried by any
// catalogue image, or when its expiry falls — never because a tag moved.
//
// "The tag changed" is not the criterion either. When an image is promoted, an
// accepted CVE either disappeared with the old image — the exception has no
// object left and should go — or it came along to the new one, and deleting the
// record would lose the justification *and* turn the audit red on the next scan.
// Only one of those is a purge, and telling them apart takes the findings, not
// the tags.

// The four states an exception can be in. They are the vocabulary the report
// and the docs share, so they are spelled once, here.
const (
	// ExceptionLive covers a repository the catalogue runs today. Nothing to
	// decide.
	ExceptionLive = "live"

	// ExceptionCarryOver covers a repository the catalogue has left behind, for
	// a CVE another catalogue repository still carries. Purging it would lose
	// the justification and block the next scan; it has to be re-filed instead.
	ExceptionCarryOver = "carry-over"

	// ExceptionObsolete covers a repository the catalogue has left behind, for a
	// CVE no catalogue image carries any more. It waives nothing and can go.
	ExceptionObsolete = "obsolete"

	// ExceptionUnknown covers a repository the catalogue has left behind, with
	// no scan evidence to say whether the CVE survived the move. Fail-closed:
	// not purgeable, because a purge on no evidence is a guess.
	ExceptionUnknown = "unknown"
)

// ExceptionVerdict is what becomes of one exception, in the words the report
// prints verbatim.
type ExceptionVerdict struct {
	// State is one of the four constants above.
	State string

	// Reason states why, for every state — an entry the report leaves in place
	// has to say what it is waiting for, or the file goes quiet again.
	Reason string

	// StillOn names the catalogue repository that still carries the CVE, which
	// is the repository the entry has to be re-filed against. Set for carry-over
	// alone, so a verdict never implies a repository it did not find the finding
	// on.
	StillOn string

	// FixedIn carries the fix the scanners reported for this CVE, when there is
	// one. An exception must never be written for a vulnerability that is fixed
	// upstream — that is image freshness, not a decision — so an entry that
	// turns out to have a fix is named as such rather than silently renewed.
	FixedIn string
}

// ClassifyException decides what becomes of the exception recorded for cve
// against repository.
//
// running is the set of repositories the catalogue runs today — registry and
// path, no tag, no digest. findings maps those same repositories to the
// HIGH/CRITICAL results the scanners reported on them; a repository absent from
// the map has an image that was not scanned.
//
// The criterion is the CVE, not the tag. An exception whose repository the
// catalogue left behind is only obsolete once the findings show no catalogue
// image carries its CVE any more — and only when every catalogue image has been
// scanned, since a CVE cannot be shown absent from an image nobody looked at.
// Anything less is reported as unknown rather than purged, the same fail-closed
// posture the cooldown and the scan gate take.
//
// The repository test comes first and does not consult the findings, which is
// load-bearing rather than an optimisation: the security audit generates its
// ignore file from these very exceptions, so a CVE accepted on a repository the
// catalogue runs is filtered out of that repository's own scan results by
// construction. Reading its absence as "gone" would delete every exception that
// is doing its job, and the next audit would go red on all of them.
//
// An entry still keyed the old way — a whole `repo:tag` where a repository
// belongs — matches no repository, so it is judged on its CVE alone. That is
// exactly what re-keying it requires, and it needs no special case.
func ClassifyException(cve, repository string, running []string, findings map[string][]Finding) ExceptionVerdict {
	ordered := append([]string(nil), running...)
	sort.Strings(ordered)

	for _, repo := range ordered {
		if repo == repository {
			return ExceptionVerdict{
				State:  ExceptionLive,
				Reason: "covers a repository the catalogue runs today",
			}
		}
	}

	for _, repo := range ordered {
		if !containsFold(FindingIDs(findings[repo]), cve) {
			continue
		}
		return ExceptionVerdict{
			State:   ExceptionCarryOver,
			Reason:  fmt.Sprintf("the catalogue runs no image from %s, but %s still carries %s: re-file the exception against it rather than deleting the justification", repository, repo, cve),
			StillOn: repo,
			FixedIn: FixVersion(findings[repo], cve),
		}
	}

	var unscanned []string
	for _, repo := range ordered {
		if _, ok := findings[repo]; !ok {
			unscanned = append(unscanned, repo)
		}
	}
	if len(unscanned) > 0 {
		return ExceptionVerdict{
			State: ExceptionUnknown,
			Reason: fmt.Sprintf("the catalogue runs no image from %s, and %s has no scan result, so %s cannot be shown to have gone",
				repository, unscannedList(unscanned), cve),
		}
	}

	return ExceptionVerdict{
		State:  ExceptionObsolete,
		Reason: fmt.Sprintf("the catalogue runs no image from %s, and no catalogue image carries %s any more", repository, cve),
	}
}

// unscannedList names the repositories that leave the question open, and stops
// naming them once the answer is "most of them" — a reason line listing twenty
// references says less than a count.
func unscannedList(repos []string) string {
	if len(repos) > 3 {
		return fmt.Sprintf("%d catalogue repositories", len(repos))
	}
	return strings.Join(repos, ", ")
}

// containsFold matches identifiers the way the scanners spell them: Trivy
// shouts, Grype capitalises, and the same CVE has to count once either way.
func containsFold(ids []string, want string) bool {
	for _, id := range ids {
		if strings.EqualFold(id, want) {
			return true
		}
	}
	return false
}
