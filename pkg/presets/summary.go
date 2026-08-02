package presets

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Where the catalogue stands, in one page
// (docs/core-concepts/security.md, issue #308).
//
// Two views already exist. The Security tab (#301) is the detail: one alert per
// finding, dated, closed by itself when a repin removes it — and 169 of them.
// The end-of-support signal (#305) is one fact about one image. Neither answers
// the question that comes first: how much is carried, how much of it needs a
// human, and what has already lapsed.
//
// This renders that page, and nothing here computes a finding, a date or a
// verdict. [Summarise] has already split the findings, [ClassifyBase] has
// already judged the bases, and known-vulnerabilities.toml already carries the
// expiry dates. What is new is the assembly, which is why it is a hundred lines
// of formatting rather than a second opinion — a page disagreeing with the tab
// about what the catalogue carries would be worse than no page at all.
//
// It lives here, next to the triage it reads, for the reason `sarif.go` does:
// the command is the I/O half, and the decisions are where they can be exercised
// without one.

// BaseNote is one image's base as the end-of-support check left it, reduced to
// what the page prints.
type BaseNote struct {
	Image  string
	State  string
	Reason string
}

// SummaryLinks are the repository-specific URLs the page points at.
//
// They are empty when the summary is rendered outside GitHub Actions, and the
// same text is then rendered without hyperlinks: a page that only worked in CI
// would be a page nobody could check before pushing it.
type SummaryLinks struct {
	// Repo is the repository's web URL, e.g. https://github.com/cidx-org/cidx.
	Repo string

	// Run is the workflow run that produced this body, so a reader can reach
	// the evidence the numbers came from.
	Run string
}

// CatalogueSummary is the state of the built-in catalogue, in the numbers a
// maintainer decides on.
type CatalogueSummary struct {
	// Images is how many distinct images the catalogue ships, and Unscanned
	// names the ones no scanner answered for. An absent number is not a zero.
	Images    int
	Unscanned []string

	// Triage is the partition [Summarise] produces, summed per image.
	Triage Triage

	// Accepted is how many HIGH/CRITICAL acceptances cover a repository the
	// catalogue runs today, and Expired the subset past its date.
	Accepted int
	Expired  []Exception

	// Bases holds the images whose base needs attention or could not be
	// checked. A supported base is deliberately absent: it is a fact about the
	// calendar and printing it daily teaches the reader to skip the section.
	Bases []BaseNote

	// Day is the date the page was written, in UTC. Unlike SECURITY-BASELINE.md
	// this body is not committed — it is overwritten every run — so a date here
	// is the only thing saying whether what you are reading is today's.
	Day time.Time

	Links SummaryLinks
}

// Scanned is how many images produced a result.
func (s CatalogueSummary) Scanned() int { return s.Images - len(s.Unscanned) }

// basesIn returns the images in one end-of-support state, in a fixed order.
func (s CatalogueSummary) basesIn(state string) []BaseNote {
	var out []BaseNote
	for _, note := range s.Bases {
		if note.State == state {
			out = append(out, note)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Image < out[j].Image })
	return out
}

// Waiting reports whether anything on the page needs a human.
//
// The three populations are of different kinds — a finding nobody has judged, an
// acceptance nobody has stood behind since its date, a base that will never be
// fixed again — so they are never added together. This only asks whether all
// three are empty, which is the one thing worth saying in a headline.
func (s CatalogueSummary) Waiting() bool {
	return s.Triage.Actionable > 0 ||
		len(s.Expired) > 0 ||
		len(s.basesIn(BaseEnded)) > 0 ||
		len(s.basesIn(BaseEndingSoon)) > 0 ||
		len(s.basesIn(BaseUnknown)) > 0
}

// SummaryDigest is the same page in flat keys.
//
// The body is markdown because a human reads it, and markdown is what an agent
// then has to guess at: the wording will change, and a script keyed on a
// sentence breaks the first time it does. The keys here do not move. It costs
// one struct and one Marshal, which is the whole argument for carrying both.
//
// Counts only. The findings themselves are in code scanning, with an API of
// their own; repeating 169 of them here would rebuild the list this page exists
// not to be.
type SummaryDigest struct {
	Day       string   `json:"day"`
	Images    int      `json:"images"`
	Scanned   int      `json:"scanned"`
	Unscanned []string `json:"unscanned"`

	Carried             int `json:"carried"`
	NeedingTriage       int `json:"needing_triage"`
	FixedUpstream       int `json:"fixed_upstream"`
	ExemptGoStdlib      int `json:"exempt_go_stdlib"`
	ExemptKernelHeaders int `json:"exempt_kernel_headers"`

	KEV     []string `json:"kev"`
	TopEPSS float64  `json:"top_epss"`

	Accepted           int `json:"accepted"`
	ExpiredAcceptances int `json:"expired_acceptances"`

	BasesEnded      int `json:"bases_eol"`
	BasesEndingSoon int `json:"bases_ending_soon"`
	BasesUnknown    int `json:"bases_unknown"`
	BasesUnchecked  int `json:"bases_unchecked"`
}

// Digest reduces the page to the counts a machine reads.
func (s CatalogueSummary) Digest() SummaryDigest {
	return SummaryDigest{
		Day:                 s.Day.UTC().Format(time.DateOnly),
		Images:              s.Images,
		Scanned:             s.Scanned(),
		Unscanned:           orEmpty(s.Unscanned),
		Carried:             s.Triage.Carried,
		NeedingTriage:       s.Triage.Actionable,
		FixedUpstream:       s.Triage.Fixable,
		ExemptGoStdlib:      s.Triage.GoStdlib,
		ExemptKernelHeaders: s.Triage.KernelHeaders,
		KEV:                 orEmpty(s.Triage.KEV),
		TopEPSS:             s.Triage.TopEPSS,
		Accepted:            s.Accepted,
		ExpiredAcceptances:  len(s.Expired),
		BasesEnded:          len(s.basesIn(BaseEnded)),
		BasesEndingSoon:     len(s.basesIn(BaseEndingSoon)),
		BasesUnknown:        len(s.basesIn(BaseUnknown)),
		BasesUnchecked:      len(s.basesIn(BaseUnchecked)),
	}
}

// orEmpty turns a nil slice into an empty one, so the JSON carries `[]` rather
// than `null` — a consumer testing the length should not have to handle both.
func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// RenderSummary writes the page.
//
// The order is the reader's: what is waiting first, because that is the question
// being asked; then what the catalogue carries, so the first number can be
// believed; then what is going away on its own; then where to look for the
// detail this page deliberately does not hold.
func RenderSummary(s CatalogueSummary) string {
	var sb strings.Builder

	sb.WriteString("# Vulnerability status\n\n")
	sb.WriteString("<!-- Rewritten by security-audit.yml on every run. Editing this body is pointless: the next audit overwrites it. -->\n\n")

	writeSummaryHeadline(&sb, s)
	writeSummaryWaiting(&sb, s)
	writeSummaryCarried(&sb, s)
	writeSummaryAutomatic(&sb, s)
	writeSummaryWhereToLook(&sb, s)
	writeSummaryDigest(&sb, s)

	fmt.Fprintf(&sb, "\n---\n\n_%s, %s._\n",
		link("Rewritten by the Security Audit", s.Links.Run),
		s.Day.UTC().Format(time.DateOnly))
	return sb.String()
}

func writeSummaryHeadline(sb *strings.Builder, s CatalogueSummary) {
	if !s.Waiting() {
		sb.WriteString("**Nothing is waiting for a decision.** ")
	} else {
		var parts []string
		if n := s.Triage.Actionable; n > 0 {
			parts = append(parts, fmt.Sprintf("%s need a judgement", plural(n, "finding", "findings")))
		}
		if n := len(s.Expired); n > 0 {
			parts = append(parts, fmt.Sprintf("%s past their date", plural(n, "acceptance is", "acceptances are")))
		}
		if n := len(s.basesIn(BaseEnded)); n > 0 {
			parts = append(parts, fmt.Sprintf("%s no longer supported", plural(n, "base is", "bases are")))
		}
		if n := len(s.basesIn(BaseEndingSoon)); n > 0 {
			parts = append(parts, fmt.Sprintf("%s support within %d days", plural(n, "base loses", "bases lose"), BaseEOLWarningDays))
		}
		if n := len(s.basesIn(BaseUnknown)); n > 0 {
			parts = append(parts, fmt.Sprintf("%s could not be checked", plural(n, "base", "bases")))
		}
		fmt.Fprintf(sb, "**%s.** ", joinList(parts))
	}

	fmt.Fprintf(sb, "The catalogue ships %s, carrying %s.\n\n",
		plural(s.Images, "image", "images"),
		plural(s.Triage.Carried, "HIGH/CRITICAL finding", "HIGH/CRITICAL findings"))

	// An unscanned image is the one thing that makes every number above an
	// understatement, so it is said before them rather than in a footnote.
	if len(s.Unscanned) > 0 {
		fmt.Fprintf(sb, "> **%s produced no scan result.** They carry nothing on this page and are not thereby clean — the audit fails the leg that could not scan, and until it passes these numbers are a floor, not a count.\n>\n",
			plural(len(s.Unscanned), "image", "images"))
		for _, image := range s.Unscanned {
			fmt.Fprintf(sb, "> - `%s`\n", image)
		}
		sb.WriteString("\n")
	}
}

func writeSummaryWaiting(sb *strings.Builder, s CatalogueSummary) {
	sb.WriteString("## Waiting for a decision\n\n")

	if !s.Waiting() {
		fmt.Fprintf(sb, "No finding is unjudged, no acceptance is past its date, and every base with a known support window keeps it for more than %d days.\n\n", BaseEOLWarningDays)
		return
	}

	sb.WriteString("| What | How many | Where to act |\n")
	sb.WriteString("| ---- | -------- | ------------ |\n")
	fmt.Fprintf(sb, "| Findings with no fix at any version | %d | %s |\n",
		s.Triage.Actionable, link("the Security tab", securityTab(s.Links.Repo)))
	fmt.Fprintf(sb, "| Acceptances past their expiry date | %d | %s |\n",
		len(s.Expired), link("`"+ExceptionsFile+"`", blob(s.Links.Repo, ExceptionsFile)))
	fmt.Fprintf(sb, "| Bases no longer supported | %d | %s |\n",
		len(s.basesIn(BaseEnded)), link("`"+CatalogueFile+"`", blob(s.Links.Repo, CatalogueFile)))
	fmt.Fprintf(sb, "| Bases losing support within %d days | %d | idem |\n",
		BaseEOLWarningDays, len(s.basesIn(BaseEndingSoon)))
	if n := len(s.basesIn(BaseUnknown)); n > 0 {
		fmt.Fprintf(sb, "| Bases nothing could resolve | %d | `pkg/presets/eol.go` |\n", n)
	}
	sb.WriteString("\n")

	writeExpiredDetails(sb, s)
	writeBaseDetails(sb, s)
}

// writeExpiredDetails names the acceptances rather than only counting them.
//
// These are named where the findings are not, and the asymmetry is the point: a
// finding has an alert of its own in code scanning, dated and closing by itself.
// An expired acceptance has nothing — `cidx security vuln ignore` writes every
// entry into the scanners' ignore file whatever its date, so the finding it
// waives never reaches a scan result at all.
func writeExpiredDetails(sb *strings.Builder, s CatalogueSummary) {
	if len(s.Expired) == 0 {
		return
	}

	expired := append([]Exception(nil), s.Expired...)
	sort.Slice(expired, func(i, j int) bool {
		if expired[i].Repository != expired[j].Repository {
			return expired[i].Repository < expired[j].Repository
		}
		return expired[i].CVE < expired[j].CVE
	})

	sb.WriteString("<details>\n")
	fmt.Fprintf(sb, "<summary>The %s past their date</summary>\n\n", plural(len(expired), "acceptance", "acceptances"))
	sb.WriteString("An exception is written with a date so the acceptance is argued again rather than inherited. Past it, the entry waives nothing until somebody re-dates it or deletes it.\n\n")
	sb.WriteString("| CVE | Repository | Severity | Expired | Justification |\n")
	sb.WriteString("| --- | ---------- | -------- | ------- | ------------- |\n")
	for _, e := range expired {
		fmt.Fprintf(sb, "| %s | `%s` | %s | %s | %s |\n",
			strings.ToUpper(e.CVE), e.Repository, strings.ToUpper(e.Severity), e.Expires, cell(e.Notes))
	}
	sb.WriteString("\n</details>\n\n")
}

func writeBaseDetails(sb *strings.Builder, s CatalogueSummary) {
	notes := append(s.basesIn(BaseEnded), s.basesIn(BaseEndingSoon)...)
	notes = append(notes, s.basesIn(BaseUnknown)...)
	if len(notes) == 0 {
		return
	}

	sb.WriteString("<details>\n")
	fmt.Fprintf(sb, "<summary>The %s worth a repin decision</summary>\n\n", plural(len(notes), "base", "bases"))
	sb.WriteString("A base past its end of support receives no further updates, so every finding on that image is permanent however fresh the tag is. Moving off one is a repin by hand, never a promotion.\n\n")
	for _, note := range notes {
		fmt.Fprintf(sb, "- `%s` — %s\n", note.Image, note.Reason)
	}
	sb.WriteString("\n</details>\n\n")
}

func writeSummaryCarried(sb *strings.Builder, s CatalogueSummary) {
	sb.WriteString("## What the catalogue carries\n\n")

	if s.Scanned() == 0 {
		sb.WriteString("No scanner result was available, so what the images carry is not stated. An absent number is not a zero.\n\n")
		return
	}

	fmt.Fprintf(sb, "**%d** HIGH/CRITICAL findings across %s, counted per image — the same CVE on five of them is five repins.\n\n",
		s.Triage.Carried, plural(s.Scanned(), "scanned image", "scanned images"))
	sb.WriteString("| Population | Count | What it means |\n")
	sb.WriteString("| ---------- | ----- | ------------- |\n")
	fmt.Fprintf(sb, "| Go stdlib in a CLI binary | %d | Exempt by class: unreachable in a tool that opens no listener, and gone when the publisher recompiles. |\n", s.Triage.GoStdlib)
	fmt.Fprintf(sb, "| Kernel headers | %d | Exempt by class: the kernel is the host's, not the container's. |\n", s.Triage.KernelHeaders)
	fmt.Fprintf(sb, "| Fixed upstream | %d | A fix exists. This is the images' age, not a decision — an exception must never be written for one. |\n", s.Triage.Fixable)
	fmt.Fprintf(sb, "| **Needing triage** | **%d** | No fix at any version, and not exempt. The only population an exception is the right instrument for. |\n\n", s.Triage.Actionable)

	if len(s.Triage.KEV) > 0 {
		fmt.Fprintf(sb, "**In CISA KEV — being exploited now: %s.** ", strings.Join(s.Triage.KEV, ", "))
	} else {
		sb.WriteString("None of them is in CISA KEV. ")
	}
	fmt.Fprintf(sb, "The highest EPSS score seen is %.2f. Both are reported for a human to read; neither gates anything.\n\n", s.Triage.TopEPSS)
}

// writeSummaryAutomatic separates what a human has to decide from what a loop
// already chases.
//
// The cooldown state is named and not restated. `cidx preset scan-targets` reads
// the registries and the audit never calls it, so answering "which candidate is
// the cooldown holding" here would mean a second computation, from a job holding
// none of the credentials the first one has — two sources for one fact, and the
// one place they could differ is the one a reader would notice. It is pointed
// at instead.
func writeSummaryAutomatic(sb *strings.Builder, s CatalogueSummary) {
	sb.WriteString("## Moving without a decision\n\n")
	fmt.Fprintf(sb, "**%d** of the findings above have a fix upstream. Nothing is accepted or argued for those: they leave when the image is repinned, and an exception written for one would record a decision where there is only a wait.\n\n",
		s.Triage.Fixable)
	fmt.Fprintf(sb, "%s proposes those repins weekly, and lists the candidates its 14-day cooldown is still holding in its own run summary — this page does not restate them, because it holds none of the evidence they are decided from.\n\n",
		link("`container-monitor.yml`", workflowRuns(s.Links.Repo, "container-monitor.yml")))
}

func writeSummaryWhereToLook(sb *strings.Builder, s CatalogueSummary) {
	sb.WriteString("## Where to look\n\n")
	sb.WriteString("| Question | Place |\n")
	sb.WriteString("| -------- | ----- |\n")
	fmt.Fprintf(sb, "| Where does the catalogue stand | this page, rewritten by every audit |\n")
	fmt.Fprintf(sb, "| Which finding, on which image, since when | %s |\n",
		link("the Security tab", securityTab(s.Links.Repo)))
	fmt.Fprintf(sb, "| What we ship, and what is accepted on it | %s |\n",
		link("`SECURITY-BASELINE.md`", blob(s.Links.Repo, "SECURITY-BASELINE.md")))
	fmt.Fprintf(sb, "| Whether an acceptance stands — the source of truth | %s |\n",
		link("`"+ExceptionsFile+"`", blob(s.Links.Repo, ExceptionsFile)))
	fmt.Fprintf(sb, "| Why any of this is judged the way it is | %s |\n\n",
		link("the supply-chain policy", blob(s.Links.Repo, "docs/core-concepts/security.md")))
}

func writeSummaryDigest(sb *strings.Builder, s CatalogueSummary) {
	encoded, err := json.MarshalIndent(s.Digest(), "", "  ")
	if err != nil {
		// Every field is a string, a number or a slice of strings, so this is
		// unreachable — and dropping the block silently would leave a consumer
		// parsing prose. Say so instead.
		fmt.Fprintf(sb, "_The machine-readable digest could not be encoded: %v._\n\n", err)
		return
	}

	sb.WriteString("<details>\n<summary>The same counts, for a machine</summary>\n\n")
	sb.WriteString("```json\n")
	sb.Write(encoded)
	sb.WriteString("\n```\n\n</details>\n")
}

// link renders text as a hyperlink, or as itself when there is no URL to point
// at — which is what happens outside GitHub Actions, where the repository is not
// known and the reader is already inside it.
func link(text, url string) string {
	if url == "" {
		return text
	}
	return "[" + text + "](" + url + ")"
}

func securityTab(repo string) string {
	if repo == "" {
		return ""
	}
	return repo + "/security/code-scanning"
}

// blob points at a file on the default branch. `HEAD` rather than a branch name:
// the page must not name a branch this repository could rename.
func blob(repo, path string) string {
	if repo == "" {
		return ""
	}
	return repo + "/blob/HEAD/" + path
}

func workflowRuns(repo, workflow string) string {
	if repo == "" {
		return ""
	}
	return repo + "/actions/workflows/" + workflow
}

// cell keeps a justification containing a pipe from splitting its row, the same
// escaping SECURITY-BASELINE.md applies.
func cell(text string) string {
	if text == "" {
		return "—"
	}
	return strings.NewReplacer("|", `\|`, "\n", " ", "\r", "").Replace(text)
}

// plural writes "1 finding" and "2 findings", because a page counting things
// should not read as though nobody looked at it.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// joinList writes "a, b and c".
func joinList(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
