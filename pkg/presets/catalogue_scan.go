package presets

import (
	"fmt"
	"sort"
	"strings"
)

// The catalogue scan: every finding the scanners report on the catalogue's
// images, rendered as SARIF for a reader that does its own triage.
//
// It is the opposite of [TriageAlerts]. The audit's SARIF is a view for people —
// it leaves out what a fix upstream will remove and what the policy has already
// answered, because a tab that lists 456 findings is a tab nobody reads. A
// consumer that computes "a fix exists" and "waiting for a judgement" itself
// needs the whole population, so nothing is filtered here:
//
//   - a finding with a fix upstream is reported, with the version that fixes it;
//   - a finding accepted in [ExceptionsFile] is reported too. The scan is run
//     without any ignore file, so an acceptance removes nothing from it; the
//     acceptances stay readable in the file that records them.
//
// One result per image, package and vulnerability. Trivy and Grype are merged
// on that key, as [TriageAlerts] merges them, and the scanners that saw it
// travel in the result's properties.

// ImageScan is what the scanners answered for one catalogue image.
type ImageScan struct {
	// Image is the pinned reference, digest included.
	Image string

	// UsedBy names the presets that run this image.
	UsedBy []string

	// Line is where [CatalogueFile] pins the image.
	Line int

	// Findings are the HIGH and CRITICAL findings of both scanners, unfiltered.
	Findings []Finding
}

// ScanGap is an image the scan could not answer for, and why. It carries no
// findings, and that is not the same as carrying none.
type ScanGap struct {
	Image  string
	Reason string
}

// ScanAlerts turns the scans into one alert per image, package and
// vulnerability, in a stable order.
func ScanAlerts(scans []ImageScan) []Alert {
	type key struct{ id, pkg string }

	var alerts []Alert
	for _, scan := range scans {
		usedBy := append([]string(nil), scan.UsedBy...)
		sort.Strings(usedBy)

		groups := map[key][]Finding{}
		var order []key
		for _, f := range scan.Findings {
			k := key{id: strings.ToUpper(f.ID), pkg: f.Package}
			if _, seen := groups[k]; !seen {
				order = append(order, k)
			}
			groups[k] = append(groups[k], f)
		}

		for _, k := range order {
			alerts = append(alerts, scanAlert(scan, usedBy, k.id, k.pkg, groups[k]))
		}
	}

	// The rule of a vulnerability is written from its first alert, so the most
	// severe comes first; the fingerprint settles the rest deterministically.
	sort.SliceStable(alerts, func(i, j int) bool {
		a, b := alerts[i], alerts[j]
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		if a.Severity != b.Severity {
			return strings.EqualFold(a.Severity, "CRITICAL")
		}
		return a.Fingerprint < b.Fingerprint
	})
	return alerts
}

func scanAlert(scan ImageScan, usedBy []string, id, pkg string, group []Finding) Alert {
	severity := "HIGH"
	for _, f := range group {
		if strings.EqualFold(f.Severity, "CRITICAL") {
			severity = "CRITICAL"
		}
	}

	scanners := distinct(group, func(f Finding) string { return f.Scanner })
	fixes := distinct(group, func(f Finding) string { return f.FixedIn })
	packageType := join(distinct(group, func(f Finding) string { return f.PackageType }), "")

	name := pkg
	if name == "" {
		name = "an unnamed package"
	}

	fix := "no fix at any version"
	if len(fixes) > 0 {
		fix = "fixed in " + strings.Join(fixes, ", ")
	}

	properties := map[string]any{
		"image":         scan.Image,
		"repository":    repositoryOf(scan.Image),
		"package":       pkg,
		"packageType":   packageType,
		"vulnerability": id,
		"severity":      severity,
		"fixAvailable":  len(fixes) > 0,
		"scanners":      scanners,
		"usedBy":        usedBy,
	}
	if len(fixes) > 0 {
		properties["fixedVersion"] = strings.Join(fixes, ", ")
	}
	if epss := topEPSS(group); epss > 0 {
		properties["epss"] = epss
	}
	if anyKEV(group) {
		properties["kev"] = true
	}

	var body strings.Builder
	fmt.Fprintf(&body, "**%s** (%s) is reported against images of the preset catalogue.\n\n", id, severity)
	body.WriteString("The catalogue scan lists every finding: those with a fix upstream and those accepted in `known-vulnerabilities.toml` are in it, and nothing is filtered. Each result names the image, the package, the scanners that saw it and the version that fixes it when there is one.\n")

	return Alert{
		RuleID:      id,
		Title:       fmt.Sprintf("%s (%s)", id, severity),
		Body:        body.String(),
		Message:     fmt.Sprintf("%s in %s on `%s` (%s): %s, reported by %s.", id, name, scan.Image, severity, fix, join(scanners, "an unnamed scanner")),
		Severity:    severity,
		File:        CatalogueFile,
		Line:        scan.Line,
		Fingerprint: fingerprint("scan", scan.Image, pkg, id),
		Properties:  properties,
	}
}

// ScanSARIF renders the catalogue scan as one SARIF run.
//
// The images that could not be scanned are written into the run's invocation,
// as an unsuccessful execution with one notification each: a file with no result
// for an image says nothing about it unless it also says it never looked.
func ScanSARIF(scans []ImageScan, gaps []ScanGap) SarifLog {
	log := SARIF(ScanAlerts(scans))
	log.Runs[0].Tool.Driver.Name = "CIDX catalogue scan"

	invocation := SarifInvocation{ExecutionSuccessful: len(gaps) == 0}
	for _, gap := range gaps {
		invocation.Notifications = append(invocation.Notifications, SarifNotification{
			Level:   "error",
			Message: SarifText{Text: fmt.Sprintf("%s was not scanned: %s. It has no result here, and is not thereby clean.", gap.Image, gap.Reason)},
		})
	}
	log.Runs[0].Invocations = []SarifInvocation{invocation}
	return log
}
