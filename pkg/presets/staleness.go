package presets

import (
	"fmt"
	"time"
)

// StaleAfter is how long a pinned tag may go without being republished before
// the catalogue says so.
//
// A year, because a year is uncontroversial: no image in the catalogue is meant
// to sit unrebuilt for that long, and the two the signal was designed against
// were well past it — prettier at thirteen months, kaniko on a project archived
// the year before (#238, #282).
//
// It is deliberately not the cooldown's fourteen days seen from the other side.
// Those measure how long a *new* version must prove itself; this measures how
// long an *old* one may go unmaintained, and the two questions have nothing to
// do with each other beyond reading the same dates.
const StaleAfter = 365 * 24 * time.Hour

// StaleTag reports that the tag the catalogue pins has not been republished in
// a long time, and how long that is.
//
// This is the question nobody was asking (issue #282). The cooldown reads a
// candidate's publication date and asks "is it old enough to trust?"; the same
// listing carries the date of the tag we already run, and nothing ever asked
// "is what we run suspiciously old?". Four of the five images that reported
// "nothing to update" in #238 were stale, abandoned, or mirrored from a dead
// channel, and every check cidx had answered that all was well.
//
// A report, never a verdict. An image can be legitimately old — a tool that is
// finished does not need a release — so what this produces is a line in the
// monitor's summary for a human to weigh, in the same family as `missing`
// (#245) and `frozen_variant` (#252): a reference that still resolves while the
// thing behind it has stopped.
//
// A registry that publishes no date says nothing rather than "stale": an
// absence of evidence must not read as evidence, which is the rule the cooldown
// already follows on the other side.
func StaleTag(publishedAt, now time.Time, tag string) (reason string, stale bool) {
	if publishedAt.IsZero() || publishedAt.After(now) {
		return "", false
	}

	age := now.Sub(publishedAt)
	if age < StaleAfter {
		return "", false
	}

	return fmt.Sprintf("%q was last published %s ago, over the %s staleness threshold: "+
		"check whether the project still publishes, and whether another registry or variant is current",
		tag, inDays(wholeDays(age)), inDays(wholeDays(StaleAfter))), true
}
