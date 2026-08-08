package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

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
				imageName, currentTag, _ := parseImageRef(preset.Image)

				// Get latest tag
				update, err := getLatestTag(imageName, currentTag, time.Now())

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
					result.Latest = update.Latest
					result.HasUpdate = update.Latest != currentTag && update.Latest != ""
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

// tagUpdate is what a registry has to say about the versions of one image:
// the newest tag worth pinning, when it was published, and — when there is no
// newer tag at all — whether the variant line the catalogue pins still exists.
type tagUpdate struct {
	// Latest is the newest tag in the pinned variant family, or currentTag when
	// nothing newer is on offer.
	Latest string

	// Published is when Latest became publicly available. The zero time means
	// the registry does not say; see registryDatesTags.
	Published time.Time

	// Superseding names the variant family that replaced the pinned one, on a
	// repository that stopped publishing it entirely (#252). Empty in every
	// other case, including the ordinary one where the family is simply at its
	// own head.
	Superseding string

	// CurrentPublished is when the tag the catalogue pins was itself published.
	//
	// The cooldown reads the *candidate's* date and asks "is it old enough to
	// trust?". Nobody was asking the mirror-image question — "is what we run
	// suspiciously old?" — and four of the five images that reported "nothing
	// to update" in #238 were in fact stale or abandoned, prettier by thirteen
	// months. The listing carries a date per tag, so this costs no request
	// (issue #282). Zero when the registry publishes none.
	CurrentPublished time.Time
}

// getLatestTag fetches the latest tag for an image from its registry, together
// with the date that version was published.
//
// Every registry answers the same two things — the tag names, and a date per
// tag where it publishes one — so every registry is read the same way: fetch
// the listing, then let presets.NewerTag say which name qualifies. Docker Hub
// and Quay.io used to pick their own way instead, matching a bare semver regex
// against a listing ordered by push date and keeping the first hit. That took
// the newest *push* rather than the newest version, and its idea of a variant
// family was a hardcoded list of seven suffixes — so `buildpack-deps:26.10`,
// an Ubuntu development branch, was offered as the successor to a Debian
// `trixie-curl` whose suffix the list did not know (#328).
//
// The date is what rule 2 of the image supply-chain policy (#242) measures the
// cooldown against, and it comes from the registry's own tag listing — the same
// call that finds the tag, so the cooldown costs no extra request. A zero time
// means this registry does not publish one; callers must treat that as "not
// promotable" rather than as "old enough".
//
// The OCI distribution API itself carries no publication date: the closest
// thing is the `created` field of the image config blob, which is a *build*
// date and can predate publication by an arbitrary amount. It is not used here
// — a wrong date would silently shorten the cooldown, which is worse than
// having none. registryDatesTags names the registries that do publish one.
func getLatestTag(image, currentTag string, now time.Time) (tagUpdate, error) {
	// A reference pinned by digest alone carries no version to compare against.
	if currentTag == "" {
		return tagUpdate{}, fmt.Errorf("no tag to compare (reference is pinned by digest only)")
	}

	registry, repo := parseRegistry(image)

	var (
		listing tagListing
		err     error
	)
	switch registry {
	case "docker.io":
		listing, err = getDockerHubTags(repo)
	case "quay.io":
		listing, err = getQuayTags(repo)
	case "gcr.io", "ghcr.io", "dhi.io":
		listing, err = listTags(image)
	default:
		return tagUpdate{}, fmt.Errorf("unknown registry: %s", registry)
	}
	if err != nil {
		return tagUpdate{}, err
	}
	return newestOf(currentTag, listing, now), nil
}

// newestOf reads a tag listing for the newest version that qualifies (#245).
//
// It returns currentTag when nothing newer is on offer — including for a tag
// that carries no version at all, where an update would be a guess.
//
// "Nothing newer" is not always good news, so the same listing answers the
// second question while it is in hand, at no extra request: is the variant line
// still published at all (#252)?
func newestOf(currentTag string, listing tagListing, now time.Time) tagUpdate {
	newest := presets.NewerTag(currentTag, listing.Tags, now)
	if newest == "" {
		return tagUpdate{
			Latest:           currentTag,
			Superseding:      presets.SupersedingVariant(currentTag, listing.Tags),
			CurrentPublished: listing.Published[currentTag],
		}
	}
	return tagUpdate{
		Latest:           newest,
		Published:        listing.Published[newest],
		CurrentPublished: listing.Published[currentTag],
	}
}

// registryDatesTags reports whether a registry says when a version became
// publicly available — the date rule 2 of the image supply-chain policy
// measures its cooldown against (docs/core-concepts/supply-chain-policy.md).
//
// Docker Hub (`last_updated`), Quay.io (`start_ts`) and gcr.io
// (`timeUploadedMs`) do. ghcr.io and dhi.io do not, and there is nothing left
// to fall back on: ghcr.io's dates sit behind a GitHub Packages API call that
// needs a `read:packages` token and answers 403 for another org's package,
// dhi.io has no repository on hub.docker.com to ask, and the config blob's
// `created` is a build date the policy rejects.
//
// This is what tells buildScanTargets that a newer version found there can
// never clear the cooldown, so it must be reported as unpromotable instead of
// becoming a candidate held for ever (#245).
func registryDatesTags(image string) bool {
	registry, _ := parseRegistry(image)

	switch registry {
	case "docker.io", "quay.io", "gcr.io":
		return true
	default:
		return false
	}
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

// getDockerHubTags lists a Docker Hub repository's tags, with the date each one
// last received content.
//
// `last_updated` is when that tag was last pushed, which is exactly the moment
// the version became publicly available. A tag republished with new content
// resets it, and for the cooldown that is the right behaviour: new content,
// new wait.
func getDockerHubTags(repo string) (tagListing, error) {
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=100&ordering=last_updated", repo)

	var result struct {
		Results []struct {
			Name        string `json:"name"`
			LastUpdated string `json:"last_updated"`
		} `json:"results"`
	}
	if err := getRegistryJSON(url, &result); err != nil {
		return tagListing{}, err
	}

	listing := tagListing{Published: make(map[string]time.Time)}
	for _, tag := range result.Results {
		listing.Tags = append(listing.Tags, tag.Name)
		// An unparseable date yields the zero time, not an error: the tag is
		// still a real candidate, it just cannot serve the cooldown.
		if published, err := time.Parse(time.RFC3339, tag.LastUpdated); err == nil {
			listing.Published[tag.Name] = published
		}
	}
	return listing, nil
}

// getQuayTags lists a Quay.io repository's tags, with the date each was pushed
// (`start_ts`, seconds since the epoch — when the tag started pointing at its
// current content).
func getQuayTags(repo string) (tagListing, error) {
	url := fmt.Sprintf("https://quay.io/api/v1/repository/%s/tag/?limit=50", repo)

	var result struct {
		Tags []struct {
			Name    string `json:"name"`
			StartTS int64  `json:"start_ts"`
		} `json:"tags"`
	}
	if err := getRegistryJSON(url, &result); err != nil {
		return tagListing{}, err
	}

	listing := tagListing{Published: make(map[string]time.Time)}
	for _, tag := range result.Tags {
		listing.Tags = append(listing.Tags, tag.Name)
		if tag.StartTS > 0 {
			listing.Published[tag.Name] = time.Unix(tag.StartTS, 0).UTC()
		}
	}
	return listing, nil
}

// getRegistryJSON reads one registry API document into payload. Docker Hub and
// Quay.io both answer plain JSON over an unauthenticated GET, which is the
// whole of what they have in common with each other and nothing they share with
// the v2 listing in registry_tags.go.
func getRegistryJSON(url string, payload any) error {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(payload)
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
				Name:  "severity",
				Usage: "Minimum severity to report: LOW, MEDIUM, HIGH, CRITICAL",
				Value: "HIGH",
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
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Show container logs in real-time",
			},
		},
		Action: func(c *cli.Context) error {
			scanner := c.String("scanner")
			severity := strings.ToUpper(c.String("severity"))
			jsonOutput := c.Bool("json")
			presetFilter := c.String("preset")
			verbose := c.Bool("verbose")

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
					trivyResult := runTrivyScan(preset.Image, severity, verbose)
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
					grypeResult := runGrypeScan(preset.Image, severity, verbose)
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
func runTrivyScan(image, severity string, verbose bool) string {
	args := []string{"run", "--rm", "aquasec/trivy:latest", "image",
		"--severity", severity + ",CRITICAL",
		"--exit-code", "1"}

	// In non-verbose mode, add --quiet flag
	if !verbose {
		args = append(args, "--quiet")
	}
	args = append(args, image)

	cmd := exec.Command("docker", args...)

	if verbose {
		// Stream output in real-time
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() == 1 {
					return fmt.Sprintf("vulnerabilities found (%s+)", severity)
				}
			}
			return fmt.Sprintf("error: %v", err)
		}
		return "clean"
	}

	// Non-verbose: capture output silently
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
func runGrypeScan(image, severity string, verbose bool) string {
	failOn := strings.ToLower(severity)
	args := []string{"run", "--rm", "anchore/grype:latest", image, "--fail-on", failOn}

	// In non-verbose mode, add --quiet flag
	if !verbose {
		args = append(args, "--quiet")
	}

	cmd := exec.Command("docker", args...)

	if verbose {
		// Stream output in real-time
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				if exitErr.ExitCode() == 1 {
					return fmt.Sprintf("vulnerabilities found (%s+)", severity)
				}
			}
			return fmt.Sprintf("error: %v", err)
		}
		return "clean"
	}

	// Non-verbose: capture output silently
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

// presetImagesCommand lists unique container images used by presets
func presetImagesCommand() *cli.Command {
	return &cli.Command{
		Name:  "images",
		Usage: "List unique container images used by presets (deduplicated)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Show which presets use each image",
			},
			&cli.BoolFlag{
				Name:  "catalogue",
				Usage: "List only the built-in catalogue, leaving out user and project presets",
			},
		},
		Action: func(c *cli.Context) error {
			jsonOutput := c.Bool("json")
			verbose := c.Bool("verbose")

			// Build map of image -> presets using it. The default is the
			// resolved registry, like `preset list` and `preset scan`: in a
			// project, the images its own presets run are images it uses.
			// --catalogue narrows it to what presets.toml ships, which is what
			// the supply-chain policy governs and all security-audit.yml should
			// ever scan -- the same leak scan-targets had in #248.
			imagePresets := registryImages()
			if c.Bool("catalogue") {
				var err error
				if imagePresets, err = catalogueImages(); err != nil {
					return err
				}
			}

			// Get sorted unique images
			images := make([]string, 0, len(imagePresets))
			for img := range imagePresets {
				images = append(images, img)
			}
			sort.Strings(images)

			if jsonOutput {
				type imageInfo struct {
					Image   string   `json:"image"`
					Presets []string `json:"presets"`
				}
				var result []imageInfo
				for _, img := range images {
					presetsUsing := imagePresets[img]
					sort.Strings(presetsUsing)
					result = append(result, imageInfo{
						Image:   img,
						Presets: presetsUsing,
					})
				}
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(result)
			}

			// Calculate deduplication stats
			totalPresets := 0
			for _, using := range imagePresets {
				totalPresets += len(using)
			}
			uniqueImages := len(images)
			duplicates := totalPresets - uniqueImages

			fmt.Printf("Unique container images: %d (from %d presets, %d deduplicated)\n\n",
				uniqueImages, totalPresets, duplicates)

			for _, img := range images {
				presetsUsing := imagePresets[img]
				if verbose || len(presetsUsing) > 1 {
					sort.Strings(presetsUsing)
					fmt.Printf("  %s\n", img)
					fmt.Printf("    └─ Used by: %s\n", strings.Join(presetsUsing, ", "))
				} else {
					fmt.Printf("  %s\n", img)
				}
			}

			return nil
		},
	}
}

// scanTarget is one line of the contract between `cidx preset scan-targets` and
// container-monitor.yml: what the workflow scans, and whether it may promote it.
//
// The JSON names are consumed by jq expressions in that workflow;
// TestScanTargetJSONContract pins them.
type scanTarget struct {
	CurrentImage string   `json:"current_image"`
	ScanImage    string   `json:"scan_image"`
	IsUpdate     bool     `json:"is_update"`
	Presets      []string `json:"presets"`
	Error        string   `json:"error,omitempty"`

	// Rules 2 and 3 of the image supply-chain policy (#242). CandidateImage is
	// set whenever a newer version exists and could be pinned, promoted or not
	// — a version held by the cooldown has to stay visible, or the policy
	// quietly swallows it. PolicyReason says which of the two happened.
	CandidateImage string   `json:"candidate_image,omitempty"`
	PublishedAt    string   `json:"published_at,omitempty"`
	AgeDays        *int     `json:"age_days,omitempty"`
	PolicyReason   string   `json:"policy_reason,omitempty"`
	CVEWaiver      []string `json:"cve_waiver,omitempty"`

	// NewerVersion names a version the registry lists but will not date (#245).
	// It is deliberately not a CandidateImage: the cooldown could never be
	// shown to have elapsed for it, so offering it as a candidate would hold
	// it for ever and repeat the same line in every weekly run. It is reported
	// as a version to look at by hand, which is a different thing.
	NewerVersion string `json:"newer_version,omitempty"`

	// Missing reports that the pinned reference no longer resolves — the
	// preset cannot run at all. #244 found two catalogue images deleted
	// upstream, and nothing had noticed until the presets failed.
	Missing bool `json:"missing,omitempty"`

	// FrozenVariant names the variant family that replaced the pinned one, on a
	// repository that publishes the pinned family no more (#252). The image
	// still pulls, so it is neither missing nor a candidate — it is a line that
	// will never receive another fix, which "up to date" hid.
	FrozenVariant string `json:"frozen_variant,omitempty"`

	// UnversionedTag reports a pinned tag carrying no version at all —
	// `buildpack-deps:trixie-curl`, `koalaman/shellcheck:stable`. Nothing a
	// registry lists can ever be newer than a name, so the promotion path,
	// which compares versions end to end, has nothing to say about these
	// images: their updates arrive as rebuilds of the same tag under a new
	// digest, and nothing here detects that (#328). Reported so the silence
	// does not read as "up to date".
	UnversionedTag bool `json:"unversioned_tag,omitempty"`

	// StaleTag reports that the pinned tag has not been republished in over a
	// year. Orthogonal to everything above: the image resolves, its family is
	// alive, no newer version exists — and it may still be abandoned. The
	// cooldown reads a candidate's date to ask whether it is old enough to
	// trust; nothing asked whether what we run is suspiciously old, and four
	// of the five images that reported "nothing to update" in #238 were stale
	// (issue #282).
	StaleTag string `json:"stale_tag,omitempty"`

	// Rebuilt reports that the pinned tag resolves to a different digest today
	// than the one pinned beside it: the same name, different content. The
	// other half of what #328 could only describe — an unversioned tag's
	// updates arrive this way and nothing detected them — and, on a versioned
	// tag, an anomaly worth a question upstream (issue #332). Never promoted,
	// only reported: see presets.RebuiltTag.
	Rebuilt string `json:"rebuilt,omitempty"`
}

// The two network calls scan-targets makes, as package variables so the
// promotion policy can be exercised without a registry — the seam convention
// already used for listRunsFunc (#170).
var (
	latestTagFunc     = getLatestTag
	resolveDigestFunc = resolveDigest
)

// presetScanTargetsCommand returns deduplicated list of images to scan (with updates resolved)
func presetScanTargetsCommand() *cli.Command {
	return &cli.Command{
		Name:  "scan-targets",
		Usage: "Get deduplicated list of images to scan (checks for updates, returns new or current)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON (required for CI)",
				Value: true,
			},
			&cli.StringFlag{
				Name:  "vuln-file",
				Value: defaultVulnFile,
				Usage: "Path to known-vulnerabilities.toml (source of the cooldown's CVE exception)",
			},
		},
		Action: func(c *cli.Context) error {
			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			targets := buildScanTargets(imagePresets, knownHighSeverityCVEs(c.String("vuln-file")), time.Now())

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(targets)
		},
	}
}

// catalogueImages maps each distinct image of the built-in catalogue to the
// presets running it.
//
// It reads the catalogue rather than the resolved registry, which also carries
// the user's and the project's own presets: those are governed by nobody but
// their author, and the promotion job's `sed` against pkg/presets/presets.toml
// silently matched nothing for them while inflating the candidate count (#248).
// registryImages maps each distinct image of the resolved preset registry to
// the presets running it: the catalogue plus whatever the user (~/.config/cidx)
// and the project (.cidx/) merged on top.
func registryImages() map[string][]string {
	imagePresets := make(map[string][]string)
	for _, name := range presets.List() {
		preset, _ := presets.Get(name)
		imagePresets[preset.Image] = append(imagePresets[preset.Image], name)
	}
	return imagePresets
}

func catalogueImages() (map[string][]string, error) {
	catalogue, err := presets.Catalogue()
	if err != nil {
		return nil, fmt.Errorf("failed to load the preset catalogue: %w", err)
	}

	imagePresets := make(map[string][]string)
	for name, preset := range catalogue {
		imagePresets[preset.Image] = append(imagePresets[preset.Image], name)
	}
	return imagePresets, nil
}

// buildScanTargets resolves, for every distinct catalogue image, what to scan
// and whether a newer version may replace it.
//
// affectingUs maps a repository (as known-vulnerabilities.toml keys exceptions,
// #238) to the vulnerabilities already known to affect it.
func buildScanTargets(imagePresets map[string][]string, affectingUs map[string][]string, now time.Time) []scanTarget {
	currentImages := make([]string, 0, len(imagePresets))
	for img := range imagePresets {
		currentImages = append(currentImages, img)
	}
	sort.Strings(currentImages)

	targets := make([]scanTarget, 0, len(currentImages))

	for _, currentImage := range currentImages {
		presetsUsing := imagePresets[currentImage]
		sort.Strings(presetsUsing)

		target := scanTarget{
			CurrentImage: currentImage,
			ScanImage:    currentImage, // Default: scan what we run today
			IsUpdate:     false,
			Presets:      presetsUsing,
		}

		// Check for update
		imageName, currentTag, currentDigest := parseImageRef(currentImage)
		imageRegistry, _ := parseRegistry(imageName)
		update, err := latestTagFunc(imageName, currentTag, now)
		latestTag, published := update.Latest, update.Published

		// Asked outside the switch on purpose: an image can be at the head of
		// its family, on a version no tag supersedes, and still be abandoned.
		// The switch below decides what to promote; this says what to look at
		// (#282).
		if reason, stale := presets.StaleTag(update.CurrentPublished, now, currentTag); stale {
			target.StaleTag = reason
		}

		// Asked outside the switch for the same reason, and it is the one
		// question the pin itself makes unaskable anywhere else: the switch
		// compares tags, and a rebuild changes no tag. Only when a digest is
		// actually pinned — with nothing pinned there is nothing to have moved
		// away from (#332).
		if currentDigest != "" && currentTag != "" {
			// A separate call from verifyPinnedImage below, which resolves the
			// digest: that one asks whether what we run still exists, this one
			// asks what the name points at now. Two questions, and the pin is
			// exactly what makes them different.
			if live, digestErr := resolveDigestFunc(imageName, currentTag); digestErr == nil {
				if reason, rebuilt := presets.RebuiltTag(currentDigest, live, currentTag, update.CurrentPublished, now); rebuilt {
					target.Rebuilt = reason
				}
			}
			// A lookup that fails says nothing rather than "rebuilt": an
			// absence of evidence must not read as evidence, which is the rule
			// StaleTag and the cooldown both already follow.
		}

		switch {
		case err != nil:
			target.Error = err.Error()
			// Still scan current image on error
		case !presets.VersionedTag(currentTag):
			// A tag that is a name rather than a version — `trixie-curl`,
			// `stable`. No tag listing can answer "is there a newer one", so
			// this image is outside the promotion path altogether, and saying
			// nothing would file it with the images that are genuinely current
			// (#328).
			target.UnversionedTag = true
			target.PolicyReason = fmt.Sprintf(
				"%q carries no version, so no tag can be compared to it: this image is updated by rebuilds of the same tag, which the promotion path does not detect — review and repin by hand",
				currentTag)
		case update.Superseding != "":
			// No successor inside the pinned variant family — and none will
			// ever come, because the repository stopped publishing that family
			// (#252). Reporting it as up to date, which is what the family
			// comparison alone says, leaves the catalogue on a line upstream
			// abandoned. Repinning across variant families changes the base
			// image, so it is a deliberate act with a human behind it, never a
			// candidate.
			target.FrozenVariant = update.Superseding
			target.PolicyReason = fmt.Sprintf(
				"the variant line of %q has no successor: %s publishes no tag in that family any more and has moved to %q — review and repin by hand",
				currentTag, imageRegistry, update.Superseding)
		case latestTag != "" && latestTag != currentTag && !registryDatesTags(imageName):
			// The registry lists this version but dates none of its tags, so
			// the cooldown can never be shown to have elapsed for it (#245).
			// Reported as a version to review by hand rather than as a
			// candidate, which would be held in every run from now on.
			target.NewerVersion = latestTag
			target.PolicyReason = fmt.Sprintf(
				"%s publishes no date for its tags, so the cooldown cannot be applied: review and pin %s by hand",
				imageRegistry, latestTag)
		case latestTag != "" && latestTag != currentTag:
			// A candidate must be pinned before it is offered for
			// promotion: container-monitor.yml writes scan_image
			// verbatim into presets.toml, so a bare tag here would
			// undo the digest pinning on the first promotion (#242).
			// Failing to resolve the digest means no promotion — the
			// current, pinned image is scanned instead.
			digest, digestErr := resolveDigestFunc(imageName, latestTag)
			if digestErr != nil {
				target.Error = fmt.Sprintf("update %s available but not pinnable: %v", latestTag, digestErr)
				break
			}

			target.CandidateImage = fmt.Sprintf("%s:%s@%s", imageName, latestTag, digest)
			if !published.IsZero() {
				target.PublishedAt = published.UTC().Format(time.RFC3339)
			}

			// Rules 2 and 3. The vulnerabilities that let a candidate skip the
			// wait are the ones already recorded against what we run today —
			// no second scan is needed to learn them, and the monitor's scan of
			// the promoted candidate is what shows the fix landed.
			decision := presets.EvaluatePromotion(published, now, affectingUs[imageRepository(currentImage)])
			target.AgeDays = decision.AgeDays
			target.PolicyReason = decision.Reason
			target.CVEWaiver = decision.WaivedFor

			if decision.Promote {
				target.ScanImage = target.CandidateImage
				target.IsUpdate = true
			}
		}

		// The other half of rule 1: the reference the catalogue runs must
		// still exist. An image deleted upstream used to surface only when
		// someone ran the preset — twice, in #244 — and it supersedes whatever
		// the update lookup had to say, because nothing else matters until it
		// is fixed.
		if err := verifyPinnedImage(imageName, currentTag, currentDigest); err != nil {
			if errors.Is(err, errImageNotFound) {
				target.Missing = true
				target.Error = err.Error()
			} else if target.Error == "" {
				target.Error = err.Error()
			}
		}

		targets = append(targets, target)
	}

	return targets
}

// verifyPinnedImage checks that the exact reference the catalogue runs still
// resolves: the digest when there is one, since that is what a runner pulls,
// and the tag otherwise.
//
// A registry that answers 404 has deleted the image, which is the case worth
// shouting about. Any other failure — a 401 from a registry we hold no
// credentials for, a timeout — is reported as an unverified image, never as a
// deleted one.
func verifyPinnedImage(imageName, tag, digest string) error {
	reference := digest
	if reference == "" {
		reference = tag
	}
	if reference == "" {
		return nil
	}

	if _, err := resolveDigestFunc(imageName, reference); err != nil {
		if errors.Is(err, errImageNotFound) {
			return fmt.Errorf("pinned image is gone: %w", err)
		}
		return fmt.Errorf("could not verify the pinned image: %w", err)
	}
	return nil
}

// knownHighSeverityCVEs indexes known-vulnerabilities.toml by the repository it
// records exceptions against. Neither the tag nor the digest is part of the key:
// an exception is a judgement about a CVE in an image in our usage, and a
// promotion inside the same repository changes none of what it rests on (#238).
//
// These are rule 3's input: the vulnerabilities demonstrably affecting what the
// catalogue runs today, already produced by the security audit, so the cooldown
// exception costs no extra scan.
//
// An absent or unreadable file yields no waivers, which leaves the cooldown in
// force — the exception is never granted on missing evidence.
func knownHighSeverityCVEs(path string) map[string][]string {
	vulns, err := loadVulnerabilities(path)
	if err != nil {
		return nil
	}

	byImage := make(map[string][]string)
	for _, v := range vulns.Vulnerabilities {
		switch strings.ToUpper(v.Severity) {
		case "HIGH", "CRITICAL":
			byImage[v.Repository] = append(byImage[v.Repository], v.CVE)
		}
	}
	for image := range byImage {
		sort.Strings(byImage[image])
		byImage[image] = slices.Compact(byImage[image])
	}
	return byImage
}
