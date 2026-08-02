package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// securitySarifCommand renders the catalogue's scan results as SARIF for GitHub
// code scanning (#301).
//
// The command is the I/O half: it reads the JSON the audit already uploaded,
// resolves the line each alert should point at, and writes the file. What is
// published and why lives in `pkg/presets/sarif.go`, next to the triage it
// reuses — the tab and `SECURITY-BASELINE.md` read the same classification, so
// they cannot come to disagree about what the catalogue carries.
//
// The scanners are not asked a second time. Both emit SARIF natively and neither
// is usable here: what makes the tab readable is what it leaves out, and no
// scanner knows which of its findings this project has already answered.
func securitySarifCommand() *cli.Command {
	return &cli.Command{
		Name:  "sarif",
		Usage: "Render what the catalogue needs a human for as SARIF, for GitHub code scanning",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "results",
				Value: "scan-results",
				Usage: "Directory holding the scanner result files",
			},
			&cli.StringFlag{
				Name:  "file",
				Value: presets.ExceptionsFile,
				Usage: "Path to known-vulnerabilities.toml",
			},
			&cli.StringFlag{
				Name:  "presets",
				Value: presets.CatalogueFile,
				Usage: "Path to the catalogue an alert points at",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   "catalogue.sarif",
				Usage:   "Where to write the SARIF",
			},
		},
		Action: func(c *cli.Context) error {
			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			// An absent exception file is not an error: it means nothing is
			// accepted, and therefore that nothing has lapsed.
			var accepted []Vulnerability
			if vulns, err := loadVulnerabilities(c.String("file")); err == nil {
				accepted = vulns.Vulnerabilities
			}

			resultsDir := c.String("results")
			alerts, scanned, missing := catalogueAlerts(imagePresets, resultsDir, accepted,
				c.String("presets"), c.String("file"), time.Now())

			encoded, err := json.MarshalIndent(presets.SARIF(alerts), "", "  ")
			if err != nil {
				return fmt.Errorf("failed to encode SARIF: %w", err)
			}

			out := c.String("output")
			if err := os.WriteFile(out, append(encoded, '\n'), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", out, err)
			}

			fmt.Printf("Wrote %s: %d alert(s) across %d scanned image(s).\n", out, len(alerts), scanned)

			// An image nobody scanned carries no alerts, and that is not the
			// same as carrying none. The audit already fails the leg that could
			// not scan; saying it again where the count is read is what stops a
			// drop in alerts being mistaken for progress — the same confusion
			// `not scanned` exists to remove in SECURITY-BASELINE.md.
			if len(missing) > 0 {
				fmt.Printf("No scan result for %d image(s), which carry no alerts here and are not thereby clean:\n  %s\n",
					len(missing), strings.Join(missing, "\n  "))
			}
			return nil
		},
	}
}

// catalogueAlerts assembles every alert the run should publish, and names the
// images no scanner answered for.
func catalogueAlerts(
	imagePresets map[string][]string,
	resultsDir string,
	accepted []Vulnerability,
	presetsFile, vulnFile string,
	today time.Time,
) (alerts []presets.Alert, scanned int, missing []string) {
	catalogueLines := tomlValueLines(presetsFile, "image")

	images := make([]string, 0, len(imagePresets))
	for image := range imagePresets {
		images = append(images, image)
	}
	sort.Strings(images)

	for _, image := range images {
		found, _, err := scanFindings(resultsDir, image)
		if err != nil {
			missing = append(missing, image)
			continue
		}
		scanned++
		alerts = append(alerts, presets.TriageAlerts(image, imagePresets[image], found, catalogueLines[image])...)
	}

	alerts = append(alerts, presets.ExpiredExceptionAlerts(exceptionsFor(accepted, vulnFile), today)...)
	return alerts, scanned, missing
}

// exceptionsFor reduces the accepted entries to what an alert about one says,
// and finds the line each sits on so the alert points at the entry to re-argue.
//
// Only HIGH and CRITICAL: that is the band the policy acts on, the band the
// audit gates on, and the band the baseline claims to cover.
func exceptionsFor(accepted []Vulnerability, vulnFile string) []presets.Exception {
	lines := vulnerabilityLines(vulnFile)

	var out []presets.Exception
	for _, v := range accepted {
		if !isHighOrCritical(v.Severity) {
			continue
		}
		out = append(out, presets.Exception{
			CVE:        v.CVE,
			Repository: v.Repository,
			Severity:   v.Severity,
			Status:     v.Status,
			Added:      v.Added,
			Expires:    v.Expires,
			// The justification is quoted in the alert body, so a newline in it
			// would end the quote rather than continue it.
			Notes: strings.NewReplacer("\n", " ", "\r", "").Replace(v.Notes),
			Line:  lines[v.Repository+" "+strings.ToUpper(v.CVE)],
		})
	}
	return out
}

// tomlValueLines maps the value of every `key = "value"` line in a TOML file to
// its line number.
//
// Read as text rather than parsed, because a parse loses exactly what is wanted
// here: the position. A missing file yields an empty map and every alert then
// points at line 1 — a worse link, not a wrong report.
func tomlValueLines(path, key string) map[string]int {
	lines := map[string]int{}
	data, err := os.ReadFile(path)
	if err != nil {
		return lines
	}

	prefix := key + " = "
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		if value := strings.Trim(strings.TrimPrefix(trimmed, prefix), `"`); value != "" {
			if _, seen := lines[value]; !seen {
				lines[value] = i + 1
			}
		}
	}
	return lines
}

// vulnerabilityLines maps "<repository> <CVE>" to the line its `cve = ` sits on.
//
// Both keys are read from the same `[[vulnerabilities]]` block, because neither
// is unique on its own: one CVE is accepted on several repositories, and one
// repository accepts several CVEs.
func vulnerabilityLines(path string) map[string]int {
	lines := map[string]int{}
	data, err := os.ReadFile(path)
	if err != nil {
		return lines
	}

	var cve string
	var cveLine int
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "cve = "):
			cve = strings.ToUpper(strings.Trim(strings.TrimPrefix(trimmed, "cve = "), `"`))
			cveLine = i + 1
		case strings.HasPrefix(trimmed, "repository = ") && cve != "":
			repository := strings.Trim(strings.TrimPrefix(trimmed, "repository = "), `"`)
			lines[repository+" "+cve] = cveLine
			cve = ""
		}
	}
	return lines
}
