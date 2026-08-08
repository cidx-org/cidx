package presets

import (
	"strings"
	"testing"
	"time"
)

const (
	pinned = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	moved  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
)

func rebuildNow(t *testing.T) time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, "2026-08-07T09:00:00Z")
	if err != nil {
		t.Fatalf("bad test clock: %v", err)
	}
	return now
}

// TestRebuiltTagSaysNothingWhenTheDigestHeld is the case that must stay silent:
// fifteen of the sixteen catalogue images were in it when the signal was first
// run, and a report on those would drown the one that mattered.
func TestRebuiltTagSaysNothingWhenTheDigestHeld(t *testing.T) {
	now := rebuildNow(t)

	tests := []struct {
		name           string
		pinned, actual string
	}{
		{name: "the tag still points where it was pinned", pinned: pinned, actual: pinned},
		{name: "nothing pinned, so nothing to move away from", pinned: "", actual: moved},
		{name: "the registry answered nothing", pinned: pinned, actual: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, rebuilt := RebuiltTag(tt.pinned, tt.actual, "trixie-curl", now.AddDate(0, 0, -1), now)
			if rebuilt {
				t.Errorf("RebuiltTag() reported %q, want silence", reason)
			}
		})
	}
}

// TestRebuiltTagReportsAnUnversionedTagThatMoved is issue #332 as observed:
// `buildpack-deps:trixie-curl` had been republished the day before the check
// first ran, and every signal cidx had answered that the image was current.
func TestRebuiltTagReportsAnUnversionedTagThatMoved(t *testing.T) {
	now := rebuildNow(t)

	reason, rebuilt := RebuiltTag(pinned, moved, "trixie-curl", now.AddDate(0, 0, -1), now)
	if !rebuilt {
		t.Fatal("a pinned tag resolving to another digest is the whole of #332: it must be reported")
	}

	for _, want := range []string{"trixie-curl", "sha256:222222222222", "sha256:111111111111", "1 day ago", "repin"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason = %q, want it to carry %q", reason, want)
		}
	}
	if strings.Contains(reason, moved) {
		t.Errorf("reason = %q, want the digests shortened to the prefix a human compares", reason)
	}
}

// TestRebuiltTagReadsAVersionedTagDifferently is the reason the check runs
// against every image rather than only the unversioned ones. On a name, a moved
// digest is the update channel; on a version, it is a promise broken, and
// nothing in cidx would have said so.
func TestRebuiltTagReadsAVersionedTagDifferently(t *testing.T) {
	now := rebuildNow(t)

	named, _ := RebuiltTag(pinned, moved, "trixie-curl", time.Time{}, now)
	versioned, rebuilt := RebuiltTag(pinned, moved, "v8.30.1", time.Time{}, now)

	if !rebuilt {
		t.Fatal("a versioned tag republished under new content must be reported too")
	}
	if versioned == named {
		t.Fatal("a republished version and a rebuilt name read the same, so the summary cannot tell them apart")
	}
	if !strings.Contains(versioned, "publisher") {
		t.Errorf("reason = %q, want a republished version to send the reader upstream, not to the pin", versioned)
	}
	if strings.Contains(versioned, "how a tag carrying no version receives its updates") {
		t.Errorf("reason = %q, want it not to call a broken promise the normal update channel", versioned)
	}
}

// TestRebuiltTagWithoutADateStillReports: ghcr and quay date none of their tags
// (#245). The rebuild is a fact about two digests, and it holds whether or not
// anyone will say when it happened — only the age drops.
func TestRebuiltTagWithoutADateStillReports(t *testing.T) {
	now := rebuildNow(t)

	tests := []struct {
		name      string
		published time.Time
	}{
		{name: "the registry publishes no date", published: time.Time{}},
		{name: "the date is in the future, so it is not a date", published: now.AddDate(0, 0, 3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reason, rebuilt := RebuiltTag(pinned, moved, "stable", tt.published, now)
			if !rebuilt {
				t.Fatal("an undated rebuild is still a rebuild")
			}
			if strings.Contains(reason, "republished") {
				t.Errorf("reason = %q, want no age claimed when none can be read", reason)
			}
		})
	}
}

func TestShortDigest(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{in: pinned, want: "sha256:111111111111"},
		{in: "sha256:abc", want: "sha256:abc"},
		{in: "notadigest", want: "notadigest"},
		{in: "", want: ""},
	}

	for _, tt := range tests {
		if got := shortDigest(tt.in); got != tt.want {
			t.Errorf("shortDigest(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
