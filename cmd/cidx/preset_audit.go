package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

// auditResult holds the audit result for a single preset.
type auditResult struct {
	Name       string   `json:"name"`
	Image      string   `json:"image"`
	KnownCVEs  int      `json:"known_cves"`
	CVEDetails []string `json:"cve_details,omitempty"`
	HasUpdate  bool     `json:"has_update"`
	LatestTag  string   `json:"latest_tag,omitempty"`
	CurrentTag string   `json:"current_tag"`
	Status     string   `json:"status"` // clean, update-available, cves-known, action-required
	UpdateErr  string   `json:"update_error,omitempty"`
}

func presetAuditCommand() *cli.Command {
	return &cli.Command{
		Name:  "audit",
		Usage: "Cross-reference known CVEs with available updates for compliance",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "vuln-file",
				Value: defaultVulnFile,
				Usage: "Path to known-vulnerabilities.toml",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			vulnFile := c.String("vuln-file")
			jsonOutput := c.Bool("json")

			// Load known vulnerabilities
			knownVulns := make(map[string][]Vulnerability) // image -> vulns

			if vulns, err := loadVulnerabilities(vulnFile); err == nil {
				for _, v := range vulns.Vulnerabilities {
					knownVulns[v.Image] = append(knownVulns[v.Image], v)
				}
			}

			if !jsonOutput {
				fmt.Println("Auditing preset images...")
				fmt.Println()
			}

			var results []auditResult
			var actionRequired, updateAvailable, cvesKnown, clean int

			for _, name := range presets.List() {
				preset, _ := presets.Get(name)
				imageName, currentTag := parseImageTag(preset.Image)

				result := auditResult{
					Name:       name,
					Image:      preset.Image,
					CurrentTag: currentTag,
				}

				// Check known CVEs for this image
				if vulns, ok := knownVulns[preset.Image]; ok {
					result.KnownCVEs = len(vulns)
					for _, v := range vulns {
						result.CVEDetails = append(result.CVEDetails, fmt.Sprintf("%s (%s)", v.CVE, v.Severity))
					}
				}

				// Check for updates
				latestTag, err := getLatestTag(imageName, currentTag)
				if err != nil {
					result.UpdateErr = err.Error()
				} else if latestTag != currentTag && latestTag != "" {
					result.HasUpdate = true
					result.LatestTag = latestTag
				}

				// Determine status
				switch {
				case result.KnownCVEs > 0 && result.HasUpdate:
					result.Status = "action-required"
					actionRequired++
				case result.HasUpdate:
					result.Status = "update-available"
					updateAvailable++
				case result.KnownCVEs > 0:
					result.Status = "cves-known"
					cvesKnown++
				default:
					result.Status = "clean"
					clean++
				}

				results = append(results, result)
			}

			// Sort by severity: action-required first, then update, then cves, then clean
			statusOrder := map[string]int{"action-required": 0, "update-available": 1, "cves-known": 2, "clean": 3}
			sort.Slice(results, func(i, j int) bool {
				if statusOrder[results[i].Status] != statusOrder[results[j].Status] {
					return statusOrder[results[i].Status] < statusOrder[results[j].Status]
				}
				return results[i].Name < results[j].Name
			})

			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(results)
			}

			// Display table
			fmt.Printf("  %-22s %-15s %-10s %-12s %s\n", "Preset", "Current", "CVEs", "Update", "Status")
			fmt.Printf("  %-22s %-15s %-10s %-12s %s\n", "──────", "───────", "────", "──────", "──────")

			for _, r := range results {
				cveStr := "-"
				if r.KnownCVEs > 0 {
					cveStr = fmt.Sprintf("%d known", r.KnownCVEs)
				}

				updateStr := "-"
				if r.HasUpdate {
					updateStr = r.LatestTag
				} else if r.UpdateErr != "" {
					updateStr = "?"
				}

				var statusStr string
				switch r.Status {
				case "action-required":
					statusStr = "\033[31m● action required\033[0m"
				case "update-available":
					statusStr = "\033[33m● update available\033[0m"
				case "cves-known":
					statusStr = "\033[33m● accepted risk\033[0m"
				case "clean":
					statusStr = "\033[32m● clean\033[0m"
				}

				fmt.Printf("  %-22s %-15s %-10s %-12s %s\n", r.Name, r.CurrentTag, cveStr, updateStr, statusStr)

				// Show CVE details for action-required
				if r.Status == "action-required" && len(r.CVEDetails) > 0 {
					fmt.Printf("    └─ %s\n", strings.Join(r.CVEDetails, ", "))
				}
			}

			// Summary
			fmt.Println()
			fmt.Printf("  Total: %d presets | ", len(results))
			if actionRequired > 0 {
				fmt.Printf("\033[31m%d action required\033[0m | ", actionRequired)
			}
			if updateAvailable > 0 {
				fmt.Printf("\033[33m%d updates\033[0m | ", updateAvailable)
			}
			if cvesKnown > 0 {
				fmt.Printf("\033[33m%d accepted\033[0m | ", cvesKnown)
			}
			fmt.Printf("\033[32m%d clean\033[0m\n", clean)

			if actionRequired > 0 {
				return fmt.Errorf("%d preset(s) require action", actionRequired)
			}
			return nil
		},
	}
}
