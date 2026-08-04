package presets

import (
	"testing"
	"time"
)

// buildpackDeps is the tag listing `library/buildpack-deps` actually answers,
// trimmed to the shapes that matter: Debian suites by codename (`trixie`,
// `bookworm`), Debian rolling channels by name (`stable`, `testing`, `sid`) and
// Ubuntu releases by number (`26.10`, `26.04`, `24.04`), all in one namespace,
// each in three variants (bare, `-curl`, `-scm`).
//
// It is the listing behind #328: on 2026-07-30, `26.10` was Ubuntu's
// development branch and `trixie-curl` was Debian 13 stable.
var buildpackDeps = []string{
	"latest", "stable", "stable-curl", "stable-scm",
	"testing", "testing-curl", "testing-scm",
	"unstable", "unstable-curl", "unstable-scm",
	"sid", "sid-curl", "sid-scm",
	"trixie", "trixie-curl", "trixie-scm",
	"bookworm", "bookworm-curl", "bookworm-scm",
	"bullseye", "bullseye-curl", "bullseye-scm",
	"stonking", "stonking-curl", "stonking-scm",
	"26.10", "26.10-curl", "26.10-scm",
	"resolute", "resolute-curl", "resolute-scm",
	"26.04", "26.04-curl", "26.04-scm",
	"25.10", "25.10-curl", "25.10-scm",
	"noble", "noble-curl", "noble-scm",
	"24.04", "24.04-curl", "24.04-scm",
	"22.04", "22.04-curl", "22.04-scm",
}

// onDay is the date a scenario is read against. No test in this repository
// touches the network, and none should depend on the day it runs either.
func onDay(t *testing.T, day string) time.Time {
	t.Helper()

	now, err := time.Parse(time.RFC3339, day)
	if err != nil {
		t.Fatalf("bad test date %q: %v", day, err)
	}
	return now
}

// TestNewerTagRefusesADevelopmentChannel is the case reported in #328, both
// halves of it, against the listing the registry really answers.
//
// `buildpack-deps` versions its tags by Debian codename *and* by Ubuntu release
// number in one namespace, so "the newest number" and "the newest version of
// what we pin" are different questions. The first defect was answering the
// first one; the second is that Ubuntu publishes the development branch of its
// next release under that release's own number, months early.
func TestNewerTagRefusesADevelopmentChannel(t *testing.T) {
	now := onDay(t, "2026-07-30T00:00:00Z")

	tests := []struct {
		name    string
		current string
		want    string
	}{
		{
			name:    "a Debian suite is offered no Ubuntu release number at all",
			current: "trixie-curl",
		},
		{
			name:    "nor is a suite that carries no variant",
			current: "trixie",
		},
		{
			name:    "an Ubuntu release is not offered the development branch of the next one",
			current: "24.04-curl",
			want:    "26.04-curl",
		},
		{
			name:    "the same, in the bare variant",
			current: "22.04",
			want:    "26.04",
		},
		{
			name:    "an Ubuntu release already at the newest published one is offered nothing",
			current: "26.04-curl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerTag(tt.current, buildpackDeps, now); got != tt.want {
				t.Errorf("NewerTag(%q, buildpack-deps) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}

// TestNewerTagOffersACalendarReleaseOnceItsMonthArrives: the refusal is read
// against the calendar, so it lets go by itself. Nothing has to be edited when
// 26.10 ships — which is the point of reading a date rather than keeping a list
// of development branches.
func TestNewerTagOffersACalendarReleaseOnceItsMonthArrives(t *testing.T) {
	now := onDay(t, "2026-10-10T00:00:00Z")

	if got := NewerTag("24.04-curl", buildpackDeps, now); got != "26.10-curl" {
		t.Errorf("NewerTag(\"24.04-curl\", buildpack-deps) = %q, want %q", got, "26.10-curl")
	}
}

// TestNewerTagStillOffersTheRepinsTheCatalogueTook: every promotion shape the
// catalogue has actually taken (#277, #280, #286, #293), so the two refusals
// above cannot quietly cost the detector its job.
//
// The calendar rule in particular only applies where the *pinned* tag is itself
// calendar-versioned — `v2.95` and `0.71` are not October of anything — and
// that is what these lock.
func TestNewerTagStillOffersTheRepinsTheCatalogueTook(t *testing.T) {
	now := onDay(t, "2026-07-30T00:00:00Z")

	tests := []struct {
		name      string
		current   string
		available []string
		want      string
	}{
		{
			name:      "commitizen 4.15.1 → 4.16.5 (#277)",
			current:   "4.15.1",
			available: []string{"latest", "4.15.1", "4.16.0", "4.16.5"},
			want:      "4.16.5",
		},
		{
			name:      "commitlint 21.0.0 → 21.2.1 (#277)",
			current:   "21.0.0",
			available: []string{"latest", "21.0.0", "21.2.1"},
			want:      "21.2.1",
		},
		{
			name:      "goreleaser v2.15.4 → v2.17.0 (#277)",
			current:   "v2.15.4",
			available: []string{"latest", "v2.15.4", "v2.16.0", "v2.17.0"},
			want:      "v2.17.0",
		},
		{
			name:      "black 26.3.1 → 26.5.1, three components and not a calendar (#277)",
			current:   "26.3.1",
			available: []string{"latest", "26.3.1", "26.4.0", "26.5.1"},
			want:      "26.5.1",
		},
		{
			name:      "gh v2.95 → v2.97, two components whose minor is no month (#277)",
			current:   "v2.95",
			available: []string{"latest", "v2.95", "v2.96", "v2.97"},
			want:      "v2.97",
		},
		{
			name:      "golangci-lint keeps its -alpine variant (#280)",
			current:   "v2.12.2-alpine",
			available: []string{"latest", "v2.13.0", "v2.13.0-alpine", "v2.13.0-alpine-dev"},
			want:      "v2.13.0-alpine",
		},
		{
			name:      "rust keeps its -slim variant (#286)",
			current:   "1.97.0-slim",
			available: []string{"1.97.1", "1.97.1-slim", "1.97.1-alpine", "1.98.0-slim"},
			want:      "1.98.0-slim",
		},
		{
			name:      "the DHI go image keeps its -alpine-dev variant (#293)",
			current:   "1.26.5-alpine-dev",
			available: []string{"1.26.6", "1.26.6-alpine-dev", "1.26.6-alpine-fips-dev"},
			want:      "1.26.6-alpine-dev",
		},
		{
			name:      "the DHI python image keeps its -alpine-dev variant (#293)",
			current:   "3.13.14-alpine-dev",
			available: []string{"3.14.6-alpine-dev", "3.13.15-alpine-dev", "3.14.6"},
			want:      "3.14.6-alpine-dev",
		},
		{
			name:      "trivy 0.71 → 0.72, two components with a leading zero (#286)",
			current:   "0.71",
			available: []string{"latest", "0.71", "0.72"},
			want:      "0.72",
		},
		{
			name:      "alpine-base 3.24 → 3.25 (#293)",
			current:   "3.24",
			available: []string{"latest", "3.24", "3.25"},
			want:      "3.25",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerTag(tt.current, tt.available, now); got != tt.want {
				t.Errorf("NewerTag(%q, %v) = %q, want %q", tt.current, tt.available, got, tt.want)
			}
		})
	}
}

// TestVersionedTag: the third state #328 names. A tag that is a name rather
// than a version can never be compared to anything a registry lists, and the
// two catalogue images in that position have to be reported rather than filed
// as up to date.
func TestVersionedTag(t *testing.T) {
	versioned := []string{"1.24", "v2.95", "26.04", "29-cli", "1.97.0-slim", "3.13.14-alpine-dev"}
	for _, tag := range versioned {
		if !VersionedTag(tag) {
			t.Errorf("VersionedTag(%q) = false, want true", tag)
		}
	}

	named := []string{"trixie-curl", "stable", "latest", "nightly", "sha-1b2c3d4", ""}
	for _, tag := range named {
		if VersionedTag(tag) {
			t.Errorf("VersionedTag(%q) = true, want false", tag)
		}
	}
}

// TestNewerTagEdges covers what the Gherkin scenarios in
// features/presets/update_detection.feature would only make noisy: the shapes a
// real registry listing throws in around the rule.
func TestNewerTagEdges(t *testing.T) {
	now := onDay(t, "2026-07-30T00:00:00Z")

	tests := []struct {
		name      string
		current   string
		available []string
		want      string
	}{
		{
			name:    "an empty listing offers nothing",
			current: "0.8.2",
		},
		{
			name:      "a four-component version compares component by component",
			current:   "1.2.3.4",
			available: []string{"1.2.3.5", "1.2.4.0", "1.10.0.0"},
			want:      "1.10.0.0",
		},
		{
			name:      "a suffix that is not a variant separator still has to match",
			current:   "29-cli",
			available: []string{"30", "30-cli-dev", "30-cli"},
			want:      "30-cli",
		},
		{
			name:      "a tag whose version overflows an int is skipped, not crashed on",
			current:   "1.0",
			available: []string{"99999999999999999999.0", "1.1"},
			want:      "1.1",
		},
		{
			name:      "a date-shaped tag stays in its own family",
			current:   "1.0",
			available: []string{"20260728", "1.1"},
			want:      "1.1",
		},
		{
			name:      "an older listing offers nothing",
			current:   "3.24",
			available: []string{"3.23", "3.22"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerTag(tt.current, tt.available, now); got != tt.want {
				t.Errorf("NewerTag(%q, %v) = %q, want %q", tt.current, tt.available, got, tt.want)
			}
		})
	}
}

// TestSupersedingVariant covers the frozen variant line (#252): the catalogue
// pins a family the repository has stopped publishing, so nothing newer will
// ever appear inside it and "up to date" is the wrong answer.
func TestSupersedingVariant(t *testing.T) {
	// The listing dhi.io/golang actually answers, trimmed to the shapes that
	// matter: no -alpine3.21-dev tag remains anywhere in it.
	golang := []string{
		"1", "1-alpine3.23-dev", "1-alpine3.24-dev",
		"1.25", "1.25-alpine3.23-dev", "1.25-alpine3.24-dev", "1.25-alpine3.24-fips-dev",
		"1.26", "1.26-alpine3.23-dev", "1.26-alpine3.24-dev",
	}

	tests := []struct {
		name      string
		current   string
		available []string
		want      string
	}{
		{
			name:      "a family the repository dropped names its successor",
			current:   "1.23-alpine3.21-dev",
			available: golang,
			want:      "-alpine3.24-dev",
		},
		{
			name:      "a family still published is not frozen, even at its own head",
			current:   "1.26-alpine3.24-dev",
			available: golang,
		},
		{
			name:      "a family still published behind its own head is not frozen either",
			current:   "1.25-alpine3.23-dev",
			available: golang,
		},
		{
			name:      "the successor keeps the rest of the suffix",
			current:   "1.23-alpine3.21-fips-dev",
			available: golang,
			want:      "-alpine3.24-fips-dev",
		},
		{
			name:      "a variant carrying no version has no line to lose",
			current:   "29-cli",
			available: []string{"30-cli-dev", "31-alpine3.24"},
		},
		{
			name:      "a bare version is not a variant line",
			current:   "3.21",
			available: []string{"3.23", "3.24"},
		},
		{
			name:      "a tag with no version at all is compared to nothing",
			current:   "latest",
			available: golang,
		},
		{
			name:      "an older family is no successor",
			current:   "3.13-alpine3.24",
			available: []string{"3.14-alpine3.21", "3.14-alpine3.23"},
		},
		{
			name:      "the newest family wins whatever order the registry listed",
			current:   "3.13-alpine3.21",
			available: []string{"3.14-alpine3.24", "3.13-alpine3.23", "3.14-alpine3.22"},
			want:      "-alpine3.24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupersedingVariant(tt.current, tt.available); got != tt.want {
				t.Errorf("SupersedingVariant(%q, %v) = %q, want %q", tt.current, tt.available, got, tt.want)
			}
		})
	}
}
