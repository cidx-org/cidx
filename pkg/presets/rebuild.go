package presets

import (
	"fmt"
	"strings"
	"time"
)

// RebuiltTag reports that the tag the catalogue pins no longer resolves to the
// digest pinned beside it — the same name, different content.
//
// This is the hole the pin leaves open (issue #332). Pinning by digest makes a
// reference immutable, which is the point: what runs is what was reviewed. The
// cost is that nothing upstream can ever reach us again. `koalaman/shellcheck:stable`
// and `buildpack-deps:trixie-curl` carry no version, so the promotion path —
// which compares tags — has nothing to say about them for ever (#328); and
// while that was reported, the rebuild itself never was. When the signal was
// first run against the catalogue, `buildpack-deps:trixie-curl` had been
// republished the day before, and nothing had noticed.
//
// A report, never a promotion. Adopting a new digest on an unchanged tag is
// also the quietest attack vector there is, and precisely the one digest
// pinning exists to stop: a compromised publisher pushes to the same name, and
// no scan can see a backdoor that carries no CVE. So the rebuild is surfaced
// for a human to weigh, in the same family as `missing` (#245),
// `frozen_variant` (#252) and `stale_tag` (#282) — a reference that still
// resolves while something behind it has changed.
//
// The same comparison reads two different ways, which is why it is worth
// running against every image rather than only the unversioned ones:
//
//   - on a tag that carries no version, a moved digest is the update channel
//     working exactly as designed. It is the only channel those images have.
//   - on a versioned tag, the version names the same release, so whatever
//     changed is something the version does not describe. Usually that is a
//     rebuild against patched base packages — which is the entire proposition
//     of the hardened images the catalogue runs, and precisely the fix channel
//     a digest pin does not receive. Occasionally it is a question for the
//     publisher. The check cannot tell the two apart, so it reports the fact
//     and names both readings rather than picking one.
//
// The second case is not the rare one. When the check was first run, four of
// the five moved digests were dhi.io images on versioned tags: the catalogue
// was pinned away from the very rebuilds it runs those images for, and the
// promotion path — which only ever sees a new version number — could not have
// said so.
//
// publishedAt is the date the registry gives for the pinned tag, which on a
// tag that moved is the date it moved. A registry that publishes no date says
// nothing rather than guessing, the same rule StaleTag follows.
func RebuiltTag(pinnedDigest, currentDigest, tag string, publishedAt, now time.Time) (reason string, rebuilt bool) {
	if pinnedDigest == "" || currentDigest == "" || pinnedDigest == currentDigest {
		return "", false
	}

	moved := fmt.Sprintf("%q now resolves to %s, not the pinned %s",
		tag, shortDigest(currentDigest), shortDigest(pinnedDigest))
	if age, ok := republishedAgo(publishedAt, now); ok {
		moved += ", republished " + age + " ago"
	}

	if VersionedTag(tag) {
		return moved + ": the version names the same release, so what changed is not something the version describes — " +
			"a rebuild against patched base packages, or a question for the publisher — review and repin by hand", true
	}

	return moved + ": this is how a tag carrying no version receives its updates — " +
		"review what changed and repin by hand, since no scan can vouch for content nobody has read", true
}

// republishedAgo renders how long ago the registry says the tag was last
// published, when it says at all.
func republishedAgo(publishedAt, now time.Time) (string, bool) {
	if publishedAt.IsZero() || publishedAt.After(now) {
		return "", false
	}
	return inDays(wholeDays(now.Sub(publishedAt))), true
}

// shortDigest trims a digest to the prefix humans compare, keeping the
// algorithm so the value stays recognisable as one.
func shortDigest(digest string) string {
	algorithm, hex, found := strings.Cut(digest, ":")
	if !found || len(hex) <= 12 {
		return digest
	}
	return algorithm + ":" + hex[:12]
}
