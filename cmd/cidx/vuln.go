package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/urfave/cli/v2"
)

// Vulnerability represents a known vulnerability exception
type Vulnerability struct {
	CVE        string   `toml:"cve"`
	Aliases    []string `toml:"aliases,omitempty"` // Alternative IDs (GHSA, etc.)
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
			vulnReportCommand(),
			vulnAddCommand(),
			vulnIgnoreCommand(),
			vulnVerifyCommand(),
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
			&cli.BoolFlag{
				Name:  "remove-expired",
				Usage: "Remove expired exceptions from the file",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			filePath := c.String("file")
			vulns, err := loadVulnerabilities(filePath)
			if err != nil {
				return err
			}

			warnDays := c.Int("days")
			today := time.Now()
			warnDate := today.AddDate(0, 0, warnDays)
			removeExpired := c.Bool("remove-expired")
			jsonOutput := c.Bool("json")

			var expired, expiring, ok []Vulnerability

			for _, v := range vulns.Vulnerabilities {
				expires, err := time.Parse("2006-01-02", v.Expires)
				if err != nil {
					if !jsonOutput {
						fmt.Printf("Warning: Invalid expiry date for %s: %s\n", v.CVE, v.Expires)
					}
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

			// JSON output
			if jsonOutput {
				type checkResult struct {
					Expired  []Vulnerability `json:"expired"`
					Expiring []Vulnerability `json:"expiring"`
					Ok       []Vulnerability `json:"ok"`
					Removed  int             `json:"removed,omitempty"`
				}
				result := checkResult{
					Expired:  expired,
					Expiring: expiring,
					Ok:       ok,
				}
				if removeExpired {
					result.Removed = len(expired)
				}
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
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

			// Remove expired if requested
			if removeExpired && len(expired) > 0 {
				// Keep only non-expired entries
				vulns.Vulnerabilities = append(expiring, ok...)
				sort.Slice(vulns.Vulnerabilities, func(i, j int) bool {
					if vulns.Vulnerabilities[i].Image != vulns.Vulnerabilities[j].Image {
						return vulns.Vulnerabilities[i].Image < vulns.Vulnerabilities[j].Image
					}
					return vulns.Vulnerabilities[i].CVE < vulns.Vulnerabilities[j].CVE
				})

				if err := saveVulnerabilities(filePath, vulns); err != nil {
					return fmt.Errorf("failed to save updated file: %w", err)
				}
				fmt.Printf("\n✓ Removed %d expired exception(s) from %s\n", len(expired), filePath)
				return nil // Don't fail after cleanup
			}

			if hasIssues {
				return cli.Exit("Expired vulnerability exceptions found - review required", 1)
			}

			return nil
		},
	}
}

func vulnReportCommand() *cli.Command {
	return &cli.Command{
		Name:  "report",
		Usage: "Generate consolidated vulnerability report across all images",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "group-by",
				Value: "cve",
				Usage: "Group by: cve (show images per CVE) or image (show CVEs per image)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				return err
			}

			groupBy := c.String("group-by")
			jsonOutput := c.Bool("json")

			if groupBy == "cve" {
				return reportByCVE(vulns, jsonOutput)
			} else if groupBy == "image" {
				return reportByImage(vulns, jsonOutput)
			}
			return fmt.Errorf("invalid group-by value: %s (use cve or image)", groupBy)
		},
	}
}

func reportByCVE(vulns *VulnerabilityFile, jsonOutput bool) error {
	// Group vulnerabilities by CVE
	type cveInfo struct {
		CVE      string   `json:"cve"`
		Severity string   `json:"severity"`
		Images   []string `json:"images"`
		Status   string   `json:"status"`
		Notes    string   `json:"notes,omitempty"`
	}

	cveMap := make(map[string]*cveInfo)
	for _, v := range vulns.Vulnerabilities {
		if _, exists := cveMap[v.CVE]; !exists {
			cveMap[v.CVE] = &cveInfo{
				CVE:      v.CVE,
				Severity: v.Severity,
				Status:   v.Status,
				Notes:    v.Notes,
				Images:   []string{},
			}
		}
		cveMap[v.CVE].Images = append(cveMap[v.CVE].Images, v.Image)
	}

	// Sort CVEs by severity (CRITICAL first) then by name
	cves := make([]*cveInfo, 0, len(cveMap))
	for _, info := range cveMap {
		sort.Strings(info.Images)
		cves = append(cves, info)
	}
	sort.Slice(cves, func(i, j int) bool {
		// CRITICAL > HIGH > MEDIUM > LOW
		sevOrder := map[string]int{"CRITICAL": 0, "HIGH": 1, "MEDIUM": 2, "LOW": 3}
		si, sj := sevOrder[cves[i].Severity], sevOrder[cves[j].Severity]
		if si != sj {
			return si < sj
		}
		return cves[i].CVE < cves[j].CVE
	})

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(cves)
	}

	// Summary stats
	criticalCount := 0
	highCount := 0
	multiImageCount := 0
	for _, c := range cves {
		if c.Severity == "CRITICAL" {
			criticalCount++
		} else if c.Severity == "HIGH" {
			highCount++
		}
		if len(c.Images) > 1 {
			multiImageCount++
		}
	}

	fmt.Printf("Vulnerability Report (grouped by CVE)\n")
	fmt.Printf("=====================================\n\n")
	fmt.Printf("Summary: %d unique CVEs (%d CRITICAL, %d HIGH)\n", len(cves), criticalCount, highCount)
	fmt.Printf("         %d CVEs affect multiple images\n\n", multiImageCount)

	for _, c := range cves {
		marker := ""
		if len(c.Images) > 1 {
			marker = fmt.Sprintf(" [%d images]", len(c.Images))
		}
		fmt.Printf("%s (%s)%s\n", c.CVE, c.Severity, marker)
		for _, img := range c.Images {
			fmt.Printf("  └─ %s\n", img)
		}
		if c.Notes != "" {
			fmt.Printf("  📝 %s\n", c.Notes)
		}
		fmt.Println()
	}

	return nil
}

func reportByImage(vulns *VulnerabilityFile, jsonOutput bool) error {
	// Group vulnerabilities by image
	type imageInfo struct {
		Image    string   `json:"image"`
		Critical int      `json:"critical"`
		High     int      `json:"high"`
		CVEs     []string `json:"cves"`
	}

	imageMap := make(map[string]*imageInfo)
	for _, v := range vulns.Vulnerabilities {
		if _, exists := imageMap[v.Image]; !exists {
			imageMap[v.Image] = &imageInfo{
				Image: v.Image,
				CVEs:  []string{},
			}
		}
		imageMap[v.Image].CVEs = append(imageMap[v.Image].CVEs, v.CVE)
		if v.Severity == "CRITICAL" {
			imageMap[v.Image].Critical++
		} else if v.Severity == "HIGH" {
			imageMap[v.Image].High++
		}
	}

	// Sort images by vulnerability count (most first)
	images := make([]*imageInfo, 0, len(imageMap))
	for _, info := range imageMap {
		sort.Strings(info.CVEs)
		images = append(images, info)
	}
	sort.Slice(images, func(i, j int) bool {
		// Sort by critical first, then high, then total
		if images[i].Critical != images[j].Critical {
			return images[i].Critical > images[j].Critical
		}
		if images[i].High != images[j].High {
			return images[i].High > images[j].High
		}
		return len(images[i].CVEs) > len(images[j].CVEs)
	})

	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(images)
	}

	fmt.Printf("Vulnerability Report (grouped by image)\n")
	fmt.Printf("=======================================\n\n")
	fmt.Printf("Summary: %d images with known vulnerabilities\n", len(images))
	fmt.Printf("         %d total vulnerability exceptions\n\n", len(vulns.Vulnerabilities))

	for _, img := range images {
		fmt.Printf("%s\n", img.Image)
		fmt.Printf("  %d CRITICAL, %d HIGH (%d total)\n", img.Critical, img.High, len(img.CVEs))
		for _, cve := range img.CVEs {
			fmt.Printf("  └─ %s\n", cve)
		}
		fmt.Println()
	}

	return nil
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
			var filtered []Vulnerability
			for _, v := range vulns.Vulnerabilities {
				if v.Image == image {
					filtered = append(filtered, v)
				}
			}

			var output string
			switch format {
			case "trivy":
				output = generateTrivyIgnore(filtered)
			case "grype":
				output = generateGrypeIgnore(filtered)
			default:
				return cli.Exit(fmt.Sprintf("Unknown format: %s (use trivy or grype)", format), 1)
			}

			// Write output
			if outPath := c.String("output"); outPath != "" {
				if err := os.WriteFile(outPath, []byte(output), 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", outPath, err)
				}
				fmt.Fprintf(os.Stderr, "Generated %s with %d exception(s) for %s\n", outPath, len(filtered), image)
			} else {
				fmt.Print(output)
			}

			return nil
		},
	}
}

func generateTrivyIgnore(vulns []Vulnerability) string {
	if len(vulns) == 0 {
		return "# No known vulnerability exceptions\n"
	}

	var sb strings.Builder
	sb.WriteString("# Generated by cidx vuln ignore\n")
	sb.WriteString("# Known vulnerability exceptions - do not scan these CVEs\n\n")
	for _, v := range vulns {
		// Trivy uses CVE identifiers
		sb.WriteString(v.CVE)
		sb.WriteString("\n")
	}
	return sb.String()
}

func generateGrypeIgnore(vulns []Vulnerability) string {
	if len(vulns) == 0 {
		return "# No known vulnerability exceptions\nignore: []\n"
	}

	var sb strings.Builder
	sb.WriteString("# Generated by cidx vuln ignore\n")
	sb.WriteString("# Known vulnerability exceptions\n\n")
	sb.WriteString("ignore:\n")
	for _, v := range vulns {
		// Grype can use CVE or GHSA identifiers - add all aliases
		sb.WriteString(fmt.Sprintf("  - vulnerability: %s\n", v.CVE))
		for _, alias := range v.Aliases {
			sb.WriteString(fmt.Sprintf("  - vulnerability: %s\n", alias))
		}
	}
	return sb.String()
}

func vulnVerifyCommand() *cli.Command {
	return &cli.Command{
		Name:  "verify",
		Usage: "Verify vulnerability exceptions work by scanning images with Trivy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to vulnerability file",
			},
			&cli.StringFlag{
				Name:  "image",
				Usage: "Only verify specific image (default: all images with exceptions)",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be scanned without running scans",
			},
		},
		Action: func(c *cli.Context) error {
			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				return err
			}

			// Get unique images with exceptions
			imageExceptions := make(map[string][]Vulnerability)
			for _, v := range vulns.Vulnerabilities {
				imageExceptions[v.Image] = append(imageExceptions[v.Image], v)
			}

			if len(imageExceptions) == 0 {
				fmt.Println("No vulnerability exceptions to verify.")
				return nil
			}

			imageFilter := c.String("image")
			dryRun := c.Bool("dry-run")

			fmt.Println("Verifying vulnerability exceptions...")
			fmt.Println()

			var failed []string
			var passed []string

			for image, imgVulns := range imageExceptions {
				if imageFilter != "" && image != imageFilter {
					continue
				}

				fmt.Printf("Image: %s (%d exceptions)\n", image, len(imgVulns))
				for _, v := range imgVulns {
					fmt.Printf("  - %s\n", v.CVE)
				}

				if dryRun {
					fmt.Println("  → [dry-run] Would scan with Trivy")
					fmt.Println()
					continue
				}

				// Generate ignore file
				trivyIgnore := generateTrivyIgnore(imgVulns)
				ignoreFile := fmt.Sprintf("/tmp/.trivyignore-%d", os.Getpid())
				if err := os.WriteFile(ignoreFile, []byte(trivyIgnore), 0644); err != nil {
					return fmt.Errorf("failed to write ignore file: %w", err)
				}
				defer func() { _ = os.Remove(ignoreFile) }()

				// Run Trivy scan
				fmt.Printf("  → Scanning with Trivy... ")

				cmd := exec.Command("docker", "run", "--rm",
					"-v", ignoreFile+":/root/.trivyignore:ro",
					"aquasec/trivy:latest", "image",
					"--severity", "HIGH,CRITICAL",
					"--ignorefile", "/root/.trivyignore",
					"--exit-code", "1",
					"--quiet",
					image,
				)

				output, err := cmd.CombinedOutput()
				if err != nil {
					fmt.Println("FAILED")
					fmt.Printf("  → Error: %s\n", strings.TrimSpace(string(output)))
					failed = append(failed, image)
				} else {
					fmt.Println("OK")
					passed = append(passed, image)
				}
				fmt.Println()
			}

			// Summary
			fmt.Println("=" + strings.Repeat("=", 50))
			fmt.Printf("Results: %d passed, %d failed\n", len(passed), len(failed))

			if len(failed) > 0 {
				fmt.Println("\nFailed images:")
				for _, img := range failed {
					fmt.Printf("  - %s\n", img)
				}
				fmt.Println("\nThese images still have vulnerabilities not covered by exceptions.")
				fmt.Println("Add missing CVEs with: cidx vuln add <CVE> <IMAGE>")
				return cli.Exit("Verification failed", 1)
			}

			fmt.Println("\nAll exceptions verified successfully!")
			return nil
		},
	}
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
