package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

// The scanners the catalogue scan runs, pinned by digest like every image here
// (rule 1 of the supply-chain policy). They are the very references
// security-audit.yml pins; TestScanScannersAreTheAuditsScanners keeps the two
// from drifting apart.
const (
	scanTrivyImage = "aquasec/trivy:0.73.0@sha256:7cced7cae583819fc7806d4cbc0dbbc7cad18b99f7d3e235192e6da8c091045c"
	scanGrypeImage = "anchore/grype:v0.116.1@sha256:1e71065c0a4cff3e6bd3b8add525ffac4343eb4971694eb90a31cf6d4d3e85db"

	// scanPhase names the containers in the events and the logs.
	scanPhase = "catalogue-scan"

	// scanDBRepository is where Trivy fetches its advisory database, as the audit
	// sets it.
	scanDBRepository = "ghcr.io/aquasecurity/trivy-db"
)

// securityScanCommand scans the catalogue's images and writes what the scanners
// found as SARIF, for a reader that triages by itself (Drassana).
//
// It is the local counterpart of security-audit.yml, and it differs from it on
// purpose. The audit generates an ignore file from known-vulnerabilities.toml
// and renders only what waits for a human; this runs the scanners with no ignore
// file and renders everything, so that an acceptance removes nothing and a
// finding with a fix upstream is not left out. See [presets.ScanAlerts].
//
// The scanners run as containers, through the same executor as `cidx run`, and
// the advisory databases are kept in the cache directory between runs.
func securityScanCommand() *cli.Command {
	return &cli.Command{
		Name:  "scan",
		Usage: "Scan every catalogue image with Trivy and Grype and write every finding as SARIF",
		Description: `Run Trivy and Grype on each image of the built-in catalogue and write one SARIF
result per image, package and vulnerability, with its severity and the version
that fixes it when there is one.

Nothing is filtered out: findings with a fix upstream are reported, and the
acceptances in known-vulnerabilities.toml remove nothing from the report. An
image that could not be scanned is named on the console and in the file.

Normally run as 'cidx run catalogue-scan', from the repository root.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   "security.sarif",
				Usage:   "Where to write the SARIF",
			},
			&cli.StringFlag{
				Name:    "cache",
				EnvVars: []string{"CIDX_SCAN_CACHE"},
				Value:   ".cidx/scan",
				Usage:   "Directory holding the advisory databases and the raw scan results, reused between runs",
			},
			&cli.StringFlag{
				Name:    "host-cache",
				EnvVars: []string{"CIDX_SCAN_HOST_CACHE"},
				Usage:   "The cache directory as the container daemon sees it, when this command runs inside a container (default: --cache)",
			},
			&cli.StringFlag{
				Name:    "docker-config",
				EnvVars: []string{"CIDX_SCAN_DOCKER_CONFIG"},
				Usage:   "Host directory holding the registry credentials (config.json), mounted read-only into the scanners",
			},
			&cli.StringFlag{
				Name:  "backend",
				Value: "auto",
				Usage: "Executor backend: auto, docker, podman",
			},
			&cli.IntFlag{
				Name:  "concurrency",
				Value: 3,
				Usage: "Images scanned at the same time",
			},
			&cli.StringFlag{
				Name:  "timeout",
				Value: "15m",
				Usage: "Time allowed to each scanner on each image",
			},
		},
		Action: securityScanAction,
	}
}

// scanRig is what running a scanner takes: where, with which credentials, for
// how long.
type scanRig struct {
	exec         executor.Executor
	workspace    string
	hostCache    string
	dockerConfig string
	timeout      string
}

func securityScanAction(c *cli.Context) error {
	imagePresets, err := catalogueImages()
	if err != nil {
		return err
	}

	cache := c.String("cache")
	resultsDir := filepath.Join(cache, "results")
	for _, dir := range []string{resultsDir, filepath.Join(cache, "trivy"), filepath.Join(cache, "grype")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
	}

	hostCache := c.String("host-cache")
	if hostCache == "" {
		if hostCache, err = filepath.Abs(cache); err != nil {
			return fmt.Errorf("failed to resolve %s: %w", cache, err)
		}
	}

	workspace, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to find the working directory: %w", err)
	}

	selector, err := executor.NewSelector(false, c.Bool("verbose"), true)
	if err != nil {
		return fmt.Errorf("failed to create executor selector: %w", err)
	}
	defer func() {
		if closeErr := selector.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(c.App.ErrWriter, "Warning: failed to close executor: %v\n", closeErr)
		}
	}()

	exec, err := selector.Select(scanPhase, executor.ParseBackendType(c.String("backend")))
	if err != nil {
		return fmt.Errorf("executor selection failed: %w", err)
	}

	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt, syscall.SIGTERM)
	defer stop()

	rig := scanRig{
		exec:         exec,
		workspace:    workspace,
		hostCache:    hostCache,
		dockerConfig: c.String("docker-config"),
		timeout:      c.String("timeout"),
	}

	images := make([]string, 0, len(imagePresets))
	for image := range imagePresets {
		images = append(images, image)
	}
	sort.Strings(images)

	fmt.Printf("Scanning %d catalogue image(s) with Trivy and Grype.\n", len(images))
	rig.refreshDatabases(ctx)

	concurrency := c.Int("concurrency")
	if concurrency < 1 {
		concurrency = 1
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	failures := map[string][]string{}
	slots := make(chan struct{}, concurrency)
	for i, image := range images {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		wg.Add(1)
		go func(n int, image string) {
			defer wg.Done()
			defer func() { <-slots }()

			mu.Lock()
			fmt.Printf("  [%d/%d] %s\n", n+1, len(images), image)
			mu.Unlock()

			if failed := rig.scanImage(ctx, n, image, resultsDir); len(failed) > 0 {
				mu.Lock()
				failures[image] = failed
				mu.Unlock()
			}
		}(i, image)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("scan interrupted: %w", err)
	}

	scans, gaps, partial := collectScans(imagePresets, images, resultsDir, failures)

	encoded, err := jsonIndent(presets.ScanSARIF(scans, gaps))
	if err != nil {
		return err
	}
	out := c.String("output")
	if err := os.WriteFile(out, append(encoded, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", out, err)
	}

	alerts := presets.ScanAlerts(scans)
	fixable := 0
	for _, alert := range alerts {
		if available, _ := alert.Properties["fixAvailable"].(bool); available {
			fixable++
		}
	}
	fmt.Printf("Wrote %s: %d finding(s) across %d scanned image(s), %d of them with a fix upstream.\n",
		out, len(alerts), len(scans), fixable)

	for _, line := range partial {
		fmt.Printf("Only one scanner answered: %s\n", line)
	}
	if len(gaps) > 0 {
		fmt.Printf("%d image(s) could not be scanned. They have no result in %s, which is not the same as clean:\n", len(gaps), out)
		for _, gap := range gaps {
			fmt.Printf("  %s — %s\n", gap.Image, gap.Reason)
		}
	}

	if len(scans) == 0 {
		return fmt.Errorf("no catalogue image could be scanned")
	}
	return nil
}

// collectScans reads back what the scanners wrote. An image neither scanner
// answered for is a gap, with the reason the runs gave when they gave one; an
// image only one answered for is reported as partial, since the other scanner's
// findings are missing from it.
func collectScans(
	imagePresets map[string][]string,
	images []string,
	resultsDir string,
	failures map[string][]string,
) (scans []presets.ImageScan, gaps []presets.ScanGap, partial []string) {
	lines := tomlValueLines(presets.CatalogueFile, "image")

	for _, image := range images {
		found, answered, err := scanFindings(resultsDir, image)
		if err != nil {
			reason := err.Error()
			if failed := failures[image]; len(failed) > 0 {
				reason = strings.Join(failed, "; ")
			}
			gaps = append(gaps, presets.ScanGap{Image: image, Reason: reason})
			continue
		}
		if len(answered) < 2 {
			partial = append(partial, fmt.Sprintf("%s (%s)", image, strings.Join(answered, ", ")))
		}
		scans = append(scans, presets.ImageScan{
			Image:    image,
			UsedBy:   imagePresets[image],
			Line:     lines[image],
			Findings: found,
		})
	}
	return scans, gaps, partial
}

// container describes one scanner run. Ephemeral, because every run writes its
// result into the cache and none of them may share a container with another.
func (s scanRig) container(name, image, command string, env map[string]string, mounts ...string) *config.ContainerConfig {
	if env == nil {
		env = map[string]string{}
	}
	if s.dockerConfig != "" {
		// Registry credentials, read-only and keyed by registry: the same form
		// security-audit.yml hands the scanners (#284).
		mounts = append(mounts, s.dockerConfig+":/docker-config:ro")
		env["DOCKER_CONFIG"] = "/docker-config"
	}
	return &config.ContainerConfig{
		Name:       name,
		Phase:      scanPhase,
		Image:      image,
		Command:    command,
		Volumes:    mounts,
		Env:        env,
		Ephemeral:  true,
		PullPolicy: "if-not-present",
		Timeout:    s.timeout,
		Workspace:  s.workspace,
	}
}

func (s scanRig) trivyCache() string { return s.hostCache + "/trivy:/root/.cache/trivy" }
func (s scanRig) grypeCache() string { return s.hostCache + "/grype:/root/.cache/grype" }
func (s scanRig) results() string    { return s.hostCache + "/results:/results" }

// refreshDatabases brings both advisory databases up to date once, before the
// scans, so that the scans can run side by side without racing to download them.
//
// A failed refresh is not fatal: the cache from an earlier run still answers,
// and a cold cache fails every scan by itself, with a better message.
func (s scanRig) refreshDatabases(ctx context.Context) {
	steps := []struct {
		title     string
		container *config.ContainerConfig
	}{
		{"Trivy", s.container("catalogue-scan-trivy-db", scanTrivyImage,
			"image --download-db-only --quiet",
			map[string]string{"TRIVY_DB_REPOSITORY": scanDBRepository},
			s.trivyCache())},
		{"Grype", s.container("catalogue-scan-grype-db", scanGrypeImage,
			"db update",
			map[string]string{"GRYPE_DB_CACHE_DIR": "/root/.cache/grype/db"},
			s.grypeCache())},
	}

	for _, step := range steps {
		fmt.Printf("Refreshing the %s advisory database.\n", step.title)
		if err := s.exec.Run(ctx, step.container); err != nil {
			fmt.Printf("Warning: the %s database could not be refreshed (%v); the copy in the cache is used.\n", step.title, err)
		}
	}
}

// scanImage runs both scanners on one image and returns what went wrong, one
// line per scanner that failed. The scanners are not gates: a finding is not a
// failure, only a scanner that could not do its work is.
//
// No ignore file is passed to either of them. That is the whole difference with
// security-audit.yml, and what keeps an acceptance from removing a finding.
func (s scanRig) scanImage(ctx context.Context, n int, image, resultsDir string) []string {
	var failed []string

	runs := []struct {
		scanner   string
		title     string
		container *config.ContainerConfig
	}{
		{"trivy", "Trivy", s.container(fmt.Sprintf("catalogue-scan-trivy-%02d", n+1), scanTrivyImage,
			fmt.Sprintf("image --severity HIGH,CRITICAL --skip-db-update --quiet --format json --output /results/%s %s",
				scanFileName("trivy", image), image),
			map[string]string{"TRIVY_DB_REPOSITORY": scanDBRepository},
			s.trivyCache(), s.results())},
		{"grype", "Grype", s.container(fmt.Sprintf("catalogue-scan-grype-%02d", n+1), scanGrypeImage,
			fmt.Sprintf("registry:%s -o json=/results/%s", image, scanFileName("grype", image)),
			map[string]string{
				"GRYPE_DB_AUTO_UPDATE": "false",
				"GRYPE_DB_CACHE_DIR":   "/root/.cache/grype/db",
			},
			s.grypeCache(), s.results())},
	}

	for _, run := range runs {
		// A result left by an earlier run must not answer for this one.
		removeScanResults(resultsDir, run.scanner, image)

		if err := s.exec.Run(ctx, run.container); err != nil {
			removeScanResults(resultsDir, run.scanner, image)
			failed = append(failed, fmt.Sprintf("%s: %v", run.title, err))
		}
	}
	return failed
}

func removeScanResults(dir, scanner, image string) {
	for _, name := range scanResultFiles(scanner, image) {
		_ = os.Remove(filepath.Join(dir, name))
	}
}

func jsonIndent(v any) ([]byte, error) {
	encoded, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode SARIF: %w", err)
	}
	return encoded, nil
}

// scanFileName is the name a scanner's result is filed under, the audit's
// spelling, which scanFindings reads back.
func scanFileName(scanner, image string) string {
	return fmt.Sprintf("%s-%s.json", scanner, auditFileName.Replace(image))
}
