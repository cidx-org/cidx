package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

// A fix-unreleased decision is written against the image the catalogue runs, and
// its reason names that image: the guard
// (TestFixUnreleasedNamesTheImageTheCatalogueRuns) exists so that a repin forces
// a new judgement. The Container Monitor repins, so until #394 one such decision
// on an image made the tests of the promotion fail for every other image too, and
// no pull request was opened at all.
//
// The promotion does not rewrite the decision — that would be the renewal by
// default the guard forbids. It does two things with it: it drops it when the new
// image no longer carries the advisory (the finding is gone, the record has
// nothing left to say), and otherwise it names it in the pull request, with what
// the scan of the new image found, so the human who must rejudge it is told.

// decisionRejudgement is one fix-unreleased decision a promotion put in question.
type decisionRejudgement struct {
	Decision string `json:"decision"`
	// Image is the candidate the catalogue now runs; Previous the one the
	// decision was written against.
	Image    string `json:"image"`
	Previous string `json:"previous"`
	// Carried and Gone split the decision's advisories by what the scan of Image
	// found. Unreadable scan results land in Carried: absence of evidence is not
	// a finding that went away.
	Carried []string `json:"carried,omitempty"`
	Gone    []string `json:"gone,omitempty"`
	// Retired says the decision and its members are removed from the file.
	Retired bool `json:"retired"`
	// Note explains a fail-closed reading.
	Note string `json:"note,omitempty"`
}

// rejudgeFixUnreleased judges the fix-unreleased decisions of every promoted
// verdict against the scan of its new image. It changes nothing: the caller
// applies the retired ones with [retireDecisions].
func rejudgeFixUnreleased(file *VulnerabilityFile, verdicts []promotionVerdict, resultsDir string) []decisionRejudgement {
	var out []decisionRejudgement

	for _, verdict := range verdicts {
		if !verdict.Promote {
			continue
		}
		repository := imageRepository(verdict.NewImage)
		next := refWithoutDigest(verdict.NewImage)

		for _, d := range file.Decisions {
			if d.Treatment != "fix-unreleased" || d.Repository != repository {
				continue
			}
			// A decision that already names the new image was rejudged against it.
			if strings.Contains(d.Reason, next) {
				continue
			}

			item := decisionRejudgement{
				Decision: d.ID,
				Image:    next,
				Previous: refWithoutDigest(verdict.CurrentImage),
			}

			found, _, err := scanFindings(resultsDir, verdict.NewImage)
			if err != nil {
				item.Carried = slices.Clone(d.Advisories)
				item.Note = fmt.Sprintf("the new image's scan could not be read (%v): kept", err)
				out = append(out, item)
				continue
			}
			found = append(found, suppressedFindings(resultsDir, verdict.NewImage)...)

			for _, advisory := range d.Advisories {
				if carries(found, advisory) {
					item.Carried = append(item.Carried, advisory)
				} else {
					item.Gone = append(item.Gone, advisory)
				}
			}
			// A decision with no advisory listed cannot be shown gone.
			item.Retired = len(d.Advisories) > 0 && len(item.Carried) == 0
			out = append(out, item)
		}
	}
	return out
}

// carries reports whether a scan found the advisory, under its own identifier or
// one of the aliases the scanner mapped it to.
func carries(found []presets.Finding, advisory string) bool {
	for _, f := range found {
		if strings.EqualFold(f.ID, advisory) {
			return true
		}
		for _, alias := range strings.Split(f.Aliases, ",") {
			if strings.EqualFold(strings.TrimSpace(alias), advisory) {
				return true
			}
		}
	}
	return false
}

// retireDecisions removes the retired decisions and their members from the file.
func retireDecisions(file *VulnerabilityFile, items []decisionRejudgement) {
	retired := make(map[string]bool)
	for _, item := range items {
		if item.Retired {
			retired[item.Decision] = true
		}
	}
	if len(retired) == 0 {
		return
	}
	file.Decisions = slices.DeleteFunc(file.Decisions, func(d Decision) bool { return retired[d.ID] })
	file.Vulnerabilities = slices.DeleteFunc(file.Vulnerabilities, func(v Vulnerability) bool { return retired[v.Decision] })
}

// rejudgementReport is the section of the promotion pull request: what was
// removed, and what a human has to judge again.
func rejudgementReport(items []decisionRejudgement) string {
	var removed, rejudge []string
	for _, item := range items {
		switch {
		case item.Retired:
			removed = append(removed, fmt.Sprintf("- `%s` — `%s` no longer carries %s: the decision and its entries are removed",
				item.Decision, item.Image, strings.Join(item.Gone, ", ")))
		default:
			line := fmt.Sprintf("- `%s` — written against `%s`, the catalogue now runs `%s`. Still carried: %s.",
				item.Decision, item.Previous, item.Image, listOrNone(item.Carried))
			if len(item.Gone) > 0 {
				line += fmt.Sprintf(" No longer carried: %s.", strings.Join(item.Gone, ", "))
			}
			if item.Note != "" {
				line += " " + item.Note + "."
			}
			rejudge = append(rejudge, line)
		}
	}

	var sb strings.Builder
	sb.WriteString("#### Removed: the finding is gone\n\n")
	sb.WriteString(listOrNoneLines(removed))
	sb.WriteString("\n\n#### To rejudge: the image changed\n\n")
	sb.WriteString(listOrNoneLines(rejudge))
	return sb.String()
}

func listOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}

func listOrNoneLines(lines []string) string {
	if len(lines) == 0 {
		return "_None._"
	}
	return strings.Join(lines, "\n")
}

// presetRejudgeCommand is the step between the promotion and its tests.
func presetRejudgeCommand() *cli.Command {
	return &cli.Command{
		Name:  "rejudge-decisions",
		Usage: "Retire or list the fix-unreleased decisions a promotion put in question",
		Description: "Reads `cidx preset scan-verdicts` output (stdin by default). For every\n" +
			"promoted image, a fix-unreleased decision written against the previous image\n" +
			"is removed when the scan of the new image no longer carries its advisories,\n" +
			"and otherwise left untouched and listed as to rejudge. Prints the Markdown\n" +
			"for the pull request.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "verdicts", Value: "-", Usage: "Path to `cidx preset scan-verdicts` output ('-' for stdin)"},
			&cli.StringFlag{Name: "results", Value: defaultResultsDir, Usage: "Directory holding the scanner result files"},
			&cli.StringFlag{Name: "vuln-file", Value: defaultVulnFile, Usage: "Path to known-vulnerabilities.toml"},
			&cli.BoolFlag{Name: "json", Usage: "Print the judgements as JSON instead of Markdown"},
		},
		Action: func(c *cli.Context) error {
			var (
				data []byte
				err  error
			)
			if path := c.String("verdicts"); path == "-" || path == "" {
				data, err = io.ReadAll(os.Stdin)
			} else {
				data, err = os.ReadFile(path)
			}
			if err != nil {
				return fmt.Errorf("could not read the verdicts: %w", err)
			}
			var verdicts []promotionVerdict
			if err := json.Unmarshal(data, &verdicts); err != nil {
				return fmt.Errorf("could not parse the verdicts: %w", err)
			}

			file, err := loadVulnerabilities(c.String("vuln-file"))
			if err != nil {
				return err
			}
			items := rejudgeFixUnreleased(file, verdicts, c.String("results"))
			if slices.ContainsFunc(items, func(i decisionRejudgement) bool { return i.Retired }) {
				retireDecisions(file, items)
				if err := saveVulnerabilities(c.String("vuln-file"), file); err != nil {
					return err
				}
			}

			if c.Bool("json") {
				encoder := json.NewEncoder(c.App.Writer)
				encoder.SetIndent("", "  ")
				return encoder.Encode(items)
			}
			_, err = fmt.Fprintln(c.App.Writer, rejudgementReport(items))
			return err
		},
	}
}
