package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// defaultTriageDir is where `cidx security baseline` leaves one triage sheet per
// scanned image, beside the scan results it is built from. It sits under
// `.cidx/`, which Git ignores: a sheet is a working document for whoever is about
// to judge acceptances, and the committed record of the outcome stays
// `known-vulnerabilities.toml` and SECURITY-BASELINE.md.
const defaultTriageDir = ".cidx/scan/triage"

// triageIndexFile is the per-image summary written next to the sheets.
const triageIndexFile = "index.tsv"

// Judging the acceptances of an image takes, CVE by CVE, what Trivy says, what
// Grype says and what is on file today. The raw JSON is several megabytes of
// minified text, which a reader without a shell can neither join nor page
// through — the 3.14.11 triage stopped on exactly that. The sheet is that join,
// done once, in a text file small enough to read: one line per (vulnerability,
// package), the two scanners merged, the current acceptance alongside, and the
// acceptances that no longer match anything listed too so they can be removed.
//
// It is a view over results already on disk. No scanner is run to produce it and
// nothing it prints changes SECURITY-BASELINE.md.

// The acceptance column's vocabulary.
const (
	acceptanceActive  = "accepted"
	acceptanceExpired = "expired"
	acceptanceStale   = "stale"
)

// triageHeader is the single header line of every sheet.
var triageHeader = []string{
	"id", "aliases", "package", "installed", "severity", "fix", "fixed_in",
	"scanners", "epss", "kev", "acceptance", "accepted_reason", "accepted_until", "description",
}

// triageIndexHeader is the single header line of index.tsv.
var triageIndexHeader = []string{"image", "carried", "accepted", "unaccepted", "fixable", "stale", "expired"}

// triageRow is one line of a sheet: a (vulnerability, package) pair carried by
// the image, or an acceptance that matches none.
type triageRow struct {
	ID          string
	Aliases     string
	Package     string
	Installed   string
	Severity    string
	Fix         string
	FixedIn     string
	Scanners    string
	EPSS        float64
	KEV         bool
	Acceptance  string // "", accepted, expired or stale
	Reason      string
	Until       string
	Description string

	// expired is whether the acceptance behind this row is past its date. It is
	// separate from Acceptance because a stale entry can be expired too, and the
	// index counts both.
	expired bool
}

// triageSheet is what one image reduces to.
type triageSheet struct {
	Rows []triageRow
}

// buildTriageSheet joins one image's findings with the acceptances on its
// repository.
//
// entries are already restricted to that repository. Matching is on the
// identifier or any alias the scanners report, in either direction: an entry
// written for a CVE still covers the GHSA a scanner prefers to name it by.
//
// Everything is sorted explicitly — severity, then identifier, then package.
// Map iteration order has bitten this repository before (#230, #233).
func buildTriageSheet(findings []presets.Finding, entries []Vulnerability, today time.Time) triageSheet {
	type key struct{ id, pkg string }
	var order []key
	groups := make(map[key][]presets.Finding)
	for _, f := range findings {
		k := key{strings.ToUpper(f.ID), f.Package}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}

	byName := make(map[string]int)
	for i, e := range entries {
		for _, name := range append([]string{e.CVE}, e.Aliases...) {
			if _, taken := byName[strings.ToUpper(name)]; !taken && name != "" {
				byName[strings.ToUpper(name)] = i
			}
		}
	}

	used := make(map[int]bool)
	var rows []triageRow
	for _, k := range order {
		group := groups[k]
		row := mergeTriageGroup(group)

		names := append([]string{k.id}, splitList(row.Aliases)...)
		for _, name := range names {
			i, ok := byName[strings.ToUpper(name)]
			if !ok {
				continue
			}
			used[i] = true
			e := entries[i]
			row.Acceptance = acceptanceActive
			if !presets.Waives(e.Expires, today) {
				row.Acceptance = acceptanceExpired
				row.expired = true
			}
			row.Reason = e.Notes
			row.Until = e.Expires
			break
		}
		rows = append(rows, row)
	}

	for i, e := range entries {
		if used[i] {
			continue
		}
		rows = append(rows, triageRow{
			ID:         strings.ToUpper(e.CVE),
			Aliases:    strings.Join(e.Aliases, ","),
			Severity:   strings.ToUpper(e.Severity),
			Acceptance: acceptanceStale,
			Reason:     e.Notes,
			Until:      e.Expires,
			expired:    !presets.Waives(e.Expires, today),
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if ra, rb := severityRank(a.Severity), severityRank(b.Severity); ra != rb {
			return ra < rb
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Package < b.Package
	})
	return triageSheet{Rows: rows}
}

func severityRank(severity string) int {
	if strings.EqualFold(severity, "CRITICAL") {
		return 0
	}
	return 1
}

// mergeTriageGroup folds the instances of one (vulnerability, package) pair —
// one per scanner that reported it — into a row.
func mergeTriageGroup(group []presets.Finding) triageRow {
	row := triageRow{ID: strings.ToUpper(group[0].ID), Package: group[0].Package, Severity: "HIGH"}

	var versions, aliases, fixed, scanners, states []string
	for _, f := range group {
		if strings.EqualFold(f.Severity, "CRITICAL") {
			row.Severity = "CRITICAL"
		}
		versions = append(versions, f.Version)
		fixed = append(fixed, f.FixedIn)
		scanners = append(scanners, strings.ToLower(f.Scanner))
		if !f.Fixed() && f.FixState != "" {
			states = append(states, f.FixState)
		}
		for _, alias := range splitList(f.Aliases) {
			if !strings.EqualFold(alias, f.ID) {
				aliases = append(aliases, alias)
			}
		}
		if f.EPSS > row.EPSS {
			row.EPSS = f.EPSS
		}
		row.KEV = row.KEV || f.KEV
		if len(oneLine(f.Description)) > len(row.Description) {
			row.Description = oneLine(f.Description)
		}
	}

	row.Installed = strings.Join(distinctFold(versions), ",")
	row.Aliases = strings.Join(distinctFold(aliases), ",")
	row.FixedIn = strings.Join(distinctFold(fixed), ",")
	row.Scanners = strings.Join(distinctFold(scanners), ",")

	switch {
	case row.FixedIn != "":
		row.Fix = "fixed"
	case len(states) > 0:
		row.Fix = strings.Join(distinctFold(states), "/")
	default:
		row.Fix = "none"
	}
	return row
}

// distinctFold sorts, drops empties and removes duplicates that differ by case
// only — the two scanners disagree on the case of a GHSA identifier.
func distinctFold(values []string) []string {
	sort.Strings(values)
	var out []string
	seen := make(map[string]bool)
	for _, v := range values {
		if v == "" || seen[strings.ToLower(v)] {
			continue
		}
		seen[strings.ToLower(v)] = true
		out = append(out, v)
	}
	return out
}

func splitList(list string) []string {
	if list == "" {
		return nil
	}
	return strings.Split(list, ",")
}

// oneLine collapses every run of whitespace — newlines and tabs included — into
// a single space. A tab or a newline inside a cell would split the row it is in.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// render writes the sheet: a header line, then one tab-separated line per row.
func (s triageSheet) render() string {
	var sb strings.Builder
	sb.WriteString(strings.Join(triageHeader, "\t"))
	sb.WriteString("\n")
	for _, r := range s.Rows {
		epss, kev := "", ""
		if r.EPSS > 0 {
			epss = strconv.FormatFloat(r.EPSS, 'f', -1, 64)
		}
		if r.KEV {
			kev = "yes"
		}
		cells := []string{
			r.ID, r.Aliases, r.Package, r.Installed, r.Severity, r.Fix, r.FixedIn,
			r.Scanners, epss, kev, r.Acceptance, r.Reason, r.Until, r.Description,
		}
		for i, c := range cells {
			cells[i] = oneLine(c)
		}
		sb.WriteString(strings.Join(cells, "\t"))
		sb.WriteString("\n")
	}
	return sb.String()
}

// counts is one image's line of the index.
//
// Carried counts sheet lines of carried findings, a (vulnerability, package) pair
// each, which is finer than the baseline's per-identifier count. Fixable is the
// ones a repin removes. Stale and Expired count acceptances: those matching no
// carried finding, and those past their date, whether or not they match.
func (s triageSheet) counts() (carried, accepted, unaccepted, fixable, stale, expired int) {
	for _, r := range s.Rows {
		if r.expired {
			expired++
		}
		if r.Acceptance == acceptanceStale {
			stale++
			continue
		}
		carried++
		if r.Acceptance == "" {
			unaccepted++
		} else {
			accepted++
		}
		if r.FixedIn != "" {
			fixable++
		}
	}
	return
}

// renderTriageIndex summarises every sheet, one line per image.
func renderTriageIndex(sheets map[string]triageSheet) string {
	images := make([]string, 0, len(sheets))
	for image := range sheets {
		images = append(images, image)
	}
	sort.Strings(images)

	var sb strings.Builder
	sb.WriteString(strings.Join(triageIndexHeader, "\t"))
	sb.WriteString("\n")
	for _, image := range images {
		carried, accepted, unaccepted, fixable, stale, expired := sheets[image].counts()
		fmt.Fprintf(&sb, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", image, carried, accepted, unaccepted, fixable, stale, expired)
	}
	return sb.String()
}

// triageSheets builds the sheet of every scanned image. An image with no result
// has no sheet: an empty one would read as clean.
func triageSheets(carried map[string][]presets.Finding, accepted []Vulnerability, today time.Time) map[string]triageSheet {
	sheets := make(map[string]triageSheet, len(carried))
	for image, findings := range carried {
		var entries []Vulnerability
		for _, v := range accepted {
			if v.Repository == imageRepository(image) && isHighOrCritical(v.Severity) {
				entries = append(entries, v)
			}
		}
		sheets[image] = buildTriageSheet(findings, entries, today)
	}
	return sheets
}

// writeTriage writes one sheet per scanned image and the index into dir.
//
// The directory is the tool's own: the `.tsv` files a previous run left are
// removed first, so an image that left the catalogue does not keep a sheet that
// reads as current. Nothing else in it is touched.
func writeTriage(dir string, carried map[string][]presets.Finding, accepted []Vulnerability, today time.Time) (int, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("failed to create %s: %w", dir, err)
	}
	old, err := filepath.Glob(filepath.Join(dir, "*.tsv"))
	if err != nil {
		return 0, fmt.Errorf("failed to list %s: %w", dir, err)
	}
	for _, name := range old {
		if err := os.Remove(name); err != nil {
			return 0, fmt.Errorf("failed to remove %s: %w", name, err)
		}
	}

	sheets := triageSheets(carried, accepted, today)
	for image, sheet := range sheets {
		path := filepath.Join(dir, triageSheetFile(image))
		if err := os.WriteFile(path, []byte(sheet.render()), 0o644); err != nil {
			return 0, fmt.Errorf("failed to write %s: %w", path, err)
		}
	}
	index := filepath.Join(dir, triageIndexFile)
	if err := os.WriteFile(index, []byte(renderTriageIndex(sheets)), 0o644); err != nil {
		return 0, fmt.Errorf("failed to write %s: %w", index, err)
	}
	return len(sheets), nil
}

// triageSheetFile names an image's sheet the way its scan results are named.
func triageSheetFile(image string) string {
	return imageFileName.Replace(image) + ".tsv"
}
