package presets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Publishing the catalogue's state to GitHub code scanning (#301).
//
// The evidence was scattered across four places — scan results in artifacts
// deleted after a day, acceptances in `known-vulnerabilities.toml`, totals in a
// generated markdown table, reasoning in issue threads — and none of them
// answers "where does the catalogue stand" without the other three. The Security
// tab is one place that keeps history across runs, dates its findings and closes
// them by itself. What it reads is SARIF.
//
// Both scanners emit SARIF natively and neither is usable here, for one reason:
// what makes the tab readable is what it leaves out. The catalogue carries 456
// HIGH/CRITICAL findings and 150 of them need a human — publishing the raw scan
// would bury the 150 under 306 findings the policy has already answered, which
// is the failure mode the tab exists to remove. No scanner can do that filtering
// either: Trivy has no "only unfixed" switch, and neither tool knows what
// `stdlib` in a Go binary or `linux-libc-dev` means in a container that listens
// on nothing. That judgement is [Summarise], and using it here is what keeps the
// tab and `SECURITY-BASELINE.md` from ever disagreeing.
//
// `known-vulnerabilities.toml` remains the source of truth. This is a view.

// The repository paths an alert points at. A finding is answered by repinning
// the image, and an expired acceptance by editing its entry, so the alert is
// anchored to the line that has to change: the Security tab links straight to
// it, and a pull request touching that line carries the alert with it.
const (
	// CatalogueFile is where the built-in catalogue pins its images.
	CatalogueFile = "pkg/presets/presets.toml"

	// ExceptionsFile is where the accepted findings are recorded.
	ExceptionsFile = "known-vulnerabilities.toml"

	// expiredRule prefixes the identifier of an alert about a lapsed
	// acceptance, so that the two alert families sharing a repository and a CVE
	// stay tellable apart — which is what [MergeAlerts] reads.
	expiredRule = "expired/"
)

// Alert is one thing the catalogue needs a human for, before it becomes SARIF.
type Alert struct {
	// RuleID is what GitHub groups and matches an alert on. It is keyed by
	// repository rather than by the pinned reference, exactly as an exception
	// is: the judgement survives a repin, and an identifier carrying a digest
	// would close every alert and open it again on every promotion.
	RuleID string

	// Title is the rule's short description — the headline in the alert list —
	// and Body is the alert page. Message is the per-occurrence line.
	Title   string
	Body    string
	Message string

	// Severity is HIGH or CRITICAL, the band the policy acts on.
	Severity string

	// File and Line are the line a human would edit.
	File string
	Line int

	// Fingerprint is what GitHub matches an alert on across runs. It is derived
	// from what the alert is about rather than from where it is written: both
	// files gain and lose lines constantly, and a positional fingerprint would
	// close and reopen every alert each time one preset moved.
	//
	// "What it is about" is the repository and the identifier — the key [RuleID]
	// and `known-vulnerabilities.toml` already use — never the pinned reference.
	// A fingerprint carrying the tag and the digest changed on every promotion,
	// so a repin closed every alert on that image and opened near-identical ones,
	// including for CVEs that never went away (#313). The reference travels in
	// [Alert.Message], where it is informative rather than identifying.
	Fingerprint string
}

// Exception is an accepted finding, reduced to what an alert about it says.
type Exception struct {
	CVE        string
	Repository string
	Severity   string
	Status     string
	Added      string
	Expires    string
	Notes      string

	// Stopped says why the entry no longer waives, as resolved against the
	// grouped decision it references — the date that passed, the capability
	// that appeared, the predicate nobody established. Empty for an entry
	// judged by its own date alone, and for a member whose decision stands.
	Stopped string

	// Line is where the entry sits in [ExceptionsFile].
	Line int
}

// ExpiredExceptions keeps the acceptances that no longer waive: past their own
// date, or members whose decision stopped standing and says why in Stopped.
//
// One definition, because two views read it: the alert published to code
// scanning (#301) and the count on the status page (#308). A page saying "19
// past their date" beside a tab showing 18 alerts would be the one bug neither
// of them could be trusted after.
//
// An entry whose date does not parse is not expired here. `cidx security vuln
// check` is what reports a malformed date, as a warning naming the entry;
// treating it as lapsed would file a typo as a decision somebody has to defend.
// It waives nothing all the same — see [Waives], which shares this parse and
// this boundary so that the day an entry stops filtering is the day this says it
// lapsed.
func ExpiredExceptions(exceptions []Exception, today time.Time) []Exception {
	var expired []Exception
	for _, e := range exceptions {
		if lapsed(e.Expires, today) || e.Stopped != "" {
			expired = append(expired, e)
		}
	}
	return expired
}

// TriageAlerts turns one image's scan results into the alerts worth publishing.
//
// Only the findings [Summarise] counts under Actionable: no fix at any version,
// not exempt by class. The rest are deliberately absent, and that is most of the
// design —
//
//   - a finding with a fix upstream is the image's age, not a decision. It
//     leaves when the publisher republishes, and `container-monitor.yml` is the
//     loop that chases it;
//   - the Go standard library in a CLI binary and the kernel headers package are
//     unreachable by construction.
//
// Both populations are counted, named and explained in `SECURITY-BASELINE.md`.
// Neither needs an alert somebody has to dismiss.
//
// Trivy and Grype are merged rather than published side by side. Of the 150
// findings needing triage, 39 are seen by both, 102 by Grype alone and 9 by
// Trivy alone: a run per scanner would show 189 alerts for 150 problems, a third
// of them twice, and publishing one scanner would hide two thirds of what needs
// attention. Merging on the identifier gives 150 and loses nothing — the
// attribution travels in the alert, which is where a reader deciding what to do
// wants it anyway.
//
// The alert is keyed on the repository, not on the reference it was found on:
// identifier, fingerprint and the `vuln add` line the body prints all drop the
// tag and the digest, exactly as an exception does. What the reference decides
// is where the alert *points* — [CatalogueFile] and the line pinning this image,
// which a repin rewrites in place, so the link follows the promotion while the
// identity survives it.
func TriageAlerts(image string, usedBy []string, findings []Finding, line int) []Alert {
	repository := repositoryOf(image)

	presetNames := append([]string(nil), usedBy...)
	sort.Strings(presetNames)

	var alerts []Alert
	for _, group := range Actionable(findings) {
		id := strings.ToUpper(group[0].ID)
		packages := join(distinct(group, func(f Finding) string { return f.Package }), "an unnamed package")
		scanners := join(distinct(group, func(f Finding) string { return f.Scanner }), "an unnamed scanner")

		var body strings.Builder
		fmt.Fprintf(&body, "`%s` carries **%s** (%s) in %s.\n\n",
			image, id, strings.ToUpper(group[0].Severity), packages)
		body.WriteString("No fix exists at any version and it is not exempt by class, so it is one of the findings the supply-chain policy asks a human to judge.\n\n")
		fmt.Fprintf(&body, "- Reported by %s.\n", scanners)
		fmt.Fprintf(&body, "- Used by preset(s): %s.\n", strings.Join(presetNames, ", "))
		if epss := topEPSS(group); epss > 0 {
			fmt.Fprintf(&body, "- EPSS %.2f — the probability of exploitation in the next 30 days. Reported, never thresholded.\n", epss)
		}
		if anyKEV(group) {
			body.WriteString("- **In CISA KEV: this one is being exploited right now.** Question 1 of the triage, and it answers on its own.\n")
		}
		body.WriteString("\nJudge it against the threat model in the supply-chain policy: a CIDX container lives for seconds, exposes no service and holds no durable secret.\n\n")
		fmt.Fprintf(&body, "If the risk is accepted, record it — with a reason and an expiry date:\n\n```\ncidx security vuln add %s %s\n```\n", id, repository)

		alerts = append(alerts, Alert{
			RuleID:      repository + "/" + id,
			Title:       fmt.Sprintf("%s: %s needs triage — no fix upstream", repository, id),
			Body:        body.String(),
			Message:     fmt.Sprintf("%s in %s on `%s`, reported by %s. No fix at any version.", id, packages, image, scanners),
			Severity:    strings.ToUpper(group[0].Severity),
			File:        CatalogueFile,
			Line:        line,
			Fingerprint: fingerprint("triage", repository, id),
		})
	}
	return alerts
}

// ExpiredExceptionAlerts reports the acceptances that have lapsed.
//
// This is the case for the file being the source of truth and the tab being a
// view: the view reads the file and says what the file says about itself. An
// exception is written with a date so the acceptance gets argued again rather
// than inherited; past that date it records a decision nobody has stood behind
// since it was taken.
//
// Until #303 nothing else could surface these at all: the audit built its
// ignore file from every entry whatever its date, so a lapsed one went on
// filtering its finding out of the audit's own results, and the finding could
// not reach [TriageAlerts] because the evidence for it was suppressed before the
// JSON was written. Now it can, and both would fire on the same repository and
// CVE — [MergeAlerts] is where that is settled.
func ExpiredExceptionAlerts(exceptions []Exception, today time.Time) []Alert {
	var alerts []Alert
	for _, e := range ExpiredExceptions(exceptions, today) {
		id := strings.ToUpper(e.CVE)

		// What stopped the waiver is the whole of the alert: a date for a legacy
		// entry, the decision's own verdict for a member — and the verdict
		// names the date, the capability or the predicate, which is what the
		// reviewer needs to re-argue it.
		why := fmt.Sprintf("expired on **%s**", e.Expires)
		title := fmt.Sprintf("%s: the exception for %s expired on %s", e.Repository, id, e.Expires)
		message := fmt.Sprintf("%s is accepted on %s by an exception that expired on %s and waives nothing until it is reviewed.", id, e.Repository, e.Expires)
		if e.Stopped != "" {
			why = "no longer stands: " + e.Stopped
			title = fmt.Sprintf("%s: the acceptance of %s no longer stands", e.Repository, id)
			message = fmt.Sprintf("%s is accepted on %s by a decision that no longer stands (%s) and waives nothing until it is reviewed.", id, e.Repository, e.Stopped)
		}

		var body strings.Builder
		fmt.Fprintf(&body, "The exception accepting **%s** on `%s` %s.\n\n", id, e.Repository, why)
		if e.Notes != "" {
			fmt.Fprintf(&body, "> %s\n\n", e.Notes)
		}
		body.WriteString("An exception is written with a date so the acceptance gets argued again rather than inherited. Past that date it waives nothing: the finding is back in the audit's scan results, and it is waiting on a decision again.\n\n")
		fmt.Fprintf(&body, "Re-date it if the judgement still holds, or delete it: `cidx security vuln check` lists them, and `cidx security vuln prune` reports the ones no catalogue image carries any more. Recorded %s, status `%s`.\n", e.Added, e.Status)

		alerts = append(alerts, Alert{
			RuleID:      expiredRule + e.Repository + "/" + id,
			Title:       title,
			Body:        body.String(),
			Message:     message,
			Severity:    strings.ToUpper(e.Severity),
			File:        ExceptionsFile,
			Line:        e.Line,
			Fingerprint: fingerprint("expired", e.Repository, id),
		})
	}

	sort.Slice(alerts, func(i, j int) bool { return alerts[i].RuleID < alerts[j].RuleID })
	return alerts
}

// MergeAlerts publishes one alert per repository and CVE.
//
// The two families could not collide before #303. A lapsed acceptance went on
// filtering its finding out of the audit's results, so [TriageAlerts] never saw
// it — which is precisely why [ExpiredExceptionAlerts] was added, and why it
// said so. Honouring the date removes the suppression, the finding comes back,
// and the same CVE on the same repository now has one alert from each.
//
// Two alerts is the wrong answer. It is one CVE, on one repository, awaiting one
// decision; the second is the same question asked twice, one of them gets
// dismissed to clear the list, and which one is a coin toss. So the expired
// alert supersedes the triage alert for that key.
//
// **The expired one is kept because it says strictly more.** It carries the
// judgement somebody already wrote and the date it lapsed, and it points at the
// entry in [ExceptionsFile] — the line a human edits to re-date or delete it.
// The triage alert points at the image pin and offers `cidx security vuln add`,
// which is the wrong instruction for a CVE that already has an entry: that
// command refuses a duplicate. Keeping the expired one also leaves the alert a
// reader has been looking at since #301 exactly where it was, rather than
// closing eighteen and opening near-identical replacements — the churn #313
// removed from repins, for the same reason.
//
// A lapsed acceptance whose finding is exempt by class, fixed upstream or no
// longer carried produces no triage alert to supersede, and its own alert stands
// alone as it always did.
func MergeAlerts(triage, expired []Alert) []Alert {
	superseded := make(map[string]bool, len(expired))
	for _, a := range expired {
		superseded[strings.TrimPrefix(a.RuleID, expiredRule)] = true
	}

	merged := make([]Alert, 0, len(triage)+len(expired))
	for _, a := range triage {
		if superseded[a.RuleID] {
			continue
		}
		merged = append(merged, a)
	}
	return append(merged, expired...)
}

// SARIF renders the alerts as a single SARIF 2.1.0 run.
//
// One run, not one per image and not one per scanner. Three things settle the
// granularity, and it is the decision that makes the tab readable or useless:
//
//   - SARIF allows 20 runs in a file and the catalogue has 21 images, so "a run
//     per image" is not on the menu at all;
//   - a separate upload per image would be 21 analyses distinguished only by
//     their code scanning category, which the alert list offers no way to filter
//     by — the reader would see one undifferentiated list and gain nothing,
//     while all 21 would have to be re-uploaded on every run to stay current;
//   - what actually distinguishes an image in the UI is the alert itself: its
//     rule identifier names the repository, and its location is the line pinning
//     that image. Both work just as well inside one run.
func SARIF(alerts []Alert) SarifLog {
	var (
		rules   []SarifRule
		results []SarifResult
		seen    = map[string]bool{}
	)

	for _, a := range alerts {
		if !seen[a.RuleID] {
			seen[a.RuleID] = true
			rules = append(rules, SarifRule{
				ID:                   a.RuleID,
				Name:                 a.RuleID,
				ShortDescription:     SarifText{Text: a.Title},
				FullDescription:      SarifText{Text: firstLine(a.Body)},
				Help:                 SarifHelp{Text: a.Body, Markdown: a.Body},
				DefaultConfiguration: SarifConfiguration{Level: sarifLevel(a.Severity)},
				Properties: SarifRuleProperties{
					// GitHub bands an alert off this number rather than off
					// `level`: 9.0 and above reads Critical, 7.0 and above High.
					SecuritySeverity: securitySeverity(a.Severity),
					Tags:             []string{"security", "supply-chain"},
				},
			})
		}

		line := a.Line
		if line < 1 {
			line = 1
		}
		results = append(results, SarifResult{
			RuleID:  a.RuleID,
			Level:   sarifLevel(a.Severity),
			Message: SarifText{Text: a.Message},
			Locations: []SarifLocation{{
				PhysicalLocation: SarifPhysicalLocation{
					ArtifactLocation: SarifArtifactLocation{URI: a.File},
					Region:           SarifRegion{StartLine: line},
				},
			}},
			PartialFingerprints: map[string]string{"primaryLocationLineHash": a.Fingerprint},
		})
	}

	return SarifLog{
		Schema:  "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
		Version: "2.1.0",
		Runs: []SarifRun{{
			Tool: SarifTool{Driver: SarifDriver{
				Name:           "CIDX catalogue audit",
				InformationURI: "https://github.com/cidx-org/cidx/blob/main/docs/core-concepts/supply-chain-policy.md",
				Rules:          rules,
			}},
			Results: results,
		}},
	}
}

// The SARIF 2.1.0 subset GitHub code scanning reads. Written out rather than
// pulled in: a schema-complete SARIF library would be a dependency carrying
// hundreds of types for the dozen fields used here.
type SarifLog struct {
	Schema  string     `json:"$schema"`
	Version string     `json:"version"`
	Runs    []SarifRun `json:"runs"`
}

type SarifRun struct {
	Tool    SarifTool     `json:"tool"`
	Results []SarifResult `json:"results"`
}

type SarifTool struct {
	Driver SarifDriver `json:"driver"`
}

type SarifDriver struct {
	Name           string      `json:"name"`
	InformationURI string      `json:"informationUri"`
	Rules          []SarifRule `json:"rules"`
}

type SarifRule struct {
	ID                   string              `json:"id"`
	Name                 string              `json:"name"`
	ShortDescription     SarifText           `json:"shortDescription"`
	FullDescription      SarifText           `json:"fullDescription"`
	Help                 SarifHelp           `json:"help"`
	DefaultConfiguration SarifConfiguration  `json:"defaultConfiguration"`
	Properties           SarifRuleProperties `json:"properties"`
}

type SarifRuleProperties struct {
	SecuritySeverity string   `json:"security-severity"`
	Tags             []string `json:"tags"`
}

type SarifConfiguration struct {
	Level string `json:"level"`
}

type SarifText struct {
	Text string `json:"text"`
}

type SarifHelp struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

type SarifResult struct {
	RuleID              string            `json:"ruleId"`
	Level               string            `json:"level"`
	Message             SarifText         `json:"message"`
	Locations           []SarifLocation   `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
}

type SarifLocation struct {
	PhysicalLocation SarifPhysicalLocation `json:"physicalLocation"`
}

type SarifPhysicalLocation struct {
	ArtifactLocation SarifArtifactLocation `json:"artifactLocation"`
	Region           SarifRegion           `json:"region"`
}

type SarifArtifactLocation struct {
	URI string `json:"uri"`
}

type SarifRegion struct {
	StartLine int `json:"startLine"`
}

// securitySeverity is the CVSS-shaped number GitHub bands an alert with. The
// scanners report HIGH and CRITICAL as words; these are the bottom of each band,
// which is what a severity carrying no score can honestly claim.
func securitySeverity(severity string) string {
	if strings.EqualFold(severity, "CRITICAL") {
		return "9.0"
	}
	return "7.0"
}

func sarifLevel(severity string) string {
	if strings.EqualFold(severity, "CRITICAL") {
		return "error"
	}
	return "warning"
}

// repositoryOf drops the tag and the digest, leaving the key an exception is
// written against.
func repositoryOf(image string) string {
	if i := strings.Index(image, "@"); i > 0 {
		image = image[:i]
	}
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		image = image[:i]
	}
	return image
}

// fingerprint derives a stable identity from what the alert is about.
func fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// distinct collects one field across a group, sorted and deduplicated, dropping
// the empties a scanner left unset.
func distinct(group []Finding, field func(Finding) string) []string {
	var values []string
	for _, f := range group {
		if v := field(f); v != "" {
			values = append(values, v)
		}
	}
	sort.Strings(values)
	return compactStrings(values)
}

// join names things the way the scan gate's verdicts already do — "Trivy and
// Grype", never a count. The fallback says so out loud rather than leaving a
// gap in the sentence: a scanner that reported a finding against no package at
// all is worth reading as odd.
func join(names []string, empty string) string {
	switch len(names) {
	case 0:
		return empty
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

func firstLine(text string) string {
	if i := strings.Index(text, "\n"); i >= 0 {
		return strings.TrimSpace(text[:i])
	}
	return strings.TrimSpace(text)
}

func topEPSS(group []Finding) float64 {
	var top float64
	for _, f := range group {
		if f.EPSS > top {
			top = f.EPSS
		}
	}
	return top
}

func anyKEV(group []Finding) bool {
	for _, f := range group {
		if f.KEV {
			return true
		}
	}
	return false
}
