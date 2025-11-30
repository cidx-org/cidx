package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/pkg/presets"
	"github.com/urfave/cli/v2"
)

func presetCommand() *cli.Command {
	return &cli.Command{
		Name:  "preset",
		Usage: "Manage built-in container presets",
		Subcommands: []*cli.Command{
			presetListCommand(),
			presetInfoCommand(),
			presetShowCommand(),
			presetExportCommand(),
			presetSearchCommand(),
			presetCheckUpdatesCommand(),
			presetScanCommand(),
		},
	}
}

// presetListCommand lists all available presets grouped by phase
func presetListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List available presets grouped by phase",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "phase",
				Aliases: []string{"p"},
				Usage:   "Filter by phase (security, code, test, build, docker, release)",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			phaseFilter := c.String("phase")
			phases := presets.GroupByPhase()

			if c.Bool("json") {
				return printPresetsJSON(phases, phaseFilter)
			}

			fmt.Println("Available presets:")
			fmt.Println()

			// Get sorted phase names
			phaseNames := make([]string, 0, len(phases))
			for phase := range phases {
				if phaseFilter == "" || phase == phaseFilter {
					phaseNames = append(phaseNames, phase)
				}
			}
			sort.Strings(phaseNames)

			if len(phaseNames) == 0 && phaseFilter != "" {
				return fmt.Errorf("no presets found for phase '%s'", phaseFilter)
			}

			// Display by phase
			for _, phase := range phaseNames {
				tools := phases[phase]
				sort.Strings(tools)

				fmt.Printf("  %s:\n", phase)
				for _, tool := range tools {
					fmt.Printf("    - %s\n", tool)
				}
				fmt.Println()
			}

			return nil
		},
	}
}

// presetInfoCommand shows detailed information about a preset
func presetInfoCommand() *cli.Command {
	return &cli.Command{
		Name:      "info",
		Usage:     "Show detailed information about a preset",
		ArgsUsage: "<preset>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: preset name")
			}

			presetName := c.Args().First()
			preset, err := presets.Get(presetName)
			if err != nil {
				return err
			}

			fmt.Printf("Preset: %s\n", preset.Name)
			fmt.Printf("Phase: %s\n", preset.Phase)
			fmt.Printf("Image: %s\n", preset.Image)
			fmt.Printf("Command: %s\n", preset.Command)
			fmt.Printf("Workdir: %s\n", preset.Workdir)
			fmt.Println()

			if len(preset.Volumes) > 0 {
				fmt.Println("Volumes:")
				for _, vol := range preset.Volumes {
					fmt.Printf("  - %s\n", vol)
				}
				fmt.Println()
			}

			if len(preset.Env) > 0 {
				fmt.Println("Environment:")
				for k, v := range preset.Env {
					fmt.Printf("  %s=%s\n", k, v)
				}
				fmt.Println()
			}

			if len(preset.ConfigFiles) > 0 {
				fmt.Println("Config files:")
				for _, file := range preset.ConfigFiles {
					fmt.Printf("  - %s\n", file)
				}
				fmt.Println()
			}

			if len(preset.Options) > 0 {
				fmt.Println("Options:")
				for name, opt := range preset.Options {
					fmt.Printf("  %s:\n", name)
					fmt.Printf("    Type: %s\n", opt.Type)
					fmt.Printf("    Default: %v\n", opt.Default)
					if opt.Description != "" {
						fmt.Printf("    Description: %s\n", opt.Description)
					}
					if opt.EnvVar != "" {
						fmt.Printf("    Environment: %s\n", opt.EnvVar)
					}
					if opt.CommandFlag != "" {
						fmt.Printf("    Flag: %s\n", opt.CommandFlag)
					}
				}
				fmt.Println()
			}

			return nil
		},
	}
}

// presetShowCommand shows the raw TOML definition of a preset
func presetShowCommand() *cli.Command {
	return &cli.Command{
		Name:      "show",
		Usage:     "Show raw TOML definition of a preset",
		ArgsUsage: "<preset>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: preset name")
			}

			presetName := c.Args().First()
			preset, err := presets.Get(presetName)
			if err != nil {
				return err
			}

			// Convert preset to TOML-friendly structure
			presetMap := map[string]presets.Preset{
				presetName: preset,
			}

			fmt.Printf("# Embedded preset: %s\n", presetName)
			fmt.Println()

			encoder := toml.NewEncoder(os.Stdout)
			encoder.Indent = ""
			return encoder.Encode(map[string]interface{}{
				"presets": presetMap,
			})
		},
	}
}

// presetExportCommand exports all presets to a file
func presetExportCommand() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export all embedded presets to a TOML file",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output file path (default: stdout)",
			},
			&cli.StringFlag{
				Name:    "phase",
				Aliases: []string{"p"},
				Usage:   "Export only presets from specific phase",
			},
		},
		Action: func(c *cli.Context) error {
			phaseFilter := c.String("phase")
			outputPath := c.String("output")

			// Collect presets
			allPresets := make(map[string]presets.Preset)
			for _, name := range presets.List() {
				preset, _ := presets.Get(name)
				if phaseFilter == "" || preset.Phase == phaseFilter {
					allPresets[name] = preset
				}
			}

			if len(allPresets) == 0 {
				return fmt.Errorf("no presets found")
			}

			// Determine output destination
			var output *os.File
			if outputPath == "" {
				output = os.Stdout
			} else {
				var err error
				output, err = os.Create(outputPath)
				if err != nil {
					return fmt.Errorf("failed to create output file: %w", err)
				}
				defer func() { _ = output.Close() }()
			}

			// Write header
			_, _ = fmt.Fprintln(output, "# CIDX Embedded Presets Export")
			_, _ = fmt.Fprintln(output, "# Generated from compiled binary")
			_, _ = fmt.Fprintf(output, "# Total presets: %d\n", len(allPresets))
			if phaseFilter != "" {
				_, _ = fmt.Fprintf(output, "# Filtered by phase: %s\n", phaseFilter)
			}
			_, _ = fmt.Fprintln(output)

			// Encode presets
			encoder := toml.NewEncoder(output)
			encoder.Indent = ""
			err := encoder.Encode(map[string]interface{}{
				"presets": allPresets,
			})

			if outputPath != "" && err == nil {
				fmt.Fprintf(os.Stderr, "Exported %d presets to %s\n", len(allPresets), outputPath)
			}

			return err
		},
	}
}

// presetSearchCommand searches for presets matching a keyword
func presetSearchCommand() *cli.Command {
	return &cli.Command{
		Name:      "search",
		Usage:     "Search presets by name, image, or description",
		ArgsUsage: "<keyword>",
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: search keyword")
			}

			keyword := strings.ToLower(c.Args().First())
			var matches []presets.Preset

			for _, name := range presets.List() {
				preset, _ := presets.Get(name)

				// Search in name, image, command, phase
				searchable := strings.ToLower(fmt.Sprintf("%s %s %s %s",
					preset.Name, preset.Image, preset.Command, preset.Phase))

				// Also search in option descriptions
				for _, opt := range preset.Options {
					searchable += " " + strings.ToLower(opt.Description)
				}

				if strings.Contains(searchable, keyword) {
					matches = append(matches, preset)
				}
			}

			if len(matches) == 0 {
				fmt.Printf("No presets found matching '%s'\n", keyword)
				return nil
			}

			fmt.Printf("Found %d preset(s) matching '%s':\n\n", len(matches), keyword)

			// Sort by name
			sort.Slice(matches, func(i, j int) bool {
				return matches[i].Name < matches[j].Name
			})

			for _, preset := range matches {
				fmt.Printf("  \033[1m%s\033[0m (%s)\n", preset.Name, preset.Phase)
				fmt.Printf("    Image: %s\n", preset.Image)
				fmt.Printf("    Command: %s\n", truncate(preset.Command, 60))
				fmt.Println()
			}

			return nil
		},
	}
}

// printPresetsJSON outputs presets as JSON
func printPresetsJSON(phases map[string][]string, phaseFilter string) error {
	fmt.Println("{")

	phaseNames := make([]string, 0, len(phases))
	for phase := range phases {
		if phaseFilter == "" || phase == phaseFilter {
			phaseNames = append(phaseNames, phase)
		}
	}
	sort.Strings(phaseNames)

	for i, phase := range phaseNames {
		tools := phases[phase]
		sort.Strings(tools)

		fmt.Printf("  \"%s\": [", phase)
		for j, tool := range tools {
			fmt.Printf("\"%s\"", tool)
			if j < len(tools)-1 {
				fmt.Print(", ")
			}
		}
		fmt.Print("]")
		if i < len(phaseNames)-1 {
			fmt.Print(",")
		}
		fmt.Println()
	}

	fmt.Println("}")
	return nil
}

// truncate shortens a string to max length
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// presetCheckUpdatesCommand checks for available updates for container images
func presetCheckUpdatesCommand() *cli.Command {
	return &cli.Command{
		Name:  "check-updates",
		Usage: "Check for available updates for container images",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			jsonOutput := c.Bool("json")

			if !jsonOutput {
				fmt.Println("Checking container image versions...")
				fmt.Println()
			}

			type updateResult struct {
				Name      string `json:"name"`
				Image     string `json:"image"`      // Full image reference (with tag)
				ImageBase string `json:"image_base"` // Image without tag
				Current   string `json:"current"`
				Latest    string `json:"latest"`
				HasUpdate bool   `json:"has_update"`
				Error     string `json:"error,omitempty"`
			}

			var results []updateResult
			var updatesAvailable int

			for _, name := range presets.List() {
				preset, _ := presets.Get(name)

				// Parse image
				imageName, currentTag := parseImageTag(preset.Image)

				// Get latest tag
				latestTag, err := getLatestTag(imageName, currentTag)

				result := updateResult{
					Name:      name,
					Image:     preset.Image, // Full image reference
					ImageBase: imageName,    // Image without tag
					Current:   currentTag,
				}

				if err != nil {
					result.Error = err.Error()
					result.Latest = "?"
				} else {
					result.Latest = latestTag
					result.HasUpdate = latestTag != currentTag && latestTag != ""
					if result.HasUpdate {
						updatesAvailable++
					}
				}

				results = append(results, result)
			}

			// Sort by name
			sort.Slice(results, func(i, j int) bool {
				return results[i].Name < results[j].Name
			})

			if c.Bool("json") {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(results)
			}

			// Print results
			for _, r := range results {
				if r.Error != "" {
					fmt.Printf("  \033[33m%-20s\033[0m %s\n", r.Name, r.Error)
				} else if r.HasUpdate {
					fmt.Printf("  \033[32m%-20s\033[0m %s → %s \033[32m⬆️  Update available\033[0m\n",
						r.Name, r.Current, r.Latest)
				} else {
					fmt.Printf("  %-20s %s \033[90m✓ Up to date\033[0m\n", r.Name, r.Current)
				}
			}

			fmt.Println()
			if updatesAvailable > 0 {
				fmt.Printf("📦 %d update(s) available\n", updatesAvailable)
			} else {
				fmt.Println("✅ All containers are up to date")
			}

			return nil
		},
	}
}

// parseImageTag splits an image reference into name and tag
func parseImageTag(image string) (name, tag string) {
	// Handle images with digest
	if idx := strings.Index(image, "@"); idx != -1 {
		return image[:idx], image[idx+1:]
	}

	// Handle images with tag
	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// Make sure it's not a port number (registry:port/image)
		afterColon := image[idx+1:]
		if !strings.Contains(afterColon, "/") {
			return image[:idx], afterColon
		}
	}

	return image, "latest"
}

// getLatestTag fetches the latest tag for an image from its registry
// currentTag is used to preserve variant suffixes (e.g., -alpine, -slim)
func getLatestTag(image, currentTag string) (string, error) {
	// Determine registry and repository
	registry, repo := parseRegistry(image)

	// Extract variant suffix from current tag (e.g., "1.24-alpine" -> "-alpine")
	variantSuffix := extractVariantSuffix(currentTag)

	switch registry {
	case "docker.io":
		return getDockerHubLatestTag(repo, variantSuffix)
	case "quay.io":
		return getQuayLatestTag(repo, variantSuffix)
	case "gcr.io", "ghcr.io":
		// GitHub/Google Container Registry - harder to query without auth
		return "", fmt.Errorf("registry %s not supported yet", registry)
	default:
		return "", fmt.Errorf("unknown registry: %s", registry)
	}
}

// extractVariantSuffix extracts the variant suffix from a tag
// e.g., "1.24-alpine" -> "-alpine", "v2.3.0" -> ""
func extractVariantSuffix(tag string) string {
	// Common variant patterns
	variants := []string{"-alpine", "-slim", "-bullseye", "-bookworm", "-buster", "-jammy", "-focal"}
	for _, v := range variants {
		if strings.HasSuffix(tag, v) {
			return v
		}
	}
	return ""
}

// parseRegistry extracts registry and repository from image name
func parseRegistry(image string) (registry, repo string) {
	parts := strings.SplitN(image, "/", 2)

	// Check if first part looks like a registry
	if len(parts) == 2 && (strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":")) {
		return parts[0], parts[1]
	}

	// Docker Hub official images (e.g., "alpine", "golang")
	if len(parts) == 1 {
		return "docker.io", "library/" + parts[0]
	}

	// Docker Hub user images (e.g., "user/repo")
	return "docker.io", image
}

// getDockerHubLatestTag gets the latest tag from Docker Hub
// variantSuffix is the variant to match (e.g., "-alpine", "" for pure semver)
func getDockerHubLatestTag(repo, variantSuffix string) (string, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=100&ordering=last_updated", repo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Build regex based on variant suffix
	// For "-alpine": match "X.Y-alpine" or "X.Y.Z-alpine"
	// For "": match pure "X.Y" or "X.Y.Z"
	var semverRegex *regexp.Regexp
	if variantSuffix != "" {
		// Escape the suffix for regex and require it at the end
		escapedSuffix := regexp.QuoteMeta(variantSuffix)
		semverRegex = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?` + escapedSuffix + `$`)
	} else {
		// Pure semver only
		semverRegex = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?$`)
	}

	for _, tag := range result.Results {
		if tag.Name != "latest" && !strings.Contains(tag.Name, "sha") &&
			!strings.Contains(tag.Name, "nightly") && semverRegex.MatchString(tag.Name) {
			return tag.Name, nil
		}
	}

	// No matching semver tag found
	return "", fmt.Errorf("no semver tags found with suffix '%s'", variantSuffix)
}

// getQuayLatestTag gets the latest tag from Quay.io
// variantSuffix is the variant to match (e.g., "-alpine", "" for pure semver)
func getQuayLatestTag(repo, variantSuffix string) (string, error) {
	url := fmt.Sprintf("https://quay.io/api/v1/repository/%s/tag/?limit=50", repo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Build regex based on variant suffix
	var semverRegex *regexp.Regexp
	if variantSuffix != "" {
		escapedSuffix := regexp.QuoteMeta(variantSuffix)
		semverRegex = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?` + escapedSuffix + `$`)
	} else {
		semverRegex = regexp.MustCompile(`^v?[0-9]+\.[0-9]+(\.[0-9]+)?$`)
	}

	for _, tag := range result.Tags {
		if tag.Name != "latest" && semverRegex.MatchString(tag.Name) {
			return tag.Name, nil
		}
	}

	return "", fmt.Errorf("no semver tags found with suffix '%s'", variantSuffix)
}

// presetScanCommand scans all preset container images for security vulnerabilities
func presetScanCommand() *cli.Command {
	return &cli.Command{
		Name:  "scan",
		Usage: "Scan all preset container images for security vulnerabilities",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "scanner",
				Aliases: []string{"s"},
				Usage:   "Scanner to use: trivy, grype, or all (default: all)",
				Value:   "all",
			},
			&cli.StringFlag{
				Name:    "severity",
				Usage:   "Minimum severity to report: LOW, MEDIUM, HIGH, CRITICAL",
				Value:   "HIGH",
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
			&cli.StringFlag{
				Name:    "preset",
				Aliases: []string{"p"},
				Usage:   "Scan only a specific preset",
			},
		},
		Action: func(c *cli.Context) error {
			scanner := c.String("scanner")
			severity := strings.ToUpper(c.String("severity"))
			jsonOutput := c.Bool("json")
			presetFilter := c.String("preset")

			// Validate scanner choice
			if scanner != "trivy" && scanner != "grype" && scanner != "all" {
				return fmt.Errorf("invalid scanner: %s (use trivy, grype, or all)", scanner)
			}

			// Check if docker is available
			if _, err := exec.LookPath("docker"); err != nil {
				return fmt.Errorf("docker is required but not found in PATH")
			}

			type scanResult struct {
				Name        string `json:"name"`
				Image       string `json:"image"`
				TrivyStatus string `json:"trivy_status,omitempty"`
				GrypeStatus string `json:"grype_status,omitempty"`
				Vulnerable  bool   `json:"vulnerable"`
				Error       string `json:"error,omitempty"`
			}

			var results []scanResult
			var vulnerableCount int

			// Get list of presets to scan
			presetNames := presets.List()
			if presetFilter != "" {
				// Check if preset exists
				if _, err := presets.Get(presetFilter); err != nil {
					return err
				}
				presetNames = []string{presetFilter}
			}

			if !jsonOutput {
				fmt.Printf("Scanning %d container image(s) with %s...\n\n", len(presetNames), scanner)
			}

			for _, name := range presetNames {
				preset, _ := presets.Get(name)

				result := scanResult{
					Name:  name,
					Image: preset.Image,
				}

				if !jsonOutput {
					fmt.Printf("Scanning %s (%s)...\n", name, preset.Image)
				}

				hasVuln := false

				// Run Trivy scan
				if scanner == "trivy" || scanner == "all" {
					trivyResult := runTrivyScan(preset.Image, severity)
					result.TrivyStatus = trivyResult
					if trivyResult != "clean" {
						hasVuln = true
					}
					if !jsonOutput {
						if trivyResult == "clean" {
							fmt.Printf("  Trivy: clean\n")
						} else {
							fmt.Printf("  Trivy: %s\n", trivyResult)
						}
					}
				}

				// Run Grype scan
				if scanner == "grype" || scanner == "all" {
					grypeResult := runGrypeScan(preset.Image, severity)
					result.GrypeStatus = grypeResult
					if grypeResult != "clean" {
						hasVuln = true
					}
					if !jsonOutput {
						if grypeResult == "clean" {
							fmt.Printf("  Grype: clean\n")
						} else {
							fmt.Printf("  Grype: %s\n", grypeResult)
						}
					}
				}

				result.Vulnerable = hasVuln
				if hasVuln {
					vulnerableCount++
				}
				results = append(results, result)

				if !jsonOutput {
					fmt.Println()
				}
			}

			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(results)
			}

			// Print summary
			fmt.Println("========================================")
			fmt.Printf("Scanned: %d containers\n", len(results))
			fmt.Printf("Clean: %d\n", len(results)-vulnerableCount)
			fmt.Printf("Vulnerable: %d\n", vulnerableCount)
			fmt.Println("========================================")

			if vulnerableCount > 0 {
				return fmt.Errorf("%d container(s) have vulnerabilities", vulnerableCount)
			}

			return nil
		},
	}
}

// runTrivyScan runs Trivy security scan on an image
func runTrivyScan(image, severity string) string {
	cmd := exec.Command("docker", "run", "--rm",
		"aquasec/trivy:latest", "image",
		"--severity", severity+",CRITICAL",
		"--exit-code", "1",
		"--quiet",
		image)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return fmt.Sprintf("vulnerabilities found (%s+)", severity)
			}
		}
		return fmt.Sprintf("error: %v", err)
	}
	_ = output
	return "clean"
}

// runGrypeScan runs Grype security scan on an image
func runGrypeScan(image, severity string) string {
	failOn := strings.ToLower(severity)
	cmd := exec.Command("docker", "run", "--rm",
		"anchore/grype:latest",
		image,
		"--fail-on", failOn,
		"--quiet")

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return fmt.Sprintf("vulnerabilities found (%s+)", severity)
			}
		}
		return fmt.Sprintf("error: %v", err)
	}
	_ = output
	return "clean"
}
