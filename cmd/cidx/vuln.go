package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

// Vulnerability represents a known vulnerability exception
type Vulnerability struct {
	CVE        string   `toml:"cve"`
	Image      string   `toml:"image"`
	Severity   string   `toml:"severity"`
	Status     string   `toml:"status"`
	Added      string   `toml:"added"`
	Expires    string   `toml:"expires"`
	Notes      string   `toml:"notes"`
	References []string `toml:"references"`
}

// VulnerabilityFile represents the known-vulnerabilities.toml structure
type VulnerabilityFile struct {
	Vulnerabilities []Vulnerability `toml:"vulnerabilities"`
}

const defaultVulnFile = "known-vulnerabilities.toml"

func vulnCommand() *cli.Command {
	return &cli.Command{
		Name:  "vuln",
		Usage: "Manage known vulnerability exceptions",
		Subcommands: []*cli.Command{
			vulnListCommand(),
			vulnCheckCommand(),
			vulnAddCommand(),
			vulnIgnoreCommand(),
		},
	}
}

func vulnListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List all known vulnerability exceptions",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "status",
				Usage: "Filter by status (awaiting-upstream, accepted-risk, mitigated)",
			},
			&cli.StringFlag{
				Name:  "image",
				Usage: "Filter by image",
			},
		},
		Action: func(c *cli.Context) error {
			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				return err
			}

			statusFilter := c.String("status")
			imageFilter := c.String("image")

			fmt.Printf("Known Vulnerability Exceptions\n")
			fmt.Printf("==============================\n\n")

			count := 0
			for _, v := range vulns.Vulnerabilities {
				if statusFilter != "" && v.Status != statusFilter {
					continue
				}
				if imageFilter != "" && v.Image != imageFilter {
					continue
				}

				count++
				printVulnerability(v)
			}

			if count == 0 {
				fmt.Println("No vulnerabilities found matching criteria.")
			} else {
				fmt.Printf("\nTotal: %d vulnerability exception(s)\n", count)
			}

			return nil
		},
	}
}

func vulnCheckCommand() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Check for expired or expiring vulnerability exceptions",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.IntFlag{
				Name:  "days",
				Value: 7,
				Usage: "Warn for exceptions expiring within N days",
			},
		},
		Action: func(c *cli.Context) error {
			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				return err
			}

			warnDays := c.Int("days")
			today := time.Now()
			warnDate := today.AddDate(0, 0, warnDays)

			var expired, expiring, ok []Vulnerability

			for _, v := range vulns.Vulnerabilities {
				expires, err := time.Parse("2006-01-02", v.Expires)
				if err != nil {
					fmt.Printf("Warning: Invalid expiry date for %s: %s\n", v.CVE, v.Expires)
					continue
				}

				if expires.Before(today) {
					expired = append(expired, v)
				} else if expires.Before(warnDate) {
					expiring = append(expiring, v)
				} else {
					ok = append(ok, v)
				}
			}

			hasIssues := false

			if len(expired) > 0 {
				hasIssues = true
				fmt.Printf("EXPIRED (%d) - Require immediate review:\n", len(expired))
				fmt.Println(strings.Repeat("-", 50))
				for _, v := range expired {
					fmt.Printf("  %s | %s | expired %s\n", v.CVE, v.Image, v.Expires)
				}
				fmt.Println()
			}

			if len(expiring) > 0 {
				fmt.Printf("EXPIRING SOON (%d) - Within %d days:\n", len(expiring), warnDays)
				fmt.Println(strings.Repeat("-", 50))
				for _, v := range expiring {
					fmt.Printf("  %s | %s | expires %s\n", v.CVE, v.Image, v.Expires)
				}
				fmt.Println()
			}

			fmt.Printf("OK (%d) - Not expiring soon\n", len(ok))

			if hasIssues {
				return cli.Exit("Expired vulnerability exceptions found - review required", 1)
			}

			return nil
		},
	}
}

func vulnAddCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Add a new vulnerability exception",
		ArgsUsage: "<CVE> <IMAGE>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "severity",
				Value: "HIGH",
				Usage: "Severity level (HIGH, CRITICAL)",
			},
			&cli.StringFlag{
				Name:  "status",
				Value: "awaiting-upstream",
				Usage: "Status (awaiting-upstream, accepted-risk, mitigated)",
			},
			&cli.IntFlag{
				Name:  "expires",
				Value: 30,
				Usage: "Days until expiry (for re-review)",
			},
			&cli.StringFlag{
				Name:  "notes",
				Usage: "Notes explaining the exception",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 2 {
				return cli.Exit("Usage: cidx vuln add <CVE> <IMAGE>", 1)
			}

			cve := c.Args().Get(0)
			image := c.Args().Get(1)

			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				// File might not exist, start fresh
				vulns = &VulnerabilityFile{}
			}

			// Check if already exists
			for _, v := range vulns.Vulnerabilities {
				if v.CVE == cve && v.Image == image {
					return cli.Exit(fmt.Sprintf("Exception already exists for %s on %s", cve, image), 1)
				}
			}

			today := time.Now()
			expires := today.AddDate(0, 0, c.Int("expires"))

			newVuln := Vulnerability{
				CVE:        cve,
				Image:      image,
				Severity:   c.String("severity"),
				Status:     c.String("status"),
				Added:      today.Format("2006-01-02"),
				Expires:    expires.Format("2006-01-02"),
				Notes:      c.String("notes"),
				References: []string{},
			}

			vulns.Vulnerabilities = append(vulns.Vulnerabilities, newVuln)

			// Sort by image, then CVE
			sort.Slice(vulns.Vulnerabilities, func(i, j int) bool {
				if vulns.Vulnerabilities[i].Image != vulns.Vulnerabilities[j].Image {
					return vulns.Vulnerabilities[i].Image < vulns.Vulnerabilities[j].Image
				}
				return vulns.Vulnerabilities[i].CVE < vulns.Vulnerabilities[j].CVE
			})

			if err := saveVulnerabilities(c.String("file"), vulns); err != nil {
				return err
			}

			fmt.Printf("Added exception for %s on %s (expires %s)\n", cve, image, expires.Format("2006-01-02"))
			return nil
		},
	}
}

func vulnIgnoreCommand() *cli.Command {
	return &cli.Command{
		Name:      "ignore",
		Usage:     "Generate ignore file for scanners (trivy/grype) for a specific image",
		ArgsUsage: "[options] <IMAGE>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "format",
				Value: "trivy",
				Usage: "Output format: trivy (.trivyignore) or grype (.grype.yaml)",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output file path (default: stdout)",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 1 {
				return cli.Exit("Usage: cidx vuln ignore [--format trivy|grype] <IMAGE>", 1)
			}

			image := c.Args().Get(0)
			format := c.String("format")

			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				// No file = no exceptions, output empty
				vulns = &VulnerabilityFile{}
			}

			// Filter by image
			var cves []string
			for _, v := range vulns.Vulnerabilities {
				if v.Image == image {
					cves = append(cves, v.CVE)
				}
			}

			var output string
			switch format {
			case "trivy":
				output = generateTrivyIgnore(cves)
			case "grype":
				output = generateGrypeIgnore(cves)
			default:
				return cli.Exit(fmt.Sprintf("Unknown format: %s (use trivy or grype)", format), 1)
			}

			// Write output
			if outPath := c.String("output"); outPath != "" {
				if err := os.WriteFile(outPath, []byte(output), 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", outPath, err)
				}
				fmt.Fprintf(os.Stderr, "Generated %s with %d exception(s) for %s\n", outPath, len(cves), image)
			} else {
				fmt.Print(output)
			}

			return nil
		},
	}
}

func generateTrivyIgnore(cves []string) string {
	if len(cves) == 0 {
		return "# No known vulnerability exceptions\n"
	}

	var sb strings.Builder
	sb.WriteString("# Generated by cidx vuln ignore\n")
	sb.WriteString("# Known vulnerability exceptions - do not scan these CVEs\n\n")
	for _, cve := range cves {
		sb.WriteString(cve)
		sb.WriteString("\n")
	}
	return sb.String()
}

func generateGrypeIgnore(cves []string) string {
	if len(cves) == 0 {
		return "# No known vulnerability exceptions\nignore: []\n"
	}

	var sb strings.Builder
	sb.WriteString("# Generated by cidx vuln ignore\n")
	sb.WriteString("# Known vulnerability exceptions\n\n")
	sb.WriteString("ignore:\n")
	for _, cve := range cves {
		sb.WriteString(fmt.Sprintf("  - vulnerability: %s\n", cve))
	}
	return sb.String()
}

func loadVulnerabilities(path string) (*VulnerabilityFile, error) {
	var vulns VulnerabilityFile
	if _, err := toml.DecodeFile(path, &vulns); err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", path, err)
	}
	return &vulns, nil
}

func saveVulnerabilities(path string, vulns *VulnerabilityFile) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Write header
	header := `# Known Vulnerabilities
#
# Track vulnerabilities that cannot be fixed by updating container images.
# These are reviewed periodically and removed when upstream fixes are available.
#
# Fields:
#   cve        - CVE identifier
#   image      - Affected container image
#   severity   - HIGH or CRITICAL
#   status     - awaiting-upstream | accepted-risk | mitigated
#   added      - Date when this exception was added (YYYY-MM-DD)
#   expires    - Date to re-check (YYYY-MM-DD), typically 30-90 days
#   notes      - Explanation of why this is accepted
#   references - Links to upstream issues/PRs tracking the fix

`
	if _, err := f.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	encoder := toml.NewEncoder(f)
	return encoder.Encode(vulns)
}

func printVulnerability(v Vulnerability) {
	fmt.Printf("%s (%s)\n", v.CVE, v.Severity)
	fmt.Printf("  Image:   %s\n", v.Image)
	fmt.Printf("  Status:  %s\n", v.Status)
	fmt.Printf("  Added:   %s\n", v.Added)
	fmt.Printf("  Expires: %s\n", v.Expires)
	if v.Notes != "" {
		fmt.Printf("  Notes:   %s\n", v.Notes)
	}
	if len(v.References) > 0 {
		fmt.Printf("  Refs:    %v\n", v.References)
	}
	fmt.Println()
}
