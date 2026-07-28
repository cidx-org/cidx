package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// promotionVerdict is one line of the contract between
// `cidx preset scan-verdicts` and the promote job of container-monitor.yml: for
// one candidate the cooldown already cleared, whether the scan results let it
// replace the running image.
//
// Before #247 the workflow read `needs.trivy-scan.result`, which is `success`
// whatever the scanners found — every scan step swallowed its own exit code, so
// the gate reported its own success by construction. The verdict is now per
// candidate and differential, and it is computed here rather than in shell so
// it can be tested (the same move #242 made for the cooldown).
//
// The JSON names are consumed by jq expressions in that workflow;
// TestPromotionVerdictJSONContract pins them.
type promotionVerdict struct {
	CurrentImage string   `json:"current_image"`
	NewImage     string   `json:"new_image"`
	Presets      []string `json:"presets"`
	Promote      bool     `json:"promote"`
	Reason       string   `json:"reason"`

	// Introduces names the findings that held the candidate back, so the
	// workflow annotation says which vulnerability rather than just "held".
	Introduces []string `json:"introduces,omitempty"`

	// PolicyReason and CVEWaiver carry the cooldown's verdict (#242) through to
	// the promotion PR, which has to state both gates: the candidate served its
	// 14 days (or was waived out of them) *and* introduced nothing new.
	PolicyReason string   `json:"policy_reason,omitempty"`
	CVEWaiver    []string `json:"cve_waiver,omitempty"`
}

// presetScanVerdictsCommand turns the monitor's scan results into a promotion
// verdict per candidate.
func presetScanVerdictsCommand() *cli.Command {
	return &cli.Command{
		Name:  "scan-verdicts",
		Usage: "Decide, per candidate, whether the scan results allow its promotion",
		Description: "Reads `cidx preset scan-targets` output (stdin by default) and the\n" +
			"scanner results the monitor produced, and reports for every candidate\n" +
			"whether it introduces a HIGH/CRITICAL finding that is not already\n" +
			"accepted for the image the catalogue runs today.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "targets",
				Value: "-",
				Usage: "Path to `cidx preset scan-targets` output ('-' for stdin)",
			},
			&cli.StringFlag{
				Name:  "results",
				Value: "scan-results",
				Usage: "Directory holding the scanner result files",
			},
			&cli.StringFlag{
				Name:  "vuln-file",
				Value: defaultVulnFile,
				Usage: "Path to known-vulnerabilities.toml (the accepted findings)",
			},
		},
		Action: func(c *cli.Context) error {
			targets, err := readScanTargets(c.String("targets"))
			if err != nil {
				return err
			}

			verdicts := buildPromotionVerdicts(targets, c.String("results"),
				knownHighSeverityCVEs(c.String("vuln-file")))

			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(verdicts)
		},
	}
}

// readScanTargets reads a scan-targets document from a file, or from stdin when
// the path is "-" — the workflow already holds it in a job output, and piping it
// saves writing it back out to disk first.
func readScanTargets(path string) ([]scanTarget, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" || path == "" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("could not read the scan targets: %w", err)
	}

	var targets []scanTarget
	if err := json.Unmarshal(data, &targets); err != nil {
		return nil, fmt.Errorf("could not parse the scan targets: %w", err)
	}
	return targets, nil
}

// buildPromotionVerdicts judges the candidates the cooldown cleared, and only
// those: a target the cooldown held is not scanned as a candidate at all (its
// scan_image is still the running image), so there is nothing to judge and
// nothing to report twice. That is the order of the two gates — cooldown first,
// scan second — and it is the pipeline that enforces it.
//
// A candidate whose scan results are missing or unreadable is held. Same
// fail-closed posture as an unresolvable digest (rule 1) and an undatable
// version (rule 2): a promotion is never taken on an assumption.
func buildPromotionVerdicts(targets []scanTarget, resultsDir string, accepted map[string][]string) []promotionVerdict {
	verdicts := make([]promotionVerdict, 0, len(targets))

	for _, target := range targets {
		if !target.IsUpdate {
			continue
		}

		verdict := promotionVerdict{
			CurrentImage: target.CurrentImage,
			NewImage:     target.ScanImage,
			Presets:      target.Presets,
			PolicyReason: target.PolicyReason,
			CVEWaiver:    target.CVEWaiver,
		}

		found, err := scanFindings(resultsDir, target.ScanImage)
		if err != nil {
			verdict.Reason = fmt.Sprintf("held: %v", err)
			verdicts = append(verdicts, verdict)
			continue
		}

		decision := presets.EvaluateScan(found, acceptedFor(accepted, target))
		verdict.Promote = decision.Promote
		verdict.Reason = decision.Reason
		verdict.Introduces = decision.Introduces
		verdicts = append(verdicts, verdict)
	}

	return verdicts
}

// acceptedFor is what "already accepted" means for one candidate.
//
// The entries recorded against the image the catalogue runs today are the
// status of the pinned image: security-audit.yml fails on any HIGH/CRITICAL
// finding that is not on file, so that record is what the running image is
// known to carry. A finding the candidate merely inherits from it is therefore
// not a regression.
//
// Entries recorded against the candidate's own reference count too, for a
// finding reviewed and accepted ahead of the promotion.
func acceptedFor(accepted map[string][]string, target scanTarget) []string {
	running := accepted[refWithoutDigest(target.CurrentImage)]
	candidate := accepted[refWithoutDigest(target.ScanImage)]

	both := make([]string, 0, len(running)+len(candidate))
	both = append(both, running...)
	return append(both, candidate...)
}

// errNoScanResults is the fail-closed case: nothing was scanned, or nothing was
// kept, so there is no evidence on which to promote.
var errNoScanResults = errors.New("no scanner result was produced for this image")

// scanResultFile is the file-name convention container-monitor.yml's scan jobs
// and this command share. It is the whole coupling between them, and
// TestScanResultFileMatchesTheWorkflowConvention pins it against the `tr` call
// in the workflow.
func scanResultFile(scanner, image string) string {
	return fmt.Sprintf("%s-%s.json", scanner, imageFileName.Replace(image))
}

// imageFileName flattens an image reference into something a file system will
// take: `/`, `:` and `@` all become `_`, exactly as `tr '/:@' '___'` does in the
// workflow.
var imageFileName = strings.NewReplacer("/", "_", ":", "_", "@", "_")

// scanFindings collects the HIGH/CRITICAL vulnerabilities both scanners reported
// for one image.
//
// A result file that is absent is not the same as one that is unreadable, but
// the consequence is: either way the candidate is held, because a promotion
// needs positive evidence, not the absence of bad news. An empty scanner output
// — which is what a failed pull leaves behind — parses as no JSON at all rather
// than as a clean image.
func scanFindings(dir, image string) ([]string, error) {
	scanners := []struct {
		name  string
		parse func([]byte) ([]string, error)
	}{
		{"trivy", trivyFindings},
		{"grype", grypeFindings},
	}

	var found []string
	read := 0

	for _, scanner := range scanners {
		data, err := os.ReadFile(filepath.Join(dir, scanResultFile(scanner.name, image)))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s result could not be read: %w", scanner.name, err)
		}

		ids, err := scanner.parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s result could not be read: %w", scanner.name, err)
		}

		read++
		found = append(found, ids...)
	}

	if read == 0 {
		return nil, errNoScanResults
	}
	return found, nil
}

// trivyFindings reads the vulnerability identifiers out of `trivy image
// --format json`.
func trivyFindings(data []byte) ([]string, error) {
	var report struct {
		Results []struct {
			Vulnerabilities []struct {
				VulnerabilityID string
				Severity        string
			}
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}

	var ids []string
	for _, result := range report.Results {
		for _, vuln := range result.Vulnerabilities {
			if isHighOrCritical(vuln.Severity) {
				ids = append(ids, vuln.VulnerabilityID)
			}
		}
	}
	return ids, nil
}

// grypeFindings reads the vulnerability identifiers out of `grype -o json`.
//
// Grype reports every severity whatever `--fail-on` says, so the filter here is
// not decoration: without it a NEGLIGIBLE finding would hold a promotion the
// policy is not concerned with.
func grypeFindings(data []byte) ([]string, error) {
	var report struct {
		Matches []struct {
			Vulnerability struct {
				ID       string `json:"id"`
				Severity string `json:"severity"`
			} `json:"vulnerability"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}

	var ids []string
	for _, match := range report.Matches {
		if isHighOrCritical(match.Vulnerability.Severity) {
			ids = append(ids, match.Vulnerability.ID)
		}
	}
	return ids, nil
}

// isHighOrCritical is the severity band the policy acts on, spelled the way
// each scanner spells it (Trivy shouts, Grype capitalises).
func isHighOrCritical(severity string) bool {
	switch strings.ToUpper(severity) {
	case "HIGH", "CRITICAL":
		return true
	default:
		return false
	}
}
