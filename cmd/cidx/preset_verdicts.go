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

	// ScannedBy names the scanners whose results actually backed this verdict.
	// One of them failing to run is not rare — a flaky registry login took a
	// Trivy job out on the very first end-to-end run of this gate — and a
	// verdict reached on one scanner must say so rather than let the promotion
	// PR claim both looked.
	ScannedBy []string `json:"scanned_by,omitempty"`

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
// A candidate no scanner produced a readable result for is held. Same
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

		found, scanners, err := scanFindings(resultsDir, target.ScanImage)
		if err != nil {
			verdict.Reason = fmt.Sprintf("held: %v", err)
			verdicts = append(verdicts, verdict)
			continue
		}

		decision := presets.EvaluateScan(presets.FindingIDs(found), acceptedFor(accepted, target))
		verdict.Promote = decision.Promote
		verdict.Reason = fmt.Sprintf("%s (scanned by %s)", decision.Reason, strings.Join(scanners, " and "))
		verdict.Introduces = decision.Introduces
		verdict.ScannedBy = scanners
		verdicts = append(verdicts, verdict)
	}

	return verdicts
}

// acceptedFor is what "already accepted" means for one candidate.
//
// The entries recorded against the repository the catalogue runs today are the
// status of the pinned image: security-audit.yml fails on any HIGH/CRITICAL
// finding that is not on file, so that record is what the running image is
// known to carry. A finding the candidate merely inherits from it is therefore
// not a regression.
//
// One lookup covers both sides of the promotion. A candidate is a newer tag of
// the repository the catalogue already runs — `preset scan-targets` proposes
// nothing else — so since #238 the running image and its candidate share the
// same entries by construction. Before that, an exception had to be re-filed
// against the candidate's own reference before the gate would honour it, and
// nobody ever did.
func acceptedFor(accepted map[string][]string, target scanTarget) []string {
	return accepted[imageRepository(target.ScanImage)]
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

// scanResultFiles is every name a scanner result for one image may be filed
// under. Two workflows produce these files and they do not spell the name the
// same way: container-monitor.yml runs `tr '/:@' '___'`, security-audit.yml runs
// `tr '/:' '__'` and leaves the `@` alone. Both are legitimate sources of
// findings, so both names are tried rather than one of the workflows being
// edited into agreement — a rename there is a silent miss here.
func scanResultFiles(scanner, image string) []string {
	monitor := scanResultFile(scanner, image)
	audit := fmt.Sprintf("%s-%s.json", scanner, auditFileName.Replace(image))
	if audit == monitor {
		return []string{monitor}
	}
	return []string{monitor, audit}
}

// imageFileName flattens an image reference into something a file system will
// take: `/`, `:` and `@` all become `_`, exactly as `tr '/:@' '___'` does in the
// workflow.
var imageFileName = strings.NewReplacer("/", "_", ":", "_", "@", "_")

// auditFileName is security-audit.yml's flattening: `tr '/:' '__'`, which leaves
// the digest separator in place.
var auditFileName = strings.NewReplacer("/", "_", ":", "_")

// scanFindings collects the HIGH/CRITICAL vulnerabilities the scanners reported
// for one image, and names the ones that actually produced a result.
//
// A result file that is absent is not the same as one that is unreadable, but
// the consequence is: with neither scanner heard from the candidate is held,
// because a promotion needs positive evidence, not the absence of bad news. An
// empty scanner output — which is what a failed pull leaves behind — parses as
// no JSON at all rather than as a clean image.
//
// One scanner missing is not fatal: the other one's findings are real evidence,
// and holding every promotion because a registry login flaked would rebuild the
// permanently-stuck gate this replaced. What the verdict must not do is imply a
// scanner that never ran, which is what the returned names are for.
func scanFindings(dir, image string) ([]presets.Finding, []string, error) {
	scanners := []struct {
		name  string // as the result file spells it
		title string // as a human reading the verdict spells it
		parse func([]byte) ([]presets.Finding, error)
	}{
		{"trivy", "Trivy", trivyFindings},
		{"grype", "Grype", grypeFindings},
	}

	var (
		found []presets.Finding
		read  []string
	)

	for _, scanner := range scanners {
		data, err := readFirstResult(dir, scanResultFiles(scanner.name, image))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("%s result could not be read: %w", scanner.name, err)
		}

		parsed, err := scanner.parse(data)
		if err != nil {
			return nil, nil, fmt.Errorf("%s result could not be read: %w", scanner.name, err)
		}

		read = append(read, scanner.title)
		found = append(found, parsed...)
	}

	if len(read) == 0 {
		return nil, nil, errNoScanResults
	}
	return found, read, nil
}

// readFirstResult returns the first of the candidate names that exists, and
// os.ErrNotExist when none does.
func readFirstResult(dir string, names []string) ([]byte, error) {
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return data, err
	}
	return nil, os.ErrNotExist
}

// trivyFindings reads the HIGH/CRITICAL findings out of `trivy image --format
// json`.
//
// `Type` is read off the enclosing result rather than the vulnerability: Trivy
// groups by target, and the ecosystem — `gobinary`, `debian`, `alpine` — is a
// property of the group. It is what tells a Go binary's embedded module list
// apart from an OS package database, which the stdlib exemption turns on.
//
// Trivy reports neither EPSS nor KEV, so those stay zero here; Grype carries
// them, and the triage reads whichever scanner knew.
func trivyFindings(data []byte) ([]presets.Finding, error) {
	var report struct {
		Results []struct {
			Type            string
			Vulnerabilities []struct {
				VulnerabilityID string
				PkgName         string
				Severity        string
				FixedVersion    string
			}
		}
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}

	var findings []presets.Finding
	for _, result := range report.Results {
		for _, vuln := range result.Vulnerabilities {
			if !isHighOrCritical(vuln.Severity) {
				continue
			}
			findings = append(findings, presets.Finding{
				ID:          vuln.VulnerabilityID,
				Severity:    strings.ToUpper(vuln.Severity),
				Package:     vuln.PkgName,
				PackageType: result.Type,
				FixedIn:     vuln.FixedVersion,
				Scanner:     "Trivy",
			})
		}
	}
	return findings, nil
}

// grypeFindings reads the HIGH/CRITICAL findings out of `grype -o json`.
//
// Grype reports every severity whatever `--fail-on` says, so the filter here is
// not decoration: without it a NEGLIGIBLE finding would hold a promotion the
// policy is not concerned with.
//
// `fix.versions` is a list — one advisory can be fixed in several release
// lines — and the first entry is enough for the only question asked of it: does
// a fix exist at all. `kev` is present only for a vulnerability CISA lists, so
// its absence is the answer rather than a gap; measured zero on this catalogue.
func grypeFindings(data []byte) ([]presets.Finding, error) {
	var report struct {
		Matches []struct {
			Vulnerability struct {
				ID       string `json:"id"`
				Severity string `json:"severity"`
				Fix      struct {
					Versions []string `json:"versions"`
				} `json:"fix"`
				EPSS []struct {
					EPSS float64 `json:"epss"`
				} `json:"epss"`
				KEV []struct {
					CVE string `json:"cve"`
				} `json:"kev"`
			} `json:"vulnerability"`
			Artifact struct {
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"artifact"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}

	var findings []presets.Finding
	for _, match := range report.Matches {
		vuln := match.Vulnerability
		if !isHighOrCritical(vuln.Severity) {
			continue
		}

		finding := presets.Finding{
			ID:          vuln.ID,
			Severity:    strings.ToUpper(vuln.Severity),
			Package:     match.Artifact.Name,
			PackageType: match.Artifact.Type,
			KEV:         len(vuln.KEV) > 0,
			Scanner:     "Grype",
		}
		if len(vuln.Fix.Versions) > 0 {
			finding.FixedIn = vuln.Fix.Versions[0]
		}
		if len(vuln.EPSS) > 0 {
			finding.EPSS = vuln.EPSS[0].EPSS
		}
		findings = append(findings, finding)
	}
	return findings, nil
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
