package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// prunedEntry pairs an exception with what the lifecycle policy made of it.
type prunedEntry struct {
	Vulnerability
	presets.ExceptionVerdict
}

// vulnPruneCommand reports — and, when asked, removes — the exceptions the
// catalogue images no longer carry.
//
// The criterion is the CVE, not the tag. `vuln list --stale` already says which
// entries match no catalogue image (#248); what it cannot say is which of them
// are safe to delete. An accepted CVE either went away with the image it was
// recorded against, or followed the promotion into the image that replaced it —
// and deleting the second kind loses the justification and turns the next audit
// red for a finding that was reviewed months ago.
//
// So this reads the findings rather than comparing tags, and it reports by
// default: `-x` is what deletes, the same shape as `repo branch cleanup`. The
// decision to stop waiving a CVE belongs to whoever accepted it.
func vulnPruneCommand() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Report the exceptions no catalogue image carries any more, and remove them on request",
		Description: "Reads the scanner results the security audit and the container monitor\n" +
			"produce, and classifies every exception: live (covers an image the\n" +
			"catalogue runs), carry-over (its CVE followed the promotion and the entry\n" +
			"must be re-filed), obsolete (no catalogue image carries it any more) or\n" +
			"unknown (no scan evidence). Only obsolete entries are ever removed, and\n" +
			"only with --execute.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "results",
				Value: "scan-results",
				Usage: "Directory holding the scanner result files (audit or monitor artifacts)",
			},
			&cli.BoolFlag{
				Name:    "execute",
				Aliases: []string{"x"},
				Usage:   "Actually remove the obsolete exceptions (default: report only)",
			},
		},
		Action: func(c *cli.Context) error {
			filePath := c.String("file")
			vulns, err := loadVulnerabilities(filePath)
			if err != nil {
				return err
			}

			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			running, findings := catalogueFindings(imagePresets, c.String("results"))

			entries := make([]prunedEntry, 0, len(vulns.Vulnerabilities))
			for _, v := range vulns.Vulnerabilities {
				entries = append(entries, prunedEntry{
					Vulnerability:    v,
					ExceptionVerdict: presets.ClassifyException(v.CVE, refWithoutDigest(v.Image), running, findings),
				})
			}

			printPruneReport(entries, len(findings), len(running), c.String("results"))

			if !c.Bool("execute") {
				return nil
			}
			return applyPrune(filePath, vulns, entries)
		},
	}
}

// catalogueFindings resolves what the catalogue runs today and what the scanners
// found on it.
//
// The findings are read from result files already produced — security-audit.yml
// scans every catalogue image daily, container-monitor.yml scans them weekly,
// and both upload their JSON. Nothing is scanned here: a command that pulled
// twenty images to answer a bookkeeping question would be run once and never
// again. An image with no result file is simply absent from the map, which is
// what makes the verdict fail-closed rather than optimistic.
func catalogueFindings(imagePresets map[string][]string, resultsDir string) ([]string, map[string][]string) {
	running := make([]string, 0, len(imagePresets))
	findings := make(map[string][]string)

	for image := range imagePresets {
		ref := refWithoutDigest(image)
		running = append(running, ref)

		if found, _, err := scanFindings(resultsDir, image); err == nil {
			findings[ref] = found
		}
	}

	sort.Strings(running)
	return running, findings
}

// printPruneReport says what each state means before listing it. A report that
// only printed counts would be read as "155 to delete", which is exactly the
// mistake it exists to prevent.
func printPruneReport(entries []prunedEntry, scanned, catalogue int, resultsDir string) {
	fmt.Println("Vulnerability Exception Lifecycle")
	fmt.Println("=================================")
	fmt.Println()
	fmt.Printf("Catalogue images: %d, of which %d have scanner results in %s\n", catalogue, scanned, resultsDir)
	fmt.Println()

	sections := []struct {
		state  string
		title  string
		legend string
	}{
		{presets.ExceptionObsolete, "OBSOLETE", "no catalogue image carries these any more — --execute removes them"},
		{presets.ExceptionCarryOver, "CARRY-OVER", "the CVE followed the promotion: re-file the entry against the image below, do not delete it"},
		{presets.ExceptionUnknown, "UNKNOWN", "no scan result for some catalogue image, so nothing can be concluded"},
		{presets.ExceptionLive, "LIVE", "covers an image the catalogue runs today"},
	}

	for _, section := range sections {
		matching := entriesInState(entries, section.state)
		if len(matching) == 0 {
			continue
		}

		fmt.Printf("%s (%d) — %s:\n", section.title, len(matching), section.legend)
		fmt.Println(strings.Repeat("-", 50))
		for _, e := range matching {
			fmt.Printf("  %s | %s", e.CVE, e.Image)
			if e.StillOn != "" {
				fmt.Printf(" → still on %s", e.StillOn)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	if scanned == 0 {
		fmt.Printf("No scanner result was found in %s, so nothing can be shown to have gone.\n", resultsDir)
		fmt.Println("Point --results at the JSON the Security Audit or Container Monitor workflow uploaded.")
		fmt.Println()
	}
}

// applyPrune removes the obsolete entries and nothing else. Carry-over and
// unknown stay: one is information that has to be re-filed, the other is a
// question nobody has answered, and deleting either would be the tool making a
// call that is not its to make.
func applyPrune(path string, vulns *VulnerabilityFile, entries []prunedEntry) error {
	obsolete := entriesInState(entries, presets.ExceptionObsolete)
	if len(obsolete) == 0 {
		fmt.Println("Nothing to remove.")
		return nil
	}

	kept := make([]Vulnerability, 0, len(entries))
	for _, e := range entries {
		if e.State != presets.ExceptionObsolete {
			kept = append(kept, e.Vulnerability)
		}
	}
	vulns.Vulnerabilities = kept

	if err := saveVulnerabilities(path, vulns); err != nil {
		return fmt.Errorf("failed to save %s: %w", path, err)
	}

	fmt.Printf("Removed %d obsolete exception(s) from %s.\n", len(obsolete), path)
	if carried := entriesInState(entries, presets.ExceptionCarryOver); len(carried) > 0 {
		fmt.Printf("%d carry-over entr(y/ies) left in place: they still describe a finding a catalogue image has.\n", len(carried))
	}
	return nil
}

// entriesInState keeps the file's order, so two runs of the report list the same
// entries in the same places and a diff of the output is readable.
func entriesInState(entries []prunedEntry, state string) []prunedEntry {
	var matching []prunedEntry
	for _, e := range entries {
		if e.State == state {
			matching = append(matching, e)
		}
	}
	return matching
}
