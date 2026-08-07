package presets

import (
	"strings"
	"testing"
	"time"
)

// TestStaleTag covers issue #282: the cooldown reads a candidate's publication
// date and asks whether it is old enough to trust, and nothing ever asked the
// mirror-image question about the tag we already run.
//
// Four of the five images that reported "nothing to update" in #238 were in
// fact stale, abandoned, or mirrored from a dead channel — prettier last
// published thirteen months earlier, kaniko from a project archived the year
// before — and every check cidx had answered that all was well.
func TestStaleTag(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		published time.Time
		wantStale bool
	}{
		{
			// prettier's shape: tmknom/prettier last published 2025-06-30,
			// thirteen months before #238 replaced it.
			name:      "thirteen months without a republication",
			published: now.AddDate(0, -13, 0),
			wantStale: true,
		},
		{
			name:      "a day past the threshold",
			published: now.Add(-StaleAfter - 24*time.Hour),
			wantStale: true,
		},
		{
			name:      "a day short of it",
			published: now.Add(-StaleAfter + 24*time.Hour),
			wantStale: false,
		},
		{
			name:      "published this morning",
			published: now.Add(-6 * time.Hour),
			wantStale: false,
		},
		{
			// A registry that publishes no date says nothing rather than
			// "stale": an absence of evidence must not read as evidence, which
			// is the rule the cooldown already follows from the other side.
			name:      "a registry that dates nothing",
			published: time.Time{},
			wantStale: false,
		},
		{
			// Clock skew between a registry and the runner. A date in the
			// future is not an age.
			name:      "published after now",
			published: now.Add(48 * time.Hour),
			wantStale: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, stale := StaleTag(tt.published, now, "v1.2.3")

			if stale != tt.wantStale {
				t.Fatalf("stale = %v, want %v (reason %q)", stale, tt.wantStale, reason)
			}
			if !stale {
				if reason != "" {
					t.Errorf("a tag that is not stale must say nothing, got %q", reason)
				}
				return
			}
			for _, want := range []string{"v1.2.3", "last published"} {
				if !strings.Contains(reason, want) {
					t.Errorf("reason = %q, want it to mention %q", reason, want)
				}
			}
		})
	}
}

// TestStaleAfterIsNotTheCooldownReversed: the two thresholds read the same
// dates and answer different questions, so nothing should tempt a future reader
// into expressing one in terms of the other.
func TestStaleAfterIsNotTheCooldownReversed(t *testing.T) {
	if StaleAfter <= PromotionCooldown {
		t.Errorf("StaleAfter (%s) must be the long horizon, not the cooldown's (%s): "+
			"one measures how long a new version proves itself, the other how long an old one "+
			"may go unmaintained", StaleAfter, PromotionCooldown)
	}
}
