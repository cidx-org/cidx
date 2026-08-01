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

// vulnPruneCommand reports — and, when asked, applies — what the lifecycle makes
// of every exception on file.
//
// The criterion is the CVE, not the tag. `vuln list --stale` already says which
// entries match no catalogue repository (#248); what it cannot say is what to do
// about them. An accepted CVE either went away with the image it was recorded
// against, or followed the promotion into the image that replaced it — and
// deleting the second kind loses the justification and turns the next audit red
// for a finding that was reviewed months ago.
//
// So this reads the findings rather than comparing tags, and it reports by
// default: `-x` is what writes, the same shape as `repo branch cleanup`. What it
// writes is only ever mechanical — an obsolete entry removed, a carry-over entry
// re-filed onto the repository that carries its CVE. Deciding to stop waiving a
// CVE, or to accept one in the first place, belongs to whoever accepted it.
func vulnPruneCommand() *cli.Command {
	return &cli.Command{
		Name:  "prune",
		Usage: "Report what the exception lifecycle makes of every entry, and apply it on request",
		Description: "Reads the scanner results the security audit and the container monitor\n" +
			"produce, and classifies every exception: live (covers a repository the\n" +
			"catalogue runs), carry-over (its CVE followed the promotion and the entry\n" +
			"must be re-filed), obsolete (no catalogue image carries it any more) or\n" +
			"unknown (no scan evidence). With --execute, obsolete entries are removed\n" +
			"and carry-over entries are re-filed; nothing else is touched.",
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
				Usage:   "Remove the obsolete entries and re-file the carry-over ones (default: report only)",
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
					ExceptionVerdict: presets.ClassifyException(v.CVE, v.key(), running, findings),
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
// found on it, both keyed by repository — which is what exceptions are keyed by.
//
// The findings are read from result files already produced — security-audit.yml
// scans every catalogue image daily, container-monitor.yml scans them weekly,
// and both upload their JSON. Nothing is scanned here: a command that pulled
// twenty images to answer a bookkeeping question would be run once and never
// again.
//
// A repository appears in the map only when **every** image the catalogue runs
// from it produced a result. The catalogue runs two tags of `rust`, and a CVE
// present on only one of them must not read as absent because the other one
// answered: partial evidence is what makes the verdict unknown rather than
// obsolete, and merging the tags first would hide exactly that.
func catalogueFindings(imagePresets map[string][]string, resultsDir string) ([]string, map[string][]presets.Finding) {
	findings := make(map[string][]presets.Finding)
	unscanned := make(map[string]bool)
	seen := make(map[string]bool)

	var running []string
	for image := range imagePresets {
		repo := imageRepository(image)
		if !seen[repo] {
			seen[repo] = true
			running = append(running, repo)
		}

		found, _, err := scanFindings(resultsDir, image)
		if err != nil {
			unscanned[repo] = true
			continue
		}
		findings[repo] = append(findings[repo], found...)
	}

	for repo := range unscanned {
		delete(findings, repo)
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
	fmt.Printf("Catalogue repositories: %d, of which %d are fully covered by scanner results in %s\n",
		catalogue, scanned, resultsDir)
	fmt.Println()

	sections := []struct {
		state  string
		title  string
		legend string
	}{
		{presets.ExceptionObsolete, "OBSOLETE", "no catalogue image carries these any more — --execute removes them"},
		{presets.ExceptionCarryOver, "CARRY-OVER", "the CVE is carried by the repository named below — --execute re-files the entry onto it"},
		{presets.ExceptionUnknown, "UNKNOWN", "some catalogue image has no scan result, so nothing can be concluded"},
		{presets.ExceptionLive, "LIVE", "covers a repository the catalogue runs today"},
	}

	for _, section := range sections {
		matching := entriesInState(entries, section.state)
		if len(matching) == 0 {
			continue
		}

		fmt.Printf("%s (%d) — %s:\n", section.title, len(matching), section.legend)
		fmt.Println(strings.Repeat("-", 50))
		for _, e := range matching {
			fmt.Printf("  %s | %s", e.CVE, e.key())
			if e.StillOn != "" {
				fmt.Printf(" → %s", e.StillOn)
			}
			if e.FixedIn != "" {
				fmt.Printf("  [fixed upstream in %s]", e.FixedIn)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	if fixable := entriesFixedUpstream(entries); len(fixable) > 0 {
		fmt.Printf("FIXED UPSTREAM (%d) — a fix exists, so these are a question of the image's age,\n", len(fixable))
		fmt.Println("not a decision. The policy says never to write an exception for one; repin the")
		fmt.Println("image instead. Nothing is removed on their account — the entry still waives a")
		fmt.Println("finding the audit would otherwise fail on until the repin happens.")
		fmt.Println(strings.Repeat("-", 50))
		for _, e := range fixable {
			fmt.Printf("  %s | %s | fixed in %s\n", e.CVE, e.StillOn, e.FixedIn)
		}
		fmt.Println()
	}

	if scanned == 0 {
		fmt.Printf("No scanner result was found in %s, so nothing can be shown to have gone.\n", resultsDir)
		fmt.Println("Point --results at the JSON the Security Audit or Container Monitor workflow uploaded.")
		fmt.Println()
	}
}

// applyPrune removes the obsolete entries and re-files the carry-over ones.
//
// Both are mechanical consequences of the CVE criterion, and neither is an
// acceptance decision: an obsolete entry waives nothing because nothing carries
// its CVE, and a re-filed one waives exactly what it always did, against the
// repository that turned out to carry it. Unknown entries stay untouched —
// that is a question nobody has answered, and acting on it would be a guess.
func applyPrune(path string, vulns *VulnerabilityFile, entries []prunedEntry) error {
	obsolete := entriesInState(entries, presets.ExceptionObsolete)
	carried := entriesInState(entries, presets.ExceptionCarryOver)
	if len(obsolete) == 0 && len(carried) == 0 {
		fmt.Println("Nothing to remove and nothing to re-file.")
		return nil
	}

	kept := make([]Vulnerability, 0, len(entries))
	for _, e := range entries {
		switch e.State {
		case presets.ExceptionObsolete:
			continue
		case presets.ExceptionCarryOver:
			kept = append(kept, refile(e))
		default:
			kept = append(kept, e.Vulnerability)
		}
	}
	vulns.Vulnerabilities = resolveRefiled(kept)

	if err := saveVulnerabilities(path, vulns); err != nil {
		return fmt.Errorf("failed to save %s: %w", path, err)
	}

	fmt.Printf("Removed %d obsolete exception(s) and re-filed %d carry-over entr(y/ies) in %s.\n",
		len(obsolete), len(carried), path)
	fmt.Printf("%d entr(y/ies) remain.\n", len(vulns.Vulnerabilities))
	return nil
}

// refile moves an entry onto the repository that carries its CVE, keeping every
// word of the justification and the dates it was argued under. The reference it
// was recorded against becomes `first_seen`, so the entry still says where the
// finding came from.
func refile(e prunedEntry) Vulnerability {
	v := e.Vulnerability
	if v.FirstSeen == "" {
		v.FirstSeen = v.Repository
	}
	v.Repository = e.StillOn
	return v
}

// resolveRefiled collapses entries that now share a key.
//
// Re-filing converges: four entries for the same runc CVE, recorded against four
// images the catalogue has since replaced, all land on the one repository that
// carries it today. Only one of them can be the exception, and the one to keep is
// the one already written about that repository — it is the judgement that was
// made about this image rather than about a neighbour. Failing that, the first in
// file order, so the choice is stable across runs.
func resolveRefiled(entries []Vulnerability) []Vulnerability {
	index := make(map[string]int, len(entries))
	var kept []Vulnerability

	for _, v := range entries {
		key := v.CVE + "|" + v.Repository
		at, seen := index[key]
		if !seen {
			index[key] = len(kept)
			kept = append(kept, v)
			continue
		}
		if imageRepository(v.FirstSeen) == v.Repository && imageRepository(kept[at].FirstSeen) != v.Repository {
			kept[at] = v
		}
	}
	return kept
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

// entriesFixedUpstream lists the entries whose CVE the scanners report a fix
// for. They are not a state — the lifecycle has already placed them — but they
// are the population the policy says must never have been accepted, and the
// report has to name them or the file goes on carrying decisions where a repin
// was the answer.
func entriesFixedUpstream(entries []prunedEntry) []prunedEntry {
	var fixable []prunedEntry
	for _, e := range entries {
		if e.FixedIn != "" {
			fixable = append(fixable, e)
		}
	}
	return fixable
}
