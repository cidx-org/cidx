package commands

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
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
		},
		Action: func(c *cli.Context) error {
			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			accepted, err := acceptedExceptions(c.String("file"))
			if err != nil {
				return err
			}

			summary := buildCatalogueSummary(imagePresets, c.String("results"), accepted, time.Now())
			page := presets.RenderSummary(summary)

			out := c.String("output")
			if out == "" {
				fmt.Print(page)
				return nil
			}
			if err := os.WriteFile(out, []byte(page), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", out, err)
			}

			fmt.Printf("Wrote %s: %d finding(s) needing triage, %d acceptance(s) past their date, %d image(s) unscanned.\n",
				out, summary.Triage.Actionable, len(summary.Expired), len(summary.Unscanned))
			return nil
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
	accepted []Vulnerability,
	now time.Time,
) presets.CatalogueSummary {
	// The same population SECURITY-BASELINE.md counts, suppressed half
	// included (#310): this page and that file answer the same question, and
	// two numbers for it would be the confusion #308 built the page to end.
	//
	// Including the caveat, for the same reason. A count the results cannot
	// vouch for is a floor in both places or in neither (#327).
	carried, accounted := carriedFindings(imagePresets, resultsDir, accepted)

	var unscanned []string
	for image := range imagePresets {
		if _, scanned := carried[image]; !scanned {
			unscanned = append(unscanned, image)
		}
	}
	sort.Strings(unscanned)

	exceptions := exceptionsFor(accepted, presets.ExceptionsFile)

	return presets.CatalogueSummary{
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
