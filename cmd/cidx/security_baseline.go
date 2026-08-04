package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// defaultBaselineFile is where the generated table lives. It sits at the root of
// the repository on purpose: it is committed, so its diff is the history of what
// the catalogue ships, and it is attached to the release assets alongside the
// binaries it describes.
const defaultBaselineFile = "SECURITY-BASELINE.md"

// securityBaselineCommand writes down what the built-in catalogue actually
// delivers: every image it runs, pinned by digest, what those images carry, and
// what has been accepted on them — with the justification and the expiry date.
//
// Nothing said that before. `known-vulnerabilities.toml` is the working record,
// written for the audit rather than for a reader, and reading it tells you what
// was once accepted, not what you install today.
//
// Carried and accepted are stated separately, because they are different numbers
// and publishing only the second is how the file came to read "0 accepted
// findings" while the catalogue carried 596 (#238). Accepted-but-unlisted would
// be a fiction; carried-but-unstated is the omission that made the fiction
// possible.
//
// The output is deliberately free of any generation timestamp. A date would
// change every run and make the diff — the only reason to commit the file —
// unreadable. Two generations of the same inputs produce byte-identical output;
// TestSecurityBaselineIsDeterministic pins that, and it is what makes the file
// checkable at all — TestSecurityBaselineIsCurrent fails a change to the
// catalogue that was not regenerated here (#310).
func securityBaselineCommand() *cli.Command {
	return &cli.Command{
		Name:  "baseline",
		Usage: "Generate " + defaultBaselineFile + ": the images the catalogue ships, what they carry and what is accepted",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "file",
				Value: defaultVulnFile,
				Usage: "Path to known-vulnerabilities.toml",
			},
			&cli.StringFlag{
				Name:  "results",
				Value: "scan-results",
				Usage: "Directory holding the scanner result files, read to state what the images carry",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   defaultBaselineFile,
				Usage:   "Where to write the baseline",
			},
			&cli.BoolFlag{
				Name:  "annotate",
				Usage: "Also emit GitHub Actions annotations for bases at or past end of support",
			},
		},
		Action: func(c *cli.Context) error {
			imagePresets, err := catalogueImages()
			if err != nil {
				return err
			}

			// An absent exception file is not an error: it means nothing is
			// accepted, which is a perfectly good baseline to publish.
			var accepted []Vulnerability
			if vulns, err := loadVulnerabilities(c.String("file")); err == nil {
				accepted = vulns.Vulnerabilities
			}

			carried, accounted := carriedFindings(imagePresets, c.String("results"), accepted)
			bases := catalogueBases(imagePresets, c.String("results"))

			out := c.String("output")
			if err := os.WriteFile(out, []byte(renderSecurityBaseline(imagePresets, accepted, carried, bases, accounted)), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", out, err)
			}

			triage := triageCatalogue(carried)
			fmt.Printf("Wrote %s: %d catalogue image(s), %d carried HIGH/CRITICAL finding(s) (%d needing triage), %d accepted.\n",
				out, len(imagePresets), triage.Carried, triage.Actionable, len(acceptedFindings(imagePresets, accepted)))

			// The end-of-support verdict is printed, never written: every number
			// in it is relative to today, and the file it would land in is
			// committed precisely so that a changed line means something
			// changed about the catalogue.
			support := resolveBaseSupport(bases, time.Now().UTC())
			fmt.Print(renderBaseSupport(support))
			if c.Bool("annotate") {
				for _, annotation := range baseSupportAnnotations(support) {
					fmt.Println(annotation)
				}
			}
			return nil
		},
	}
}

// baselineFinding is one accepted HIGH/CRITICAL finding, resolved against the
// image the catalogue runs today.
type baselineFinding struct {
	Image    string // the pinned catalogue reference, not the repository key
	CVE      string
	Severity string
	Status   string
	Expires  string
	Notes    string
}

// carriedFindings reads what the scanners saw on every catalogue image, keyed by
// the pinned reference: the findings their reports still show, plus the accepted
// ones their reports no longer do.
//
// That second half is #310. `security-audit.yml` generates each image's ignore
// file out of the entries accepted on that image's repository, so an accepted
// finding is deleted from its own image's results by construction (#238):
// reading only what the report shows publishes everything the catalogue carries
// *except the part it has already argued about*, under a heading claiming to say
// what it carries. Grype has always kept that half in `ignoredMatches`; Trivy
// keeps it in `ExperimentalModifiedFindings` under `--show-suppressed`, which
// the audit passes since #311 — the evidence this number needed did not exist
// before, which is why it was never counted.
//
// Which of the two halves counts is `presets.Carried`'s decision, not this
// function's: this is the I/O half, exactly as `cidx security sarif` is.
//
// An image with no result is simply absent, and the baseline says so rather than
// printing a zero: "not scanned" and "carries nothing" are the two answers this
// file must never confuse.
//
// The second return says whether the suppressed half is accounted for, which is
// the difference between publishing a total and publishing a floor. It is the
// same question `vuln prune` gates its deletions on
// ([presets.SuppressionEvidence]), asked of one number instead of one entry: the
// count is complete when every scanned repository carrying an acceptance can say
// what its ignore file took out of its results — because something was recorded
// as suppressed, or because the audit stated the file was empty (#327). A
// repository with no acceptance subtracts nothing from this count and is not
// asked.
func carriedFindings(imagePresets map[string][]string, resultsDir string, accepted []Vulnerability) (map[string][]presets.Finding, bool) {
	waived := make(map[string][]string, len(accepted))
	for _, v := range accepted {
		waived[v.Repository] = append(waived[v.Repository], v.CVE)
	}

	carried := make(map[string][]presets.Finding)
	evidence := newIgnoreEvidence()
	for image := range imagePresets {
		hidden := suppressedFindings(resultsDir, image)
		evidence.observe(resultsDir, image, hidden)

		found, _, err := scanFindings(resultsDir, image)
		if err != nil {
			continue
		}
		carried[image] = presets.Carried(found, hidden, waived[imageRepository(image)])
	}

	accounted := true
	for image := range carried {
		repository := imageRepository(image)
		if len(waived[repository]) > 0 && !evidence.Conclusive(repository) {
			accounted = false
		}
	}
	return carried, accounted
}

// triageCatalogue splits the findings image by image and sums the result.
//
// Per image, not across the catalogue: the same CVE on five images is five
// things to look at and five repins, and collapsing them would understate what
// is carried. The images are walked in a fixed order so the summary is
// reproducible (#230, #233).
func triageCatalogue(carried map[string][]presets.Finding) presets.Triage {
	images := make([]string, 0, len(carried))
	for image := range carried {
		images = append(images, image)
	}
	sort.Strings(images)

	var total presets.Triage
	for _, image := range images {
		total.Add(presets.Summarise(carried[image]))
	}
	return total
}

// acceptedFindings keeps the exceptions that describe a repository the catalogue
// actually runs, and only those.
//
// An entry recorded against a repository the catalogue has moved past waives
// nothing — that is what `vuln prune` is about — so publishing it here would
// overstate what is accepted. The baseline says what is true of the images
// shipped, or it is not worth committing.
//
// Severities outside HIGH/CRITICAL are dropped: they are the band the policy
// acts on, the band the audit gates on, and the band this table claims to cover.
func acceptedFindings(imagePresets map[string][]string, accepted []Vulnerability) []baselineFinding {
	byRepo := make(map[string][]string, len(imagePresets))
	for image := range imagePresets {
		repo := imageRepository(image)
		byRepo[repo] = append(byRepo[repo], image)
	}
	for repo := range byRepo {
		sort.Strings(byRepo[repo])
	}

	var findings []baselineFinding
	for _, v := range accepted {
		if !isHighOrCritical(v.Severity) {
			continue
		}
		// An exception covers a repository, and the catalogue may run several
		// tags of it — `rust:1.97.0` and `rust:1.97.0-slim`. The table is per
		// image, so the entry appears against each one it waives a finding on.
		for _, image := range byRepo[v.Repository] {
			findings = append(findings, baselineFinding{
				Image:    image,
				CVE:      v.CVE,
				Severity: strings.ToUpper(v.Severity),
				Status:   v.Status,
				Expires:  v.Expires,
				Notes:    v.Notes,
			})
		}
	}

	// CRITICAL before HIGH, then by image and identifier. Map iteration order
	// has bitten this repository twice (#230, #233); every ordering here is
	// explicit for that reason.
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Image != findings[j].Image {
			return findings[i].Image < findings[j].Image
		}
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity // CRITICAL < HIGH
		}
		return findings[i].CVE < findings[j].CVE
	})
	return findings
}

func renderSecurityBaseline(imagePresets map[string][]string, accepted []Vulnerability, carried map[string][]presets.Finding, bases map[string]presets.BaseOS, accounted bool) string {
	images := make([]string, 0, len(imagePresets))
	for image := range imagePresets {
		images = append(images, image)
	}
	sort.Strings(images)

	findings := acceptedFindings(imagePresets, accepted)
	countByImage := make(map[string]int, len(findings))
	for _, f := range findings {
		countByImage[f.Image]++
	}

	triage := triageCatalogue(carried)

	var sb strings.Builder
	sb.WriteString("# Security baseline\n\n")
	sb.WriteString("<!-- Generated by `cidx security baseline`. Do not edit by hand. -->\n\n")
	sb.WriteString("What the built-in preset catalogue ships: every container image it runs, pinned\n")
	sb.WriteString("by digest, what those images carry, and every HIGH/CRITICAL finding accepted on\n")
	sb.WriteString("them — with the reason it was accepted and the date that acceptance has to be\n")
	sb.WriteString("argued again.\n\n")
	sb.WriteString("Carried and accepted are different numbers, and both are stated. A file saying\n")
	sb.WriteString("only what is accepted read \"0 accepted findings\" on a catalogue carrying 596.\n")
	sb.WriteString("Accepted is a subset of carried: accepting a finding records that it was looked\n")
	sb.WriteString("at, it does not take it out of the image — so the carried count includes what\n")
	sb.WriteString("the audit's ignore file removes from its own scan results (#310).\n\n")
	sb.WriteString("This file is committed, so its diff is the history of what the catalogue\n")
	sb.WriteString("delivers, and it carries no generation date: the same inputs produce the same\n")
	sb.WriteString("bytes, and a changed line means something actually changed.\n\n")
	sb.WriteString("Only the built-in catalogue is described here. Presets your own `presets.toml`\n")
	sb.WriteString("declares are yours, and CIDX makes no claim about them (guardrail 1).\n\n")
	sb.WriteString("An entry past its expiry date waives nothing until it is reviewed; `cidx\n")
	sb.WriteString("security vuln check` is what reports those, and `cidx security vuln prune`\n")
	sb.WriteString("reports the ones no catalogue image carries any more.\n\n")

	fmt.Fprintf(&sb, "**%d images. %d accepted HIGH/CRITICAL finding(s) across %d of them.**\n\n",
		len(images), len(findings), len(countByImage))

	sb.WriteString("## What the images carry\n\n")
	writeCarriedSection(&sb, triage, len(carried), len(images))

	// #311's posture applied to a count rather than to a deletion: an absence
	// only means something once the results can say what was taken out of them.
	// Two things settle that and the second was missing until #327 — something
	// recorded as suppressed, or the audit stating that the ignore file it built
	// was empty. With neither, the accepted findings below are missing from the
	// number above and cannot be added back. Saying so is the difference between
	// publishing a total and publishing a floor — the same difference the `not
	// scanned` cell makes below.
	if len(carried) > 0 && len(findings) > 0 && !accounted {
		sb.WriteString("These results cannot say what their ignore file took out of them, so the number\n")
		sb.WriteString("above counts only what the scanners still showed: the accepted findings listed\n")
		sb.WriteString("below were removed from their own images' reports by the ignore file the audit\n")
		sb.WriteString("generates from them, and cannot be counted back. Read it as a floor. Trivy keeps\n")
		sb.WriteString("that record under `--show-suppressed` and the audit states how many entries it\n")
		sb.WriteString("wrote alongside its results — generate from its artifacts for the whole number\n")
		sb.WriteString("(#311, #327).\n\n")
	}

	sb.WriteString("## Images\n\n")
	sb.WriteString("The **Base** column is the distribution the image is built on, as the scanners\n")
	sb.WriteString("report it. It is here because it decides whether the findings above can ever\n")
	sb.WriteString("go away: once a base stops being supported, its packages receive no further\n")
	sb.WriteString("updates and every finding on that image is permanent, however fresh the tag\n")
	sb.WriteString("is. `none` is an image with no distribution underneath it — a scratch or\n")
	sb.WriteString("static build — which is an answer, not a gap.\n\n")
	sb.WriteString("The date that support ends is deliberately **not** in this file. It is\n")
	sb.WriteString("relative to the day you read it and it comes from a third party, so it would\n")
	sb.WriteString("change these lines without anything changing about the catalogue — the same\n")
	sb.WriteString("reason there is no generation date. `cidx security baseline` prints it, and\n")
	sb.WriteString("the daily security audit reports it.\n\n")
	sb.WriteString("| Image | Presets | Base | Carried HIGH/CRITICAL | Accepted |\n")
	sb.WriteString("| ----- | ------- | ---- | --------------------- | -------- |\n")
	for _, image := range images {
		names := append([]string(nil), imagePresets[image]...)
		sort.Strings(names)

		carriedCell := "not scanned"
		if found, scanned := carried[image]; scanned {
			carriedCell = fmt.Sprintf("%d", presets.Summarise(found).Carried)
		}

		base, scanned := bases[image]
		acceptedCell := "none"
		if n := countByImage[image]; n > 0 {
			acceptedCell = fmt.Sprintf("%d", n)
		}
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s |\n",
			image, strings.Join(names, ", "), baseCell(base, scanned), carriedCell, acceptedCell)
	}
	sb.WriteString("\n")

	sb.WriteString("## Accepted findings\n\n")
	if len(findings) == 0 {
		sb.WriteString("No HIGH/CRITICAL finding is accepted on any image the catalogue ships.\n")
		return sb.String()
	}

	sb.WriteString("| Image | CVE | Severity | Status | Expires | Justification |\n")
	sb.WriteString("| ----- | --- | -------- | ------ | ------- | ------------- |\n")
	for _, f := range findings {
		fmt.Fprintf(&sb, "| `%s` | %s | %s | %s | %s | %s |\n",
			f.Image, f.CVE, f.Severity, f.Status, f.Expires, escapeCell(f.Notes))
	}
	return sb.String()
}

// writeCarriedSection states what the scanners found, split the way the policy's
// questions split it. The split is the useful part: 596 findings reads as a
// catastrophe, and the number that actually needs a human is the last line.
func writeCarriedSection(sb *strings.Builder, triage presets.Triage, scanned, total int) {
	if scanned == 0 {
		sb.WriteString("No scanner result was available when this file was generated, so what the\n")
		sb.WriteString("images carry is not stated here. Point `cidx security baseline --results` at\n")
		sb.WriteString("the JSON the Security Audit workflow uploaded. An absent number is not a\n")
		sb.WriteString("zero.\n\n")
		return
	}

	if scanned < total {
		fmt.Fprintf(sb, "Scanner results were available for %d of the %d images; the rest are marked\n"+
			"`not scanned` below and count towards nothing.\n\n", scanned, total)
	}

	fmt.Fprintf(sb, "The catalogue carries **%d** HIGH/CRITICAL findings, counted per image — the\n"+
		"same CVE on five images is five repins. They split into four populations:\n\n", triage.Carried)
	sb.WriteString("| Population | Count | What it means |\n")
	sb.WriteString("| ---------- | ----- | ------------- |\n")
	fmt.Fprintf(sb, "| Go stdlib in a CLI binary | %d | Exempt by class: it goes away when the publisher recompiles, and `net/http` is unreachable in a tool that opens no listener. |\n", triage.GoStdlib)
	fmt.Fprintf(sb, "| Kernel headers | %d | Exempt by class: the kernel is the host's, not the container's. The scanner flags the headers package for its version string. |\n", triage.KernelHeaders)
	fmt.Fprintf(sb, "| Fixed upstream | %d | A fix exists. This is the images' age, not a decision — an exception must never be written for one. |\n", triage.Fixable)
	fmt.Fprintf(sb, "| **Needing triage** | **%d** | No fix at any version, and not exempt. The only population an exception is the right instrument for. |\n\n", triage.Actionable)

	if len(triage.KEV) > 0 {
		fmt.Fprintf(sb, "**In CISA KEV — being exploited now: %s.**\n\n", strings.Join(triage.KEV, ", "))
	} else {
		sb.WriteString("None of them is in CISA KEV. ")
	}
	fmt.Fprintf(sb, "The highest EPSS score seen is %.2f. Both are reported\nfor a human to read; neither gates anything.\n\n", triage.TopEPSS)
}

// escapeCell keeps a justification containing a pipe from splitting the row it
// is written in, and collapses newlines the same way.
func escapeCell(text string) string {
	if text == "" {
		return "—"
	}
	return strings.NewReplacer("|", `\|`, "\n", " ", "\r", "").Replace(text)
}
