package presets

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// End of support for the base an image is built on
// (docs/core-concepts/security.md, issue #303).
//
// A CVE scan answers "what is broken in this image today". It does not answer
// "will anything ever fix it". Once the distribution underneath an image stops
// being supported, its packages receive no further security updates: the
// findings the scanners report on it are permanent, however fresh the tag is
// and however diligently the cooldown promotes it.
//
// This is the third signal in the same family as `missing` (#245, a pinned
// reference deleted upstream) and `frozen_variant` (#252, a variant line the
// publisher stopped building). All three describe an image that still pulls and
// still scans, and will never improve again. This one is the widest of the
// three, because it is a property of the base rather than of the tag: a
// perfectly current `probatum:0.2.1` sits on Alpine 3.20, whose support ended
// on 2026-04-01, and nothing in the tag, the digest or the finding counts says
// so.
//
// The input costs nothing extra. Trivy already reports the base of every image
// it scans, in `Metadata.OS` — `{"Family": "alpine", "Name": "3.20.10"}` — and
// the audit already scans every catalogue image daily. That field was being
// discarded.
//
// What this file does *not* do is track lifecycles. It maps an OS family onto
// an endoflife.date product, matches a version against the release lines that
// product publishes, and subtracts two dates.

// BaseEOLWarningDays is how far ahead the end of support is worth reporting.
//
// The two ends of the range are the argument. An end of support two years out
// is a fact about the calendar: nothing is decided by knowing it, and reporting
// it every day trains the reader to skip the section. Two months out is too
// late to be comfortable — getting off an abandoned base is a repin by hand
// (the `frozen_variant` escapes in #252 moved a base version, a variant line
// and a language version at once), it needs the replacement to exist, to work,
// and to be scanned before it is trusted.
//
// A quarter is the smallest window that still leaves room for that work to be
// scheduled rather than rushed. It also survives the slowest loop that could
// act on it: `container-monitor.yml` runs weekly, so 90 days is roughly
// thirteen chances to notice before the date, and `security-audit.yml` — where
// this is actually reported — runs daily.
const BaseEOLWarningDays = 90

// The states one image's base can be in. They are spelled the way
// `cidx preset scan-targets` spells its own policy reasons (`frozen_variant`,
// `newer_version`), because a reader meeting them in a workflow summary should
// not have to notice they came from a different command.
const (
	// BaseSupported is the ordinary case: the base is on a release line that is
	// still receiving updates, and its end of support is further away than
	// BaseEOLWarningDays.
	BaseSupported = "supported"

	// BaseEndingSoon is the actionable one. The base is still supported, and
	// the date it stops is close enough to plan the repin now.
	BaseEndingSoon = "approaching_eol"

	// BaseEnded is the state this signal exists to make visible: the base
	// stopped being supported on a date that has already passed. Every finding
	// on that image is permanent until the image leaves the base, and no amount
	// of promotion will change it.
	BaseEnded = "eol"

	// BaseUnknown is fail-closed, and it is a bug report rather than a
	// vulnerability: nothing here could say whether the base is supported.
	// Either the OS family is not one this file maps to an endoflife.date
	// product, or the version matches no release line that product publishes,
	// or the line it matches carries no end-of-support date.
	//
	// The alternative — treating an unrecognised family as "nothing to worry
	// about" — is the failure mode the whole supply-chain policy is written
	// against: an unresolvable digest, an undatable candidate and an unreadable
	// scan all refuse rather than assume, and so does this.
	BaseUnknown = "unknown_base"

	// BaseUnchecked is the degraded case, and the one that must never break
	// anything: endoflife.date did not answer. The image was scanned, its base
	// is known, and only the calendar is missing. An outage on somebody else's
	// API is not a reason to fail a scan or a monitor run — it is a reason to
	// say the check did not happen.
	BaseUnchecked = "unchecked"

	// BaseNone is an image with no distribution underneath it at all: a scratch
	// or static build, which Trivy reports with no `Metadata.OS`. Three
	// catalogue images are in this state (kaniko, ruff, shellcheck), and it is
	// an answer rather than a gap — a base that does not exist cannot stop
	// being supported.
	BaseNone = "no_base"
)

// BaseOS is the distribution an image is built on, as Trivy reports it in
// `Metadata.OS`. An image with neither field set has no distribution base.
type BaseOS struct {
	Family  string // Trivy's spelling: "alpine", "debian", "fedora"
	Version string // the full version: "3.20.10", "13.6", "44"
}

// String renders the base the way the reports print it.
func (b BaseOS) String() string {
	if b.Family == "" {
		return ""
	}
	if b.Version == "" {
		return b.Family
	}
	return b.Family + " " + b.Version
}

// eolProducts maps the OS family Trivy reports onto the product endoflife.date
// files its release cycles under.
//
// It holds exactly the families the catalogue actually runs, measured rather
// than guessed: alpine (13 images), debian (4) and fedora (1). Adding a family
// nothing runs would be a claim nobody has checked; a family that turns up
// later lands in BaseUnknown, which is loud, and adding it is one line.
//
// Only alpine needs translating — endoflife.date files it under
// `alpine-linux`, and `/api/alpine.json` is a redirect rather than a document.
// The other two match their family name, which is why the map holds them
// explicitly instead of falling back to the family: an identity default would
// make every unmapped family look like a valid product and turn the
// fail-closed path into a 404 at best, and a wrong answer at worst.
var eolProducts = map[string]string{
	"alpine": "alpine-linux",
	"debian": "debian",
	"fedora": "fedora",
}

// EOLProduct names the endoflife.date product for an OS family, and reports
// whether one is mapped at all.
func EOLProduct(family string) (string, bool) {
	product, ok := eolProducts[strings.ToLower(strings.TrimSpace(family))]
	return product, ok
}

// EOLCycle is one release line as endoflife.date describes it. Only the two
// fields this signal reads are decoded; the rest of the document is ignored on
// purpose, so a new field upstream cannot break the parse.
type EOLCycle struct {
	// Cycle is the release line: "3.20", "13", "44".
	Cycle string `json:"cycle"`

	// EOL is a date string ("2026-04-01"), or a boolean. endoflife.date uses
	// `false` for a line with no announced end of support and `true` for one
	// that has ended without a published date, so it cannot be decoded as a
	// string.
	EOL json.RawMessage `json:"eol"`
}

// EndOfSupport reads the end-of-support date of a release line.
//
// `dated` is false when there is no date to read: an unannounced end
// (`false`), an undated end (`true`), or a value neither of those shapes. All
// three are the same to a caller that has to subtract two dates, and all three
// land in BaseUnknown rather than in "supported" — the calendar is the whole
// evidence here, and without it there is no verdict to give.
func (c EOLCycle) EndOfSupport() (date time.Time, dated bool) {
	var text string
	if err := json.Unmarshal(c.EOL, &text); err != nil {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

// MatchCycle finds the release line a version belongs to.
//
// A version is in a cycle when it is that cycle, or when the cycle is a
// component-wise prefix of it: Alpine's `3.20.10` belongs to `3.20`, Debian's
// `13.6` to `13`, Fedora's `44` to `44`. The dot boundary is what makes it
// component-wise, and it matters — a plain string prefix would file `3.20.10`
// under `3.2`, a line that ended in 2015.
//
// The longest match wins, so a product listing both `3` and `3.20` answers
// with the more specific line.
func MatchCycle(cycles []EOLCycle, version string) (EOLCycle, bool) {
	version = strings.TrimSpace(version)
	if version == "" {
		return EOLCycle{}, false
	}

	var (
		match EOLCycle
		best  = -1
	)
	for _, cycle := range cycles {
		if !versionInCycle(version, cycle.Cycle) {
			continue
		}
		if len(cycle.Cycle) > best {
			match, best = cycle, len(cycle.Cycle)
		}
	}
	return match, best >= 0
}

// versionInCycle reports whether version belongs to cycle, comparing whole
// dot-separated components.
func versionInCycle(version, cycle string) bool {
	if cycle == "" {
		return false
	}
	return version == cycle || strings.HasPrefix(version, cycle+".")
}

// BaseSupport is what is known about the support window of one image's base:
// the base itself, the release line it belongs to, when that line stops being
// updated, and how long that leaves.
type BaseSupport struct {
	// Base is the distribution the scan reported.
	Base BaseOS

	// Product and Cycle are what the base resolved to on endoflife.date. Both
	// are empty when it resolved to nothing.
	Product string
	Cycle   string

	// EOL is the date support ends, zero when no date is known.
	EOL time.Time

	// DaysLeft is how many days remain until EOL, negative once the date has
	// passed. Meaningless unless EOL is set.
	DaysLeft int

	// State is one of the constants above.
	State string

	// Reason is one sentence a human can act on, and the only thing the
	// workflow annotations print beyond the image name.
	Reason string
}

// Dated reports whether an end-of-support date was actually established.
func (s BaseSupport) Dated() bool { return !s.EOL.IsZero() }

// NeedsAttention is the set of states worth putting in front of somebody: a
// base that has stopped being supported, one about to, and one nothing could
// resolve. BaseUnchecked is deliberately not in it — an endoflife.date outage
// is reported, but it is not the catalogue's problem to act on.
func (s BaseSupport) NeedsAttention() bool {
	switch s.State {
	case BaseEnded, BaseEndingSoon, BaseUnknown:
		return true
	}
	return false
}

// ClassifyBase decides what the cycles say about one image's base.
//
// It is pure: the caller fetches the cycle list and hands it over, so the
// decision is testable without a network, and an endoflife.date outage is a
// case the caller reports with UncheckedBase rather than a failure mode buried
// in here.
func ClassifyBase(base BaseOS, cycles []EOLCycle, now time.Time) BaseSupport {
	support := BaseSupport{Base: base}

	if base.Family == "" {
		support.State = BaseNone
		support.Reason = "the image has no distribution base — nothing to lose support"
		return support
	}

	product, mapped := EOLProduct(base.Family)
	if !mapped {
		support.State = BaseUnknown
		support.Reason = fmt.Sprintf(
			"no endoflife.date product is mapped for the OS family %q — the base cannot be checked, so it is reported rather than assumed supported (add it to eolProducts)",
			base.Family)
		return support
	}
	support.Product = product

	cycle, found := MatchCycle(cycles, base.Version)
	if !found {
		support.State = BaseUnknown
		support.Reason = fmt.Sprintf(
			"endoflife.date publishes no release line matching %s %s under %q — the base cannot be checked",
			base.Family, base.Version, product)
		return support
	}
	support.Cycle = cycle.Cycle

	eol, dated := cycle.EndOfSupport()
	if !dated {
		support.State = BaseUnknown
		support.Reason = fmt.Sprintf(
			"endoflife.date announces no end-of-support date for %s %s — the base cannot be checked",
			base.Family, cycle.Cycle)
		return support
	}

	support.EOL = eol
	support.DaysLeft = daysBetween(now, eol)

	switch {
	case support.DaysLeft < 0:
		support.State = BaseEnded
		support.Reason = fmt.Sprintf(
			"%s %s stopped being supported on %s, %d days ago — its findings will never be fixed, whatever the tag says",
			base.Family, cycle.Cycle, eol.Format(time.DateOnly), -support.DaysLeft)
	case support.DaysLeft <= BaseEOLWarningDays:
		support.State = BaseEndingSoon
		support.Reason = fmt.Sprintf(
			"%s %s stops being supported on %s, in %d days",
			base.Family, cycle.Cycle, eol.Format(time.DateOnly), support.DaysLeft)
	default:
		support.State = BaseSupported
		support.Reason = fmt.Sprintf(
			"%s %s is supported until %s, in %d days",
			base.Family, cycle.Cycle, eol.Format(time.DateOnly), support.DaysLeft)
	}
	return support
}

// UncheckedBase is the verdict when endoflife.date could not be reached.
//
// It exists so the outage is a stated result rather than an error travelling up
// the stack: the scan happened, the base is known, and only the calendar is
// missing. Nothing downstream fails on it.
func UncheckedBase(base BaseOS, err error) BaseSupport {
	product, _ := EOLProduct(base.Family)
	return BaseSupport{
		Base:    base,
		Product: product,
		State:   BaseUnchecked,
		Reason:  fmt.Sprintf("end of support not checked: %v", err),
	}
}

// daysBetween counts whole days from one instant to another, on calendar days
// rather than on 24-hour periods: an end of support is a date, and "3 days
// left" must not become 2 because the run happens in the afternoon.
func daysBetween(from, to time.Time) int {
	fromDay := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toDay := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toDay.Sub(fromDay).Hours() / 24)
}
