package presets

import (
	"sort"
	"strings"
)

// Triaging a scanner finding (docs/core-concepts/security.md, issue #238).
//
// A CIDX container lives for seconds, listens on nothing and persists nothing,
// so the threat it defends against is a compromised image — not a parser flaw in
// a library nothing calls. Without that, every finding reads as equally urgent
// and the only available answer is "patch it", which is how a catalogue ends up
// churning on findings nobody could have exploited.
//
// The policy asks four questions in order. The two this file answers
// mechanically are the second and the third:
//
//	2. Is a fix available?  A fix upstream means the finding disappears when the
//	   publisher republishes. That is a question about the image's age, not about
//	   the vulnerability, and an exception must never be written for one.
//	3. Is it reachable here? Two classes are unreachable by construction and are
//	   exempt as classes rather than case by case.
//
// The first (KEV, EPSS) and the fourth (what removal costs) are judgement, and
// stay with the human. The data is carried through where a scanner reports it,
// and nothing here turns it into a threshold.

// Finding is one HIGH/CRITICAL result, reduced to the fields the triage reads.
//
// Both scanners are parsed into this shape: they disagree on spelling
// (`gobinary` vs `go-module`, `debian` vs `deb`) and on coverage — only Trivy
// reports the kernel headers package, only Grype reports EPSS — so the triage
// has to see them side by side rather than one at a time.
type Finding struct {
	// ID is the identifier the scanner reported: a CVE, or a GHSA where the
	// scanner knows the advisory but not the CVE it maps to.
	ID string

	// Severity is HIGH or CRITICAL; the callers filter before constructing one.
	Severity string

	// Package names the installed package the finding is against, as the
	// scanner spells it.
	Package string

	// PackageType is the ecosystem the package comes from — `gobinary` /
	// `go-module` for a Go binary's embedded module list, `debian` / `deb` /
	// `apk` / `rpm` for an OS package.
	PackageType string

	// FixedIn is the version that fixes it, empty when no fix exists at any
	// version. Trivy calls it `FixedVersion`, Grype `fix.versions`.
	FixedIn string

	// EPSS is the probability of exploitation in the next 30 days, 0 when the
	// scanner reported none. Grype carries it, Trivy does not. It is reported,
	// never thresholded: it tells a human where to look first.
	EPSS float64

	// KEV records that CISA lists the vulnerability as actively exploited.
	// Measured zero on this catalogue, which is exactly why the field is
	// reported rather than acted on automatically.
	KEV bool

	// Scanner names the tool that reported this instance — "Trivy" or "Grype".
	//
	// The triage merges both and counts a vulnerability once, which is right for
	// a count and lossy for a reader: on this catalogue Grype alone sees 102 of
	// the 150 findings needing triage, and a reader deciding what to do about one
	// wants to know whether the other scanner corroborated it. Nothing here
	// weighs the answer; it travels through to the report.
	Scanner string
}

// Fixed reports whether a fix exists upstream — question 2 of the policy.
//
// A fixable finding is a statement about the image's age, and the answer is to
// republish or repin, never to accept. `cidx security vuln add` refuses to
// record an exception for one.
func (f Finding) Fixed() bool { return f.FixedIn != "" }

// The two classes the policy exempts by construction, named once here because
// the reports, the tests and the documentation share the vocabulary.
const (
	// ExemptGoStdlib is the Go standard library compiled into a CLI binary. It
	// vanishes when the publisher recompiles — no action exists at our end — and
	// the paths flagged (`net/http`, `crypto/*`) are unreachable in a tool that
	// opens no listener.
	ExemptGoStdlib = "go-stdlib"

	// ExemptKernelHeaders is the kernel headers package. The kernel is the
	// host's and is not in the container; the scanner flags the headers because
	// they carry a version string.
	ExemptKernelHeaders = "kernel-headers"
)

// goPackageTypes are the two spellings of "this came out of a compiled Go
// binary's module list": Trivy's `gobinary`, Grype's `go-module`.
var goPackageTypes = map[string]bool{"gobinary": true, "go-module": true}

// kernelHeaderPackages are the package names that are kernel headers and
// nothing else: Debian's `linux-libc-dev` and Alpine's `linux-headers`. The
// match is on the exact name, deliberately — a prefix match on `linux` would
// swallow `linux-pam` and `util-linux`, which are real packages doing real
// work in the container.
var kernelHeaderPackages = map[string]bool{"linux-libc-dev": true, "linux-headers": true}

// Exempt names the class that makes this finding unreachable here, or "" when
// none does.
//
// The two tests are deliberately narrow, because an exemption that is too broad
// hides findings that matter:
//
//   - Go stdlib: the package must be named exactly `stdlib` *and* come from a
//     Go binary. Every other Go module stays in the queue — the runc and
//     containerd findings on the Ansible image are `gobinary` too, and they are
//     the real ones.
//   - Kernel headers: the package name must be exactly `linux-libc-dev` or
//     `linux-headers`. No type test, because those names belong to no other
//     ecosystem.
func (f Finding) Exempt() string {
	switch {
	case goPackageTypes[strings.ToLower(f.PackageType)] && f.Package == "stdlib":
		return ExemptGoStdlib
	case kernelHeaderPackages[f.Package]:
		return ExemptKernelHeaders
	}
	return ""
}

// Triage is what a set of findings splits into. The four counts partition the
// set: Fixable + GoStdlib + KernelHeaders + Actionable == Carried.
type Triage struct {
	// Carried is every distinct HIGH/CRITICAL finding the scanners reported.
	// It is the number the baseline was missing: publishing only Accepted is
	// how the file came to read "0 accepted findings" on a catalogue carrying
	// 596.
	Carried int

	// Fixable is fixed upstream: image freshness, never exception territory.
	Fixable int

	// GoStdlib and KernelHeaders are the two exempt classes.
	GoStdlib      int
	KernelHeaders int

	// Actionable is what is left: no fix at any version, and reachable enough
	// to be worth an argument. The only population an exception is the right
	// instrument for.
	Actionable int

	// KEV names the findings CISA lists as actively exploited, and TopEPSS is
	// the highest exploitation probability seen. Both are reported for a human
	// to read; neither gates anything.
	KEV     []string
	TopEPSS float64
}

// Summarise splits the findings on one image into the four populations.
//
// It is per image on purpose: the same CVE on five images is five things to
// look at, and collapsing them would understate what the catalogue carries.
// Call it once per image and Add the results.
//
// The class exemptions are labelled before fixability, which inverts the order
// the policy asks its questions in, and deliberately. The policy orders the
// questions by what to *do* — a fix exists, so wait, and stop asking. The split
// here labels findings by *why they are out of the queue*, and the class is the
// stabler answer: a Go stdlib finding is unreachable whether or not the Go team
// has shipped the fix yet, and most of them have one, so labelling by fix first
// would report zero stdlib findings on a catalogue full of them. Actionable is
// the same number either way, which is the number that matters.
//
// Findings are grouped by identifier first, because both scanners report the
// same vulnerability and it must be counted once. Within a group:
//
//   - exempt only if *every* instance is exempt under the same class — Trivy
//     reporting a CVE against `linux-libc-dev` while Grype reports the same CVE
//     against `openssl` is a real finding, and the narrower reading is the safe
//     one;
//   - fixable if *any* scanner found a fix — one of them knowing about it is
//     enough for it to exist.
func Summarise(findings []Finding) Triage {
	order, byID := groupByID(findings)

	var triage Triage
	for _, id := range order {
		group := byID[id]
		triage.Carried++

		for _, f := range group {
			if f.EPSS > triage.TopEPSS {
				triage.TopEPSS = f.EPSS
			}
			if f.KEV {
				triage.KEV = append(triage.KEV, group[0].ID)
				break
			}
		}

		switch classOf(group) {
		case ExemptGoStdlib:
			triage.GoStdlib++
		case ExemptKernelHeaders:
			triage.KernelHeaders++
		case classFixable:
			triage.Fixable++
		default:
			triage.Actionable++
		}
	}

	sort.Strings(triage.KEV)
	return triage
}

// Actionable returns the findings Summarise counts under Actionable, grouped by
// identifier and in the order the scanners reported them.
//
// It exists so a report can name what needs a human rather than only count it,
// and it shares Summarise's grouping and classification on purpose: a view
// listing a different population from the one the baseline counts would be a
// second opinion, not a view of the same thing.
func Actionable(findings []Finding) [][]Finding {
	order, byID := groupByID(findings)

	var groups [][]Finding
	for _, id := range order {
		if group := byID[id]; classOf(group) == "" {
			groups = append(groups, group)
		}
	}
	return groups
}

// groupByID collects the instances of each vulnerability, keyed case-insensitively
// because the two scanners do not agree on the case of a GHSA identifier, and
// returns the keys in the order they were first seen so the output is stable.
func groupByID(findings []Finding) ([]string, map[string][]Finding) {
	byID := make(map[string][]Finding)
	order := make([]string, 0, len(findings))
	for _, f := range findings {
		key := strings.ToUpper(f.ID)
		if _, seen := byID[key]; !seen {
			order = append(order, key)
		}
		byID[key] = append(byID[key], f)
	}
	return order, byID
}

// classFixable labels a group that has a fix upstream. Unlike the two exempt
// classes it is not a property of the vulnerability but of the image's age, so
// it is internal to the classification rather than part of the vocabulary.
const classFixable = "fixed-upstream"

// classOf names why a group is out of the triage queue, or "" when it is still
// in it — the Actionable population.
//
// The order matters and is the one Summarise documents: class before fixability,
// because a Go stdlib finding is unreachable whether or not the fix has shipped.
func classOf(group []Finding) string {
	switch {
	case allExempt(group, ExemptGoStdlib):
		return ExemptGoStdlib
	case allExempt(group, ExemptKernelHeaders):
		return ExemptKernelHeaders
	case anyFixed(group):
		return classFixable
	}
	return ""
}

// Add accumulates another image's split into this one.
//
// Nothing is deduplicated across images: a CVE on five of them is five findings
// to look at, five images to repin, and the number the baseline publishes has to
// say so.
func (t *Triage) Add(other Triage) {
	t.Carried += other.Carried
	t.Fixable += other.Fixable
	t.GoStdlib += other.GoStdlib
	t.KernelHeaders += other.KernelHeaders
	t.Actionable += other.Actionable

	t.KEV = append(t.KEV, other.KEV...)
	sort.Strings(t.KEV)
	t.KEV = compactStrings(t.KEV)

	if other.TopEPSS > t.TopEPSS {
		t.TopEPSS = other.TopEPSS
	}
}

// compactStrings removes adjacent duplicates from a sorted slice. A KEV entry is
// worth naming once however many images carry it — unlike the counts, which are
// per image because each one is a separate repin.
func compactStrings(sorted []string) []string {
	out := sorted[:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

func anyFixed(group []Finding) bool {
	for _, f := range group {
		if f.Fixed() {
			return true
		}
	}
	return false
}

// allExempt reports whether every instance in the group is exempt under the
// same class. A group split across two classes stays actionable rather than
// being filed under whichever one came first.
func allExempt(group []Finding, class string) bool {
	for _, f := range group {
		if f.Exempt() != class {
			return false
		}
	}
	return len(group) > 0
}

// FindingIDs reduces findings to the identifiers the scan gate compares, which
// works on identity alone.
func FindingIDs(findings []Finding) []string {
	ids := make([]string, 0, len(findings))
	for _, f := range findings {
		ids = append(ids, f.ID)
	}
	return ids
}

// FixVersion returns the fix reported for cve among findings, or "".
//
// Used to answer "is this one of the ones an exception must never be written
// for?" about an identifier rather than about a scan result.
func FixVersion(findings []Finding, cve string) string {
	for _, f := range findings {
		if strings.EqualFold(f.ID, cve) && f.Fixed() {
			return f.FixedIn
		}
	}
	return ""
}
