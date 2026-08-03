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
	// ExceptionLive covers a repository the catalogue runs today, for a CVE the
	// scanners still see on it. It is waiving a finding the audit would fail on;
	// nothing to decide.
	ExceptionLive = "live"

	// ExceptionCarryOver covers a repository the catalogue has left behind, for
	// a CVE another catalogue repository still carries. Purging it would lose
	// the justification and block the next scan; it has to be re-filed instead.
	ExceptionCarryOver = "carry-over"

	// ExceptionObsolete covers a CVE no catalogue image carries any more,
	// whether or not the catalogue still runs the repository it was written
	// against. It waives nothing and can go.
	ExceptionObsolete = "obsolete"

	// ExceptionUnknown covers an entry with no scan evidence to settle whether
	// its CVE is still carried. Fail-closed: not purgeable, because a purge on
	// no evidence is a guess.
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
// HIGH/CRITICAL results the scanners' reports **show**; a repository absent from
// the map has an image that was not scanned.
//
// suppressed maps the same repositories to what the audit's ignore file kept out
// of those reports. The two together are what the scanners saw, and only the two
// together can be asked whether a CVE is still carried: the audit generates that
// ignore file from these very entries, so an accepted CVE is deleted from its own
// repository's visible results by construction. Reading that absence as "gone"
// would purge every exception doing its job — which is why this test used to
// return live on the repository alone, and why the lifecycle could then never
// close on a repin of the same repository (#311). An exception only ever left the
// file when its repository did.
//
// Both scanners record what they suppressed: Grype in `ignoredMatches`, Trivy in
// `ExperimentalModifiedFindings` under `--show-suppressed`, which
// `security-audit.yml` passes for exactly this reason.
//
// recordsSuppressions is whether the results in hand actually carry that record,
// and it gates the *absence* conclusion alone. Trivy's report keeps no trace of
// whether the flag was passed — the field is simply omitted when a scan
// suppressed nothing — so results produced without it are indistinguishable from
// results that suppressed nothing, and reading them as the latter deletes every
// exception the ignore file was hiding. Measured on the audit's own artifacts
// from the day before the flag landed: four ansible entries, all still carried,
// all reported obsolete. Positive evidence never needs the gate — a CVE seen is
// a CVE carried, whichever half of the report it was seen in.
//
// The criterion is the CVE, not the tag, and it is the same one wherever the
// entry sits. An exception is obsolete once no catalogue image carries its CVE
// any more — and only when every catalogue image has been scanned, since a CVE
// cannot be shown absent from an image nobody looked at. Anything less is
// reported as unknown rather than purged, the same fail-closed posture the
// cooldown and the scan gate take.
//
// An entry still keyed the old way — a whole `repo:tag` where a repository
// belongs — matches no repository, so it is judged on its CVE alone. That is
// exactly what re-keying it requires, and it needs no special case.
func ClassifyException(cve, repository string, running []string, findings, suppressed map[string][]Finding, recordsSuppressions bool) ExceptionVerdict {
	ordered := append([]string(nil), running...)
	sort.Strings(ordered)

	carries := func(repo string) bool {
		return containsFold(FindingIDs(findings[repo]), cve) ||
			containsFold(FindingIDs(suppressed[repo]), cve)
	}

	// A fix is read from wherever it was reported. An exception must never be
	// written for a vulnerability that is fixed upstream, so an entry that turns
	// out to have one is an entry that should not exist: a repin candidate, not
	// a renewal. It keeps its state and stays on file — it still waives a
	// finding the audit would fail on until the repin happens — and the report
	// names it rather than letting it be renewed in silence (#312).
	fixFor := func(repo string) string {
		if fix := FixVersion(findings[repo], cve); fix != "" {
			return fix
		}
		return FixVersion(suppressed[repo], cve)
	}

	scanned := func(repo string) bool {
		_, ok := findings[repo]
		return ok
	}

	// unrecorded is the reason an entry stays put when the results cannot say
	// what their ignore file took out of them. It is not "no finding" — it is
	// "nobody kept the receipt", and the two must never read the same.
	unrecorded := fmt.Sprintf("the scan results record nothing they suppressed, so %s cannot be told apart from a finding the audit's own ignore file removed", cve)

	for _, repo := range ordered {
		if repo != repository {
			continue
		}
		switch {
		case carries(repository):
			return ExceptionVerdict{
				State:   ExceptionLive,
				Reason:  fmt.Sprintf("the catalogue runs %s and it still carries %s", repository, cve),
				FixedIn: fixFor(repository),
			}
		case !scanned(repository):
			return ExceptionVerdict{
				State:  ExceptionUnknown,
				Reason: fmt.Sprintf("%s has no scan result, so %s cannot be shown to have gone", repository, cve),
			}
		case !recordsSuppressions:
			return ExceptionVerdict{State: ExceptionUnknown, Reason: unrecorded}
		default:
			return ExceptionVerdict{
				State:  ExceptionObsolete,
				Reason: fmt.Sprintf("the catalogue still runs %s, but no image of it carries %s any more", repository, cve),
			}
		}
	}

	for _, repo := range ordered {
		if !carries(repo) {
			continue
		}
		return ExceptionVerdict{
			State:   ExceptionCarryOver,
			Reason:  fmt.Sprintf("the catalogue runs no image from %s, but %s still carries %s: re-file the exception against it rather than deleting the justification", repository, repo, cve),
			StillOn: repo,
			FixedIn: fixFor(repo),
		}
	}

	var unscanned []string
	for _, repo := range ordered {
		if !scanned(repo) {
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
	if !recordsSuppressions {
		return ExceptionVerdict{State: ExceptionUnknown, Reason: unrecorded}
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
