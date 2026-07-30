package presets

import (
	"fmt"
	"sort"
	"strings"
)

// The lifecycle of a vulnerability exception
// (docs/core-concepts/security.md, issue #238).
//
// An exception is recorded against `repo:tag`, and the catalogue moves. Once it
// does, the record covers an image nothing runs any more — and the file keeps
// it for ever, because nothing ever asked whether it still had an object. 155
// of 155 entries were in that state.
//
// "The tag changed" is not the question, though. When an image is promoted, an
// accepted CVE either disappeared with the old image — the exception has no
// object left and should go — or it came along to the new one, and deleting the
// record would lose the justification *and* turn the audit red on the next
// scan. Only one of those is a purge, and telling them apart takes the findings,
// not the tags.

// The four states an exception can be in. They are the vocabulary the report
// and the docs share, so they are spelled once, here.
const (
	// ExceptionLive covers an image the catalogue runs today. Nothing to decide.
	ExceptionLive = "live"

	// ExceptionCarryOver covers an image the catalogue has left behind, for a
	// CVE the image that replaced it still carries. Purging it would lose the
	// justification and block the next scan; it has to be re-filed instead.
	ExceptionCarryOver = "carry-over"

	// ExceptionObsolete covers an image the catalogue has left behind, for a CVE
	// no catalogue image carries any more. It waives nothing and can go.
	ExceptionObsolete = "obsolete"

	// ExceptionUnknown covers an image the catalogue has left behind, with no
	// scan evidence to say whether the CVE survived the move. Fail-closed: not
	// purgeable, because a purge on no evidence is a guess.
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

	// StillOn names the catalogue image that still carries the CVE. Set for
	// carry-over alone, so a verdict never implies an image it did not find the
	// finding on.
	StillOn string
}

// ClassifyException decides what becomes of the exception recorded for cve
// against image.
//
// running is the set of references the catalogue runs today, digest-free, in the
// form known-vulnerabilities.toml records exceptions in. findings maps those same
// references to the HIGH/CRITICAL identifiers the scanners reported on them; a
// reference absent from the map was not scanned.
//
// The criterion is the CVE, not the tag. An exception whose image the catalogue
// left behind is only obsolete once the findings show no catalogue image carries
// its CVE any more — and only when every catalogue image has been scanned, since
// a CVE cannot be shown absent from an image nobody looked at. Anything less is
// reported as unknown rather than purged, the same fail-closed posture the
// cooldown and the scan gate take.
func ClassifyException(cve, image string, running []string, findings map[string][]string) ExceptionVerdict {
	ordered := append([]string(nil), running...)
	sort.Strings(ordered)

	for _, ref := range ordered {
		if ref == image {
			return ExceptionVerdict{
				State:  ExceptionLive,
				Reason: "covers an image the catalogue runs today",
			}
		}
	}

	for _, ref := range ordered {
		if containsFold(findings[ref], cve) {
			return ExceptionVerdict{
				State:   ExceptionCarryOver,
				Reason:  fmt.Sprintf("the catalogue no longer runs %s, but %s still carries %s: re-file the exception against it rather than deleting the justification", image, ref, cve),
				StillOn: ref,
			}
		}
	}

	var unscanned []string
	for _, ref := range ordered {
		if _, ok := findings[ref]; !ok {
			unscanned = append(unscanned, ref)
		}
	}
	if len(unscanned) > 0 {
		return ExceptionVerdict{
			State: ExceptionUnknown,
			Reason: fmt.Sprintf("the catalogue no longer runs %s, and %s has no scan result, so %s cannot be shown to have gone",
				image, unscannedList(unscanned), cve),
		}
	}

	return ExceptionVerdict{
		State:  ExceptionObsolete,
		Reason: fmt.Sprintf("the catalogue no longer runs %s, and no catalogue image carries %s any more", image, cve),
	}
}

// unscannedList names the images that leave the question open, and stops naming
// them once the answer is "most of them" — a reason line listing twenty
// references says less than a count.
func unscannedList(refs []string) string {
	if len(refs) > 3 {
		return fmt.Sprintf("%d catalogue images", len(refs))
	}
	return strings.Join(refs, ", ")
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
