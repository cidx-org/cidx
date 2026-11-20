package commands

import (
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixtureOld = "acme/tool:1.0.0@sha256:" + zeroDigest
	fixtureNew = "acme/tool:1.0.1@sha256:" + zeroDigest
)

// fixtureCatalogue is a vulnerability file with one fix-unreleased decision on
// acme/tool, written against 1.0.0, and a second one on another image that the
// promotion does not touch.
func fixtureCatalogue() *VulnerabilityFile {
	return &VulnerabilityFile{
		Decisions: []Decision{
			{
				ID: "tool-fix-unreleased", Repository: "acme/tool", Model: "fix-unreleased",
				Advisories: []string{"CVE-2026-0001", "CVE-2026-0002"}, Treatment: "fix-unreleased",
				Reviewed: "2026-11-18", ReviewBy: "2026-12-18",
				Reason: "CVE-2026-0001 and CVE-2026-0002: acme/tool:1.0.0 carries them, fix released nowhere.",
			},
			{
				ID: "other-fix-unreleased", Repository: "acme/other", Model: "fix-unreleased",
				Advisories: []string{"CVE-2026-0003"}, Treatment: "fix-unreleased",
				Reviewed: "2026-11-18", ReviewBy: "2026-12-18",
				Reason: "CVE-2026-0003: acme/other:2.0.0 carries it.",
			},
		},
		Vulnerabilities: []Vulnerability{
			{CVE: "CVE-2026-0001", Repository: "acme/tool", Severity: "HIGH", Decision: "tool-fix-unreleased"},
			{CVE: "CVE-2026-0002", Repository: "acme/tool", Severity: "HIGH", Decision: "tool-fix-unreleased"},
			{CVE: "CVE-2026-0003", Repository: "acme/other", Severity: "HIGH", Decision: "other-fix-unreleased"},
		},
	}
}

func promotedFixture() []promotionVerdict {
	return []promotionVerdict{{CurrentImage: fixtureOld, NewImage: fixtureNew, Promote: true}}
}

// TestRejudgementListsADecisionWhoseFindingIsStillCarried: the decision is not
// rewritten to name the new image — it is named in the report, with what the
// scan of the new image found.
func TestRejudgementListsADecisionWhoseFindingIsStillCarried(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, fixtureNew, map[string]string{"CVE-2026-0001": "HIGH"})
	writeGrypeResult(t, dir, fixtureNew, map[string]string{"CVE-2026-0001": "High"})

	file := fixtureCatalogue()
	items := rejudgeFixUnreleased(file, promotedFixture(), dir)

	if len(items) != 1 {
		t.Fatalf("got %d judgements, want 1 (the decision on another image is not in question): %+v", len(items), items)
	}
	item := items[0]
	if item.Decision != "tool-fix-unreleased" || item.Retired {
		t.Fatalf("decision must be listed and kept: %+v", item)
	}
	if strings.Join(item.Carried, ",") != "CVE-2026-0001" || strings.Join(item.Gone, ",") != "CVE-2026-0002" {
		t.Errorf("carried = %v, gone = %v", item.Carried, item.Gone)
	}

	retireDecisions(file, items)
	if len(file.Decisions) != 2 || len(file.Vulnerabilities) != 3 {
		t.Errorf("a decision still carried must stay on file untouched: %d decisions, %d entries", len(file.Decisions), len(file.Vulnerabilities))
	}
	if strings.Contains(file.Decisions[0].Reason, "1.0.1") {
		t.Error("the reason must not be rewritten to name the new image")
	}

	report := rejudgementReport(items)
	for _, want := range []string{"To rejudge", "`tool-fix-unreleased`", "acme/tool:1.0.1", "Still carried: CVE-2026-0001", "No longer carried: CVE-2026-0002"} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

// TestRejudgementRetiresADecisionWhoseFindingIsGone: the finding disappeared, so
// the decision and its entries do too, and the report says so.
func TestRejudgementRetiresADecisionWhoseFindingIsGone(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, fixtureNew, map[string]string{})
	writeGrypeResult(t, dir, fixtureNew, map[string]string{})

	file := fixtureCatalogue()
	items := rejudgeFixUnreleased(file, promotedFixture(), dir)
	if len(items) != 1 || !items[0].Retired {
		t.Fatalf("the decision must be retired: %+v", items)
	}

	retireDecisions(file, items)
	if len(file.Decisions) != 1 || file.Decisions[0].ID != "other-fix-unreleased" {
		t.Errorf("decisions left: %+v", file.Decisions)
	}
	if len(file.Vulnerabilities) != 1 || file.Vulnerabilities[0].CVE != "CVE-2026-0003" {
		t.Errorf("entries left: %+v", file.Vulnerabilities)
	}
	if report := rejudgementReport(items); !strings.Contains(report, "no longer carries CVE-2026-0001, CVE-2026-0002") {
		t.Errorf("report: %s", report)
	}
}

// TestRejudgementKeepsADecisionItCannotJudge: an unreadable scan is not a finding
// that went away.
func TestRejudgementKeepsADecisionItCannotJudge(t *testing.T) {
	items := rejudgeFixUnreleased(fixtureCatalogue(), promotedFixture(), t.TempDir())
	if len(items) != 1 || items[0].Retired || len(items[0].Carried) != 2 {
		t.Fatalf("an unreadable scan must keep the decision, listed: %+v", items)
	}
}

// TestRejudgementIgnoresWhatWasNotPromoted: a held candidate changes nothing.
func TestRejudgementIgnoresWhatWasNotPromoted(t *testing.T) {
	held := []promotionVerdict{{CurrentImage: fixtureOld, NewImage: fixtureNew, Promote: false}}
	if items := rejudgeFixUnreleased(fixtureCatalogue(), held, t.TempDir()); len(items) != 0 {
		t.Errorf("got %+v", items)
	}
}

// TestRejudgementSkipsADecisionAlreadyNamingTheNewImage: it was rejudged against
// the image the catalogue now runs.
func TestRejudgementSkipsADecisionAlreadyNamingTheNewImage(t *testing.T) {
	file := fixtureCatalogue()
	file.Decisions[0].Reason += " Rejudged on acme/tool:1.0.1."
	if items := rejudgeFixUnreleased(file, promotedFixture(), t.TempDir()); len(items) != 0 {
		t.Errorf("got %+v", items)
	}
}

// TestRejudgeCommandWritesTheRetirementToTheFile runs the command the promote job
// runs, on a fixture file, and reads the file back.
func TestRejudgeCommandWritesTheRetirementToTheFile(t *testing.T) {
	dir := t.TempDir()
	writeTrivyResult(t, dir, fixtureNew, map[string]string{})
	vulnFile := filepath.Join(dir, "known-vulnerabilities.toml")
	if err := saveVulnerabilities(vulnFile, fixtureCatalogue()); err != nil {
		t.Fatal(err)
	}
	verdicts := filepath.Join(dir, "verdicts.json")
	writeJSON(t, verdicts, promotedFixture())

	var out strings.Builder
	app := NewApp()
	app.Writer = &out
	if err := app.Run([]string{"cidx", "preset", "rejudge-decisions",
		"--verdicts", verdicts, "--results", dir, "--vuln-file", vulnFile}); err != nil {
		t.Fatalf("rejudge-decisions: %v", err)
	}

	file, err := loadVulnerabilities(vulnFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(file.Decisions) != 1 || len(file.Vulnerabilities) != 1 {
		t.Errorf("file after: %d decisions, %d entries", len(file.Decisions), len(file.Vulnerabilities))
	}
	if !strings.Contains(out.String(), "`tool-fix-unreleased`") {
		t.Errorf("output: %s", out.String())
	}
}
