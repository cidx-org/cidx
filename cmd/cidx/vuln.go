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
	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// Vulnerability is one accepted HIGH/CRITICAL finding: a judgement about a CVE
// in a repository, in our usage.
//
// It is keyed by **repository and CVE**. The tag it was first seen on is
// context, not identity — an exception dies when its CVE is no longer carried by
// any catalogue image, or when its expiry falls, never because a tag moved
// (#238).
type Vulnerability struct {
	CVE     string   `toml:"cve"`
	Aliases []string `toml:"aliases,omitempty"` // Alternative IDs (GHSA, etc.)

	// Repository is half the key: registry and path, no tag, no digest.
	Repository string `toml:"repository"`

	// FirstSeen is the reference the finding was first recorded against, kept
	// so the entry says where it came from. It is read by humans and by nothing
	// else: no lookup matches on it.
	FirstSeen string `toml:"first_seen,omitempty"`

	Severity   string   `toml:"severity"`
	Status     string   `toml:"status"`
	Added      string   `toml:"added"`
	Expires    string   `toml:"expires"`
	Notes      string   `toml:"notes"`
	References []string `toml:"references"`

	// Image is the pre-#297 key, a whole `repo:tag`. It is read so an
	// un-migrated file still parses, moved to FirstSeen on load, and never
	// written back. An entry that still carries one has no repository, so it
	// matches no catalogue image — which is true of every one of them, and is
	// what `cidx security vuln prune` re-files.
	Image string `toml:"image,omitempty" json:"-"`
}

// key is what the lifecycle matches against the catalogue's repositories.
//
// A migrated entry answers with its repository. An un-migrated one answers with
// the whole `repo:tag` it was recorded against, which equals no repository and
// is therefore judged on its CVE alone — exactly what re-keying it needs, with
// no special case anywhere.
func (v Vulnerability) key() string {
	if v.Repository != "" {
		return v.Repository
	}
	return v.FirstSeen
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
			vulnPruneCommand(),
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
				Usage: "Filter by image or repository",
			},
			&cli.BoolFlag{
				Name:  "stale",
				Usage: "List only entries recorded against a repository the catalogue no longer runs",
			},
		},
		Action: func(c *cli.Context) error {
			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				return err
			}

			statusFilter := c.String("status")
			imageFilter := c.String("image")
			stale := c.Bool("stale")

			entries := vulns.Vulnerabilities
			if stale {
				running, err := catalogueRepositories()
				if err != nil {
					return err
				}
				entries = staleVulnerabilities(entries, running)
				fmt.Printf("Stale Vulnerability Exceptions\n")
				fmt.Printf("==============================\n\n")
			} else {
				fmt.Printf("Known Vulnerability Exceptions\n")
				fmt.Printf("==============================\n\n")
			}

			count := 0
			for _, v := range entries {
				if statusFilter != "" && v.Status != statusFilter {
					continue
				}
				if imageFilter != "" && v.Repository != imageRepository(imageFilter) {
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

			if stale && count > 0 {
				fmt.Printf("\nThese record a repository no catalogue preset runs, so they match nothing\n")
				fmt.Printf("and waive nothing. Nothing is removed automatically: delete the entries\n")
				fmt.Printf("you have reviewed from %s.\n", c.String("file"))
			}

			return nil
		},
	}
}

// staleVulnerabilities returns the exceptions recorded against a repository the
// catalogue no longer runs.
//
// A repository the catalogue left behind can waive nothing, and until #248
// nothing ever said so: the file accumulated dead records and rule 3's CVE
// waiver went quiet along with them. Re-keying by repository (#238) removed the
// bulk of this — a promotion inside the same repository no longer orphans
// anything — but a repository genuinely replaced still leaves entries behind.
//
// Deciding a record is dead is a judgement call — the CVE may still be carried
// elsewhere — so this only reports. `vuln prune` is what resolves it.
func staleVulnerabilities(vulns []Vulnerability, running map[string]bool) []Vulnerability {
	var stale []Vulnerability
	for _, v := range vulns {
		if !running[v.Repository] {
			stale = append(stale, v)
		}
	}
	return stale
}

// catalogueRepositories is the set of repositories the catalogue runs today, in
// the form known-vulnerabilities.toml records exceptions against.
func catalogueRepositories() (map[string]bool, error) {
	images, err := catalogueImages()
	if err != nil {
		return nil, err
	}

	repos := make(map[string]bool, len(images))
	for image := range images {
		repos[imageRepository(image)] = true
	}
	return repos, nil
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
					fmt.Printf("  %s | %s | expired %s\n", v.CVE, v.Repository, v.Expires)
				}
				fmt.Println()
			}

			if len(expiring) > 0 {
				fmt.Printf("EXPIRING SOON (%d) - Within %d days:\n", len(expiring), warnDays)
				fmt.Println(strings.Repeat("-", 50))
				for _, v := range expiring {
					fmt.Printf("  %s | %s | expires %s\n", v.CVE, v.Repository, v.Expires)
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

			switch groupBy {
			case "cve":
				return reportByCVE(vulns, jsonOutput)
			case "image":
				return reportByImage(vulns, jsonOutput)
			default:
				return fmt.Errorf("invalid group-by value: %s (use cve or image)", groupBy)
			}
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
		cveMap[v.CVE].Images = append(cveMap[v.CVE].Images, v.Repository)
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
		switch c.Severity {
		case "CRITICAL":
			criticalCount++
		case "HIGH":
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
		if _, exists := imageMap[v.Repository]; !exists {
			imageMap[v.Repository] = &imageInfo{
				Image: v.Repository,
				CVEs:  []string{},
			}
		}
		imageMap[v.Repository].CVEs = append(imageMap[v.Repository].CVEs, v.CVE)
		switch v.Severity {
		case "CRITICAL":
			imageMap[v.Repository].Critical++
		case "HIGH":
			imageMap[v.Repository].High++
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
			&cli.StringFlag{
				Name:  "results",
				Value: "scan-results",
				Usage: "Directory holding the scanner result files, read to refuse an exception for a CVE that is fixed upstream",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() < 2 {
				return cli.Exit("Usage: cidx vuln add <CVE> <IMAGE>", 1)
			}

			cve := c.Args().Get(0)
			// The exception is keyed by repository: the judgement is about a
			// CVE in an image, in our usage, and none of that changes when the
			// tag moves. The reference given here is kept as context — scanners
			// paste back the digest-pinned form (#242), and the digest is not
			// part of what a human wants to read.
			reference := refWithoutDigest(c.Args().Get(1))
			repository := imageRepository(reference)

			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				// File might not exist, start fresh
				vulns = &VulnerabilityFile{}
			}

			// Check if already exists
			for _, v := range vulns.Vulnerabilities {
				if v.CVE == cve && v.Repository == repository {
					return cli.Exit(fmt.Sprintf("Exception already exists for %s on %s", cve, repository), 1)
				}
			}

			if err := refuseIfFixedUpstream(cve, c.Args().Get(1), c.String("results")); err != nil {
				return err
			}

			today := time.Now()
			expires := today.AddDate(0, 0, c.Int("expires"))

			newVuln := Vulnerability{
				CVE:        cve,
				Repository: repository,
				FirstSeen:  reference,
				Severity:   c.String("severity"),
				Status:     c.String("status"),
				Added:      today.Format("2006-01-02"),
				Expires:    expires.Format("2006-01-02"),
				Notes:      c.String("notes"),
				References: []string{},
			}

			vulns.Vulnerabilities = append(vulns.Vulnerabilities, newVuln)

			if err := saveVulnerabilities(c.String("file"), vulns); err != nil {
				return err
			}

			fmt.Printf("Added exception for %s on %s (first seen on %s, expires %s)\n",
				cve, repository, reference, expires.Format("2006-01-02"))
			return nil
		},
	}
}

// refuseIfFixedUpstream stops an exception being written for a vulnerability the
// publisher has already fixed.
//
// Question 2 of the policy: a fix upstream means the finding disappears when the
// image is rebuilt, so the question is the image's age, not the vulnerability.
// Recording an exception there would file a decision where there is only a wait,
// and it would keep waiving the finding for the ninety days nobody spends
// bumping the image. The policy says never, so this refuses rather than warns —
// there is no `--force`, because the correct action is a repin, and it is not
// one this command performs.
//
// With no scan result for the image the check cannot run, and the exception is
// written: refusing on missing evidence would make the command unusable
// wherever the audit's artifacts are not to hand. This is the one place the
// fail-closed posture does not apply, because nothing is being promoted or
// deleted — the entry is still argued again at its expiry.
func refuseIfFixedUpstream(cve, image, resultsDir string) error {
	found, scanners, err := scanFindings(resultsDir, image)
	if err != nil {
		fmt.Printf("No scanner result for %s in %s, so whether %s is fixed upstream could not be checked.\n",
			image, resultsDir, cve)
		return nil
	}

	fix := presets.FixVersion(found, cve)
	if fix == "" {
		return nil
	}

	return cli.Exit(fmt.Sprintf(
		"%s is fixed upstream in %s (%s), so it is a question of the image's age, not a decision to record.\n"+
			"An exception here would waive a finding that goes away on the next rebuild. Repin %s instead.",
		cve, fix, strings.Join(scanners, " and "), imageRepository(image)), 1)
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

			// security-audit.yml passes the catalogue reference straight
			// through, digest included (#242) — the exception is keyed by
			// repository, so both the tag and the digest are dropped here.
			// An exception written for `rust` therefore covers both
			// `rust:1.97.0` and `rust:1.97.0-slim`, which is the point.
			repository := imageRepository(c.Args().Get(0))
			format := c.String("format")

			vulns, err := loadVulnerabilities(c.String("file"))
			if err != nil {
				// No file = no exceptions, output empty
				vulns = &VulnerabilityFile{}
			}

			var filtered []Vulnerability
			for _, v := range vulns.Vulnerabilities {
				if v.Repository == repository {
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
				fmt.Fprintf(os.Stderr, "Generated %s with %d exception(s) for %s\n", outPath, len(filtered), repository)
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
		fmt.Fprintf(&sb, "  - vulnerability: %s\n", v.CVE)
		for _, alias := range v.Aliases {
			fmt.Fprintf(&sb, "  - vulnerability: %s\n", alias)
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
				imageExceptions[v.Repository] = append(imageExceptions[v.Repository], v)
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

	for i, v := range vulns.Vulnerabilities {
		vulns.Vulnerabilities[i] = migrateVulnerability(v)
	}
	return &vulns, nil
}

// migrateVulnerability moves a pre-#297 entry onto the repository key without
// guessing what its repository should be.
//
// The old `image = "repo:tag"` becomes `first_seen`, and the repository is left
// empty on purpose: the tag it names is one the catalogue no longer runs, and
// deriving `golangci/golangci-lint` from `golangci/golangci-lint:v2.6.2` would
// file twelve kernel CVEs against an image that stopped carrying them the day it
// moved to Alpine — they are on the Rust images now. Which repository actually
// carries the CVE is a question the scan results answer, and
// `cidx security vuln prune` is what asks it.
func migrateVulnerability(v Vulnerability) Vulnerability {
	if v.Image == "" {
		return v
	}
	if v.FirstSeen == "" {
		v.FirstSeen = refWithoutDigest(v.Image)
	}
	v.Image = ""
	return v
}

// saveVulnerabilities writes the file back, deterministically.
//
// The TOML encoder is deliberately not used. It re-indents and re-orders
// everything it touches, so removing 101 entries showed up as 538 insertions and
// 1552 deletions and nobody could tell from the diff what had actually been
// dropped (#289). The whole point of the report-then-apply split is that the
// diff is reviewable, and a fixed layout written by hand is what makes a purge
// look like a purge.
//
// Entries are sorted by repository then CVE, so two runs on the same content
// produce the same bytes and an added entry lands next to its neighbours instead
// of at the end.
func saveVulnerabilities(path string, vulns *VulnerabilityFile) error {
	entries := deduplicateVulnerabilities(vulns.Vulnerabilities)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Repository != entries[j].Repository {
			return entries[i].Repository < entries[j].Repository
		}
		return entries[i].CVE < entries[j].CVE
	})
	vulns.Vulnerabilities = entries

	var sb strings.Builder
	sb.WriteString(vulnFileHeader)
	for _, v := range entries {
		sb.WriteString("\n[[vulnerabilities]]\n")
		writeTOMLString(&sb, "cve", v.CVE)
		writeTOMLList(&sb, "aliases", v.Aliases)
		writeTOMLString(&sb, "repository", v.Repository)
		writeTOMLString(&sb, "first_seen", v.FirstSeen)
		writeTOMLString(&sb, "severity", v.Severity)
		writeTOMLString(&sb, "status", v.Status)
		writeTOMLString(&sb, "added", v.Added)
		writeTOMLString(&sb, "expires", v.Expires)
		writeTOMLString(&sb, "notes", v.Notes)
		fmt.Fprintf(&sb, "  references = %s\n", tomlList(v.References))
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

const vulnFileHeader = `# Known Vulnerabilities
#
# Accepted HIGH/CRITICAL findings on the images the built-in preset catalogue
# runs. An entry is a judgement about a CVE in a repository, in our usage: it is
# keyed by repository and CVE, and it dies when no catalogue image carries the
# CVE any more, or when it expires — never because a tag moved.
#
# A vulnerability that is fixed upstream never belongs here: that is a question
# about the image's age, not a decision. "cidx security vuln add" refuses one.
#
# Fields:
#   cve        - CVE (or GHSA) identifier
#   repository - Affected repository, without tag or digest
#   first_seen - The reference it was first recorded against (context only)
#   severity   - HIGH or CRITICAL
#   status     - awaiting-upstream | accepted-risk | mitigated | third-party
#   added      - Date when this exception was added (YYYY-MM-DD)
#   expires    - Date to re-check (YYYY-MM-DD), typically 30-90 days
#   notes      - Explanation of why this is accepted
#   references - Links to upstream issues/PRs tracking the fix
#
# Generated by "cidx security vuln add" and "cidx security vuln prune"; hand edits
# survive as long as they follow this layout.
`

// writeTOMLString emits one key, skipping it entirely when empty — an absent
// optional field says less than an empty one.
func writeTOMLString(sb *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(sb, "  %s = %s\n", key, tomlQuote(value))
}

func writeTOMLList(sb *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(sb, "  %s = %s\n", key, tomlList(values))
}

func tomlList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, tomlQuote(v))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// tomlQuote writes a TOML basic string. Only the escapes a justification can
// realistically contain are handled; a control character in a CVE note would be
// a bug upstream of here.
func tomlQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + replacer.Replace(value) + `"`
}

func printVulnerability(v Vulnerability) {
	fmt.Printf("%s (%s)\n", v.CVE, v.Severity)
	fmt.Printf("  Repo:    %s\n", v.Repository)
	if v.FirstSeen != "" {
		fmt.Printf("  Seen on: %s\n", v.FirstSeen)
	}
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

// deduplicateVulnerabilities removes duplicate entries by (CVE, image) pair.
// When duplicates exist, the last entry wins (most recently added).
func deduplicateVulnerabilities(vulns []Vulnerability) []Vulnerability {
	seen := make(map[string]int) // key -> index in result
	var result []Vulnerability

	for _, v := range vulns {
		key := v.CVE + "|" + v.Repository
		if idx, exists := seen[key]; exists {
			// Replace with newer entry
			result[idx] = v
		} else {
			seen[key] = len(result)
			result = append(result, v)
		}
	}

	return result
}
