package presets

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// The cycle lists below are the real shape of the endoflife.date documents, cut
// down to the fields this code reads. No test in this package performs a
// request: the cycles are always handed in.

func alpineCycles() []EOLCycle {
	return cyclesFrom(`[
		{"cycle":"3.24","eol":"2028-06-01"},
		{"cycle":"3.23","eol":"2027-11-01"},
		{"cycle":"3.22","eol":"2027-05-01"},
		{"cycle":"3.21","eol":"2026-11-01"},
		{"cycle":"3.20","eol":"2026-04-01"},
		{"cycle":"3.2","eol":"2017-05-01"}
	]`)
}

func debianCycles() []EOLCycle {
	return cyclesFrom(`[
		{"cycle":"13","eol":"2028-08-09"},
		{"cycle":"12","eol":"2026-07-11"},
		{"cycle":"11","eol":"2024-08-14"}
	]`)
}

func fedoraCycles() []EOLCycle {
	return cyclesFrom(`[
		{"cycle":"44","eol":"2027-06-02"},
		{"cycle":"43","eol":"2026-12-09"}
	]`)
}

func cyclesFrom(document string) []EOLCycle {
	var cycles []EOLCycle
	if err := json.Unmarshal([]byte(document), &cycles); err != nil {
		panic(err)
	}
	return cycles
}

func day(date string) time.Time {
	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		panic(err)
	}
	return parsed
}

// TestEOLProductMapsTheFamiliesTheCatalogueRuns pins the one translation that
// is not the identity. Trivy says `alpine`; endoflife.date files it under
// `alpine-linux`, and `/api/alpine.json` is a redirect rather than a document —
// reading the family straight through would have asked for the wrong URL.
func TestEOLProductMapsTheFamiliesTheCatalogueRuns(t *testing.T) {
	tests := []struct {
		family  string
		product string
		mapped  bool
	}{
		{"alpine", "alpine-linux", true},
		{"debian", "debian", true},
		{"fedora", "fedora", true},
		{"Alpine", "alpine-linux", true}, // Trivy's spelling is lower-case; do not depend on it
		{" debian ", "debian", true},

		// Everything else is fail-closed. `ubuntu` and `wolfi` are real
		// distributions no catalogue image runs, and an identity fallback would
		// have made the first look mapped and the second answer 404.
		{"ubuntu", "", false},
		{"wolfi", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		product, mapped := EOLProduct(tt.family)
		if mapped != tt.mapped || product != tt.product {
			t.Errorf("EOLProduct(%q) = (%q, %v), want (%q, %v)", tt.family, product, mapped, tt.product, tt.mapped)
		}
	}
}

// TestMatchCycleComparesWholeComponents: the dot boundary is the whole of this
// function. A plain string prefix files Alpine 3.20.10 under 3.2, a line that
// ended in 2017, and the report would call a supported base long dead.
func TestMatchCycleComparesWholeComponents(t *testing.T) {
	tests := []struct {
		name    string
		cycles  []EOLCycle
		version string
		cycle   string
		found   bool
	}{
		{"alpine patch version lands on its minor line", alpineCycles(), "3.20.10", "3.20", true},
		{"alpine 3.23.4 is 3.23, not 3.2", alpineCycles(), "3.23.4", "3.23", true},
		{"a version equal to its cycle matches", alpineCycles(), "3.24", "3.24", true},
		{"debian point release lands on its suite", debianCycles(), "13.6", "13", true},
		{"debian 12 is not debian 1", debianCycles(), "12.11", "12", true},
		{"fedora has no components to strip", fedoraCycles(), "44", "44", true},

		// The longest match wins, so a document listing both a major and a
		// minor line answers with the specific one.
		{"the most specific line wins", cyclesFrom(`[{"cycle":"3","eol":"2020-01-01"},{"cycle":"3.20","eol":"2026-04-01"}]`), "3.20.10", "3.20", true},

		{"a version no line covers matches nothing", alpineCycles(), "3.99.1", "", false},
		{"an empty version matches nothing", alpineCycles(), "", "", false},
		{"an empty document matches nothing", nil, "3.20.10", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle, found := MatchCycle(tt.cycles, tt.version)
			if found != tt.found || cycle.Cycle != tt.cycle {
				t.Errorf("MatchCycle(%q) = (%q, %v), want (%q, %v)", tt.version, cycle.Cycle, found, tt.cycle, tt.found)
			}
		})
	}
}

// TestClassifyBase walks every verdict the signal can reach, on the dates the
// catalogue actually sits on.
func TestClassifyBase(t *testing.T) {
	tests := []struct {
		name     string
		base     BaseOS
		cycles   []EOLCycle
		now      string
		state    string
		daysLeft int
	}{
		// Measured on the catalogue: probatum 0.2.1 is built on Alpine 3.20,
		// whose support ended on 2026-04-01. It pulls, it scans, and every
		// finding on it is permanent.
		{
			name: "a base whose support already ended", base: BaseOS{"alpine", "3.20.10"},
			cycles: alpineCycles(), now: "2026-08-02", state: BaseEnded, daysLeft: -123,
		},
		// Debian 12 died on 2026-07-11. Nothing in the catalogue runs it today,
		// which is exactly why it is the fabricated case: the verdict has to be
		// right before an image lands on it, not after.
		{
			name: "debian 12, three weeks past its date", base: BaseOS{"debian", "12.11"},
			cycles: debianCycles(), now: "2026-08-02", state: BaseEnded, daysLeft: -22,
		},
		{
			name: "the day support ends is not yet past", base: BaseOS{"alpine", "3.21.4"},
			cycles: alpineCycles(), now: "2026-11-01", state: BaseEndingSoon, daysLeft: 0,
		},
		{
			name: "the day after is past", base: BaseOS{"alpine", "3.21.4"},
			cycles: alpineCycles(), now: "2026-11-02", state: BaseEnded, daysLeft: -1,
		},
		{
			name: "exactly at the warning threshold", base: BaseOS{"alpine", "3.21.4"},
			cycles: alpineCycles(), now: "2026-08-03", state: BaseEndingSoon, daysLeft: BaseEOLWarningDays,
		},
		{
			name: "one day beyond the threshold is still quiet", base: BaseOS{"alpine", "3.21.4"},
			cycles: alpineCycles(), now: "2026-08-02", state: BaseSupported, daysLeft: BaseEOLWarningDays + 1,
		},
		{
			name: "the catalogue's fedora, a year out", base: BaseOS{"fedora", "44"},
			cycles: fedoraCycles(), now: "2026-08-02", state: BaseSupported, daysLeft: 304,
		},
		{
			name: "the catalogue's debian", base: BaseOS{"debian", "13.6"},
			cycles: debianCycles(), now: "2026-08-02", state: BaseSupported, daysLeft: 738,
		},

		// Fail-closed, three ways. None of them may read as "supported".
		{
			name: "an OS family nothing maps", base: BaseOS{"ubuntu", "24.04"},
			cycles: nil, now: "2026-08-02", state: BaseUnknown,
		},
		{
			name: "a version no release line covers", base: BaseOS{"alpine", "3.99.0"},
			cycles: alpineCycles(), now: "2026-08-02", state: BaseUnknown,
		},
		{
			name: "a release line with no announced date", base: BaseOS{"debian", "14.0"},
			cycles: cyclesFrom(`[{"cycle":"14","eol":false}]`), now: "2026-08-02", state: BaseUnknown,
		},
		{
			name: "a release line ended without a date", base: BaseOS{"debian", "9.13"},
			cycles: cyclesFrom(`[{"cycle":"9","eol":true}]`), now: "2026-08-02", state: BaseUnknown,
		},

		// An image with no distribution underneath it: kaniko, ruff and
		// shellcheck are all in this state. It is an answer, not a gap.
		{
			name: "an image built on scratch", base: BaseOS{},
			cycles: alpineCycles(), now: "2026-08-02", state: BaseNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyBase(tt.base, tt.cycles, day(tt.now))

			if got.State != tt.state {
				t.Fatalf("ClassifyBase(%v) state = %q, want %q (reason: %s)", tt.base, got.State, tt.state, got.Reason)
			}
			if got.Dated() && got.DaysLeft != tt.daysLeft {
				t.Errorf("ClassifyBase(%v) days left = %d, want %d", tt.base, got.DaysLeft, tt.daysLeft)
			}
			if got.Reason == "" {
				t.Errorf("ClassifyBase(%v) gave no reason to act on", tt.base)
			}
		})
	}
}

// TestClassifyBaseNeverReportsAnUnknownBaseAsSupported is the posture the whole
// supply-chain policy takes — an unresolvable digest, an undatable candidate
// and an unreadable scan all refuse rather than assume. A family this code does
// not know is a gap in this code, and silence would hide it for ever.
func TestClassifyBaseNeverReportsAnUnknownBaseAsSupported(t *testing.T) {
	for _, base := range []BaseOS{
		{"ubuntu", "24.04"},
		{"wolfi", "20230201"},
		{"photon", "5.0"},
		{"alpine", "3.99.0"},
	} {
		got := ClassifyBase(base, alpineCycles(), day("2026-08-02"))

		if got.State == BaseSupported {
			t.Errorf("ClassifyBase(%v) reported an unresolvable base as supported", base)
		}
		if !got.NeedsAttention() {
			t.Errorf("ClassifyBase(%v) state %q does not ask for attention", base, got.State)
		}
	}
}

// TestUncheckedBaseNeverAsksForAction: endoflife.date being down says nothing
// about the catalogue. It is reported, and that is all — never an error, never
// a state anything gates on.
func TestUncheckedBaseNeverAsksForAction(t *testing.T) {
	got := UncheckedBase(BaseOS{"alpine", "3.20.10"}, errors.New("dial tcp: i/o timeout"))

	if got.State != BaseUnchecked {
		t.Errorf("UncheckedBase state = %q, want %q", got.State, BaseUnchecked)
	}
	if got.NeedsAttention() {
		t.Error("an endoflife.date outage should not ask the catalogue for anything")
	}
	if got.Dated() {
		t.Error("an unchecked base cannot carry an end-of-support date")
	}
	if got.Product != "alpine-linux" {
		t.Errorf("UncheckedBase product = %q, want the product it failed to reach", got.Product)
	}
	if !strings.Contains(got.Reason, "i/o timeout") {
		t.Errorf("UncheckedBase reason %q does not say what went wrong", got.Reason)
	}
}

// TestEndOfSupportReadsBothShapesEndoflifeDatePublishes: `eol` is a date string
// on a dated line and a boolean otherwise, so it cannot be decoded as a string.
func TestEndOfSupportReadsBothShapesEndoflifeDatePublishes(t *testing.T) {
	tests := []struct {
		document string
		dated    bool
		date     string
	}{
		{`{"cycle":"3.20","eol":"2026-04-01"}`, true, "2026-04-01"},
		{`{"cycle":"14","eol":false}`, false, ""},
		{`{"cycle":"9","eol":true}`, false, ""},
		{`{"cycle":"9"}`, false, ""},
		{`{"cycle":"9","eol":"not a date"}`, false, ""},
	}

	for _, tt := range tests {
		var cycle EOLCycle
		if err := json.Unmarshal([]byte(tt.document), &cycle); err != nil {
			t.Fatalf("could not decode %s: %v", tt.document, err)
		}

		date, dated := cycle.EndOfSupport()
		if dated != tt.dated {
			t.Errorf("%s: dated = %v, want %v", tt.document, dated, tt.dated)
		}
		if dated && date.Format(time.DateOnly) != tt.date {
			t.Errorf("%s: date = %s, want %s", tt.document, date.Format(time.DateOnly), tt.date)
		}
	}
}

// TestDaysLeftCountsCalendarDays: an end of support is a date, and "3 days
// left" must not read as 2 because the run happened in the afternoon.
func TestDaysLeftCountsCalendarDays(t *testing.T) {
	afternoon := time.Date(2026, 8, 2, 17, 30, 0, 0, time.UTC)

	got := ClassifyBase(BaseOS{"alpine", "3.21.4"}, alpineCycles(), afternoon)

	if got.DaysLeft != BaseEOLWarningDays+1 {
		t.Errorf("days left = %d at 17:30, want %d — the hour must not shorten the count",
			got.DaysLeft, BaseEOLWarningDays+1)
	}
}
