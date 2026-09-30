package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

// securitySummaryCommand writes the page saying where the catalogue stands
// (#308).
//
// It is the I/O half, exactly as `cidx security sarif` is: it reads the scan
// results the audit already uploaded, the acceptances on file and the bases
// those scans reported, and hands them to `presets.RenderSummary`. Every number
// it prints was computed by code the audit already runs — the triage of
// `pkg/presets/findings.go`, the expiry test of `pkg/presets/sarif.go`, the
// end-of-support verdicts of `pkg/presets/eol.go`. Nothing is recomputed, and
// nothing is decided here.
//
// The one thing this command is missing on purpose is the cooldown: answering
// "which candidate is being held" means reading the registries, which
// `cidx preset scan-targets` does from the monitor, with the credentials the
// monitor holds. Doing it again from the audit would be a second source for one
// fact, and the only place two sources matter is where they disagree.
func securitySummaryCommand() *cli.Command {
	return &cli.Command{
		Name:  "summary",
		Usage: "Render where the catalogue stands: what is waiting for a decision, and what is not",
		Description: "Assembles the numbers the daily audit already produced — the finding\n" +
			"triage, the acceptances past their date, the bases losing support — into\n" +
			"one page. security-audit.yml publishes it to a tracking issue; run it\n" +
			"locally against downloaded artifacts to see the same page.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "results",
				Value: defaultResultsDir,
				Usage: "Directory holding the scanner result files",
			},
			&cli.StringFlag{
				Name:  "file",
				Value: presets.ExceptionsFile,
				Usage: "Path to known-vulnerabilities.toml",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Where to write the page (default: stdout)",
			},
			&cli.BoolFlag{
				Name:  "fail-if-waiting",
				Usage: "Exit non-zero when anything on the page needs a human (for the audit gate)",
			},
			&cli.IntFlag{
				Name:  "grace-days",
				Usage: "Give an unanswered finding this many days, from the audit that first reported it, before it fails the gate (needs --first-seen)",
			},
			&cli.StringFlag{
				Name:  "first-seen",
				Usage: "JSON file mapping \"<repository>/<ID>\" to the day the audit first reported it (needs --grace-days)",
			},
		},
		Action: func(c *cli.Context) error {
			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			record, err := acceptedRecord(c.String("file"))
			if err != nil {
				return err
			}

			grace, err := graceFromFlags(c)
			if err != nil {
				return err
			}

			summary := buildCatalogueSummary(imagePresets, c.String("results"), record, time.Now())
			summary.Grace = grace
			page := presets.RenderSummary(summary)

			out := c.String("output")
			if out == "" {
				fmt.Print(page)
				return gateOn(c, summary)
			}
			if err := os.WriteFile(out, []byte(page), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", out, err)
			}

			fmt.Printf("Wrote %s: %d finding(s) needing triage, %d acceptance(s) past their date, %d image(s) unscanned.\n",
				out, summary.Unanswered, len(summary.Expired), len(summary.Unscanned))
			return gateOn(c, summary)
		},
	}
}

// buildCatalogueSummary gathers what the page states, from the evidence the
// audit produced.
//
// It takes `now` rather than reading the clock so a scenario can stage an
// expiry date, the same seam `catalogueAlerts` uses.
func buildCatalogueSummary(
	imagePresets map[string][]string,
	resultsDir string,
	record *VulnerabilityFile,
	now time.Time,
) presets.CatalogueSummary {
	// The same population SECURITY-BASELINE.md counts, suppressed half
	// included (#310): this page and that file answer the same question, and
	// two numbers for it would be the confusion #308 built the page to end.
	//
	// Including the caveat, for the same reason. A count the results cannot
	// vouch for is a floor in both places or in neither (#327).
	var accepted []Vulnerability
	if record != nil {
		accepted = record.Vulnerabilities
	}
	carried, accounted := carriedFindings(imagePresets, resultsDir, accepted)

	var unscanned []string
	for image := range imagePresets {
		if _, scanned := carried[image]; !scanned {
			unscanned = append(unscanned, image)
		}
	}
	sort.Strings(unscanned)

	exceptions := ExceptionsFor(record, now, presets.ExceptionsFile)

	left := unaccepted(carried, accepted)

	return presets.CatalogueSummary{
		Unanswered:   triageCatalogue(left).Actionable,
		Findings:     unansweredFindings(left),
		Images:       len(imagePresets),
		Unscanned:    unscanned,
		CarriedFloor: !accounted,
		Triage:       triageCatalogue(carried),
		Accepted:     len(acceptedFindings(imagePresets, accepted)),
		Expired:      presets.ExpiredExceptions(exceptions, now),
		Bases:        baseNotes(catalogueBases(imagePresets, resultsDir), now),
		Day:          now,
		Links:        summaryLinks(),
	}
}

// baseNotes keeps the bases a reader has to do something about, or that nothing
// could resolve.
//
// A supported base is dropped: it is a fact about the calendar, and printing
// twenty of them daily is how a section teaches its reader to skip it. An
// endoflife.date outage is kept, because an unchecked base must never read as a
// checked one.
func baseNotes(bases map[string]presets.BaseOS, now time.Time) []presets.BaseNote {
	var notes []presets.BaseNote
	for image, support := range resolveBaseSupport(bases, now) {
		if !support.NeedsAttention() && support.State != presets.BaseUnchecked {
			continue
		}
		notes = append(notes, presets.BaseNote{
			Image:  image,
			State:  support.State,
			Reason: support.Reason,
		})
	}
	sort.Slice(notes, func(i, j int) bool { return notes[i].Image < notes[j].Image })
	return notes
}

// summaryLinks reads the repository the page points at from the environment
// GitHub Actions already sets.
//
// No flag, and no default repository baked in: outside a run there is nothing to
// point at, the reader is already in the checkout, and the page renders the same
// text without hyperlinks. Guessing a repository would be the one way to publish
// a link that goes somewhere else.
func summaryLinks() presets.SummaryLinks {
	server, repo := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY")
	if server == "" || repo == "" {
		return presets.SummaryLinks{}
	}

	links := presets.SummaryLinks{Repo: server + "/" + repo}
	if run := os.Getenv("GITHUB_RUN_ID"); run != "" {
		links.Run = links.Repo + "/actions/runs/" + run
	}
	return links
}

// gateOn turns the page into the audit's verdict when --fail-if-waiting is set.
//
// The gate asks the same question the page answers, deliberately: the audit used
// to fail on every unaccepted HIGH/CRITICAL, which included the 427 findings a
// fix already exists for — and the policy forbids writing exceptions for those
// ("never write an exception for one of these, it would record a decision where
// there is only a wait"). The only route to green was the one thing the policy
// refuses, so the audit was red for weeks and stopped meaning anything (#436).
//
// Waiting() is what the Security tab already publishes, so the gate, the tab and
// SECURITY-BASELINE.md cannot disagree about what needs a human.
func gateOn(c *cli.Context, summary presets.CatalogueSummary) error {
	if !c.Bool("fail-if-waiting") || !summary.Failing() {
		return nil
	}

	// With a window the gate fails on a subset of what the page lists, and says
	// which: a red that names nothing is the one nobody can act on.
	if summary.Grace != nil {
		past := summary.PastGrace()
		named := make([]string, 0, len(past))
		for _, f := range past {
			label := f.Repository + " " + f.ID
			if f.KEV {
				label += " (known exploited: no window)"
			}
			named = append(named, label)
		}
		if len(named) > 10 {
			named = append(named[:10], fmt.Sprintf("… and %d more", len(past)-10))
		}
		return fmt.Errorf("the catalogue is waiting on a human: %d finding(s) past their %d-day window (%d more are inside it and listed on the page), "+
			"%d acceptance(s) past their date, plus any base named on the page above.\n  %s",
			len(past), summary.Grace.Days, summary.Unanswered-len(past), len(summary.Expired), strings.Join(named, "\n  "))
	}

	return fmt.Errorf("the catalogue is waiting on a human: %d finding(s) with no fix at any version, "+
		"%d acceptance(s) past their date, plus any base named on the page above. The findings are in "+
		"the Security tab; an exception is argued with: cidx security vuln add <CVE> <image>",
		summary.Unanswered, len(summary.Expired))
}

// unaccepted drops the findings an exception already answers.
//
// Carried deliberately keeps them: the baseline reports what an image carries
// and what was accepted as two numbers, and hiding the second inside the first
// is how that file once read "0 accepted findings" on a catalogue carrying 596.
// Triage over that set therefore counts a finding somebody has already argued,
// which is right for "what does this image carry" and wrong for "what is left
// for a human" -- the question the gate and the page's headline ask (#439).
//
// Nobody noticed because the exceptions on file were all kernel-header ones:
// an exempt class, never Actionable, so acceptance and actionability had never
// once overlapped.
func unaccepted(carried map[string][]presets.Finding, accepted []Vulnerability) map[string][]presets.Finding {
	answered := make(map[string]bool, len(accepted))
	for _, v := range accepted {
		answered[v.Repository+"\x00"+strings.ToUpper(v.CVE)] = true
	}

	left := make(map[string][]presets.Finding, len(carried))
	for image, findings := range carried {
		repository := imageRepository(image)
		kept := make([]presets.Finding, 0, len(findings))
		for _, f := range findings {
			if !answered[repository+"\x00"+strings.ToUpper(f.ID)] {
				kept = append(kept, f)
			}
		}
		left[image] = kept
	}
	return left
}

// unansweredFindings names what unaccepted left, one per image and identifier:
// the same population triageCatalogue counts as Actionable, in a fixed order.
func unansweredFindings(left map[string][]presets.Finding) []presets.UnansweredFinding {
	images := make([]string, 0, len(left))
	for image := range left {
		images = append(images, image)
	}
	sort.Strings(images)

	var out []presets.UnansweredFinding
	for _, image := range images {
		for _, group := range presets.Actionable(left[image]) {
			kev := false
			for _, f := range group {
				kev = kev || f.KEV
			}
			out = append(out, presets.UnansweredFinding{Repository: imageRepository(image), ID: group[0].ID, KEV: kev})
		}
	}
	return out
}

// graceFromFlags reads the window and where its ages come from.
//
// The two go together: a window with no source of ages cannot be applied, and a
// source with no window would do nothing — a setting that parses and does
// nothing is worse than an absent one (#322). Either alone is refused rather
// than quietly ignored. Neither is the gate as it always was.
func graceFromFlags(c *cli.Context) (*presets.Grace, error) {
	file := c.String("first-seen")
	windowSet := c.IsSet("grace-days")
	switch {
	case !windowSet && file == "":
		return nil, nil
	case windowSet && file == "":
		return nil, fmt.Errorf("--grace-days needs --first-seen: a finding's age cannot be judged without knowing when the audit first reported it")
	case !windowSet:
		return nil, fmt.Errorf("--first-seen needs --grace-days: the dates would change nothing without a window")
	}
	days := c.Int("grace-days")
	if days < 0 {
		return nil, fmt.Errorf("--grace-days must not be negative, got %d", days)
	}
	seen, err := readFirstSeen(file)
	if err != nil {
		return nil, err
	}
	return &presets.Grace{Days: days, FirstSeen: seen}, nil
}

// readFirstSeen loads the day the audit first reported each finding. An
// unreadable or malformed file is an error, not an empty map: an empty map means
// "the source answered and has seen nothing", which reads every finding as new
// today, and an audit that cannot read its own clock must not grant that.
func readFirstSeen(path string) (map[string]time.Time, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read the first-seen dates: %w", err)
	}
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s is not a JSON object of \"<repository>/<ID>\" to a date: %w", path, err)
	}
	seen := make(map[string]time.Time, len(raw))
	for key, value := range raw {
		day, err := time.Parse(time.RFC3339, value)
		if err != nil {
			day, err = time.Parse(time.DateOnly, value)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %q for %s is not a date (RFC 3339 or YYYY-MM-DD)", path, value, key)
		}
		// The repository may itself hold slashes (ghcr.io/ansible/...): the ID is
		// what follows the last one.
		i := strings.LastIndex(key, "/")
		if i <= 0 {
			return nil, fmt.Errorf("%s: key %q is not <repository>/<ID>", path, key)
		}
		seen[presets.AlertKey(key[:i], key[i+1:])] = day
	}
	return seen, nil
}
