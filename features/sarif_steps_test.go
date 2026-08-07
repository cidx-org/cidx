package features

import (
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cucumber/godog"
)

// Steps for features/security/code_scanning_view.feature.
//
// They run the real thing: the staged findings go through the same triage,
// the same alert construction and the same SARIF renderer the audit's report
// job runs. A step asserting against a simulation would pin the simulation
// (#270).

// RegisterSarifSteps registers the code scanning view step definitions.
func RegisterSarifSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^"([^"]*)" reports "([^"]*)" on "([^"]*)"$`, tc.scannerReportsOnImage)
	ctx.Given(`^the exception "([^"]*)" on "([^"]*)" expired on "([^"]*)"$`, tc.exceptionExpiresOn)
	ctx.Given(`^the exception "([^"]*)" on "([^"]*)" expires on "([^"]*)"$`, tc.exceptionExpiresOn)
	ctx.Given(`^the exception "([^"]*)" on "([^"]*)" carries no expiry date$`, tc.exceptionCarriesNoExpiry)
	ctx.When(`^the catalogue findings are published to code scanning$`, tc.publishToCodeScanning)
	ctx.When(`^the scanners' ignore file is built on "([^"]*)"$`, tc.buildIgnoreFile)
	ctx.Then(`^the ignore file should carry "([^"]*)"$`, tc.ignoreFileShouldCarry(true))
	ctx.Then(`^the ignore file should not carry "([^"]*)"$`, tc.ignoreFileShouldCarry(false))
	ctx.When(`^"([^"]*)" is repinned to "([^"]*)"$`, tc.imageIsRepinned)
	ctx.Then(`^"([^"]*)" should be published as an alert$`, tc.shouldBePublished)
	ctx.Then(`^"([^"]*)" should not be published as an alert$`, tc.shouldNotBePublished)
	ctx.Then(`^"([^"]*)" should be published as (\d+) alert$`, tc.shouldBePublishedTimes)
	ctx.Then(`^the alert for "([^"]*)" should say "([^"]*)"$`, tc.alertShouldSay)
	ctx.Then(`^the alert for "([^"]*)" should point at "([^"]*)"$`, tc.alertShouldPointAt)
	ctx.Then(`^the alert for "([^"]*)" should have kept its identity$`, tc.alertShouldHaveKeptItsIdentity)
	ctx.Then(`^no alert should be published$`, tc.noAlertPublished)
	ctx.Then(`^"([^"]*)" should be reported as unscanned$`, tc.shouldBeReportedUnscanned)
}

// scannerReportsOnImage stages a finding one named scanner reported. The
// unattributed "the scanners report ..." step leaves the field empty, which is
// how a finding both of them saw is staged — its plural is the claim.
func (tc *TestContext) scannerReportsOnImage(scanner, cve, image string) error {
	return tc.stageFinding(image, presets.Finding{ID: cve, Severity: "HIGH", Scanner: scanner})
}

func (tc *TestContext) exceptionExpiresOn(cve, repository, expires string) error {
	staged, _ := tc.Config["sarif_exceptions"].([]presets.Exception)
	tc.Config["sarif_exceptions"] = append(staged, presets.Exception{
		CVE:        cve,
		Repository: repository,
		Severity:   "HIGH",
		Status:     "third-party",
		Added:      "2020-01-01",
		Expires:    expires,
		Notes:      "staged by a scenario",
	})
	return nil
}

func (tc *TestContext) exceptionCarriesNoExpiry(cve, repository string) error {
	return tc.exceptionExpiresOn(cve, repository, "")
}

// buildIgnoreFile applies the real decision the scanners' ignore file is built
// from: `cidx security vuln ignore` writes one line per entry presets.Waives
// keeps, and nothing else decides what that file contains. The generators
// themselves are a `for` range over the result, pinned by
// TestTheIgnoreFileDropsAnExpiredAcceptance in cmd/cidx.
func (tc *TestContext) buildIgnoreFile(date string) error {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return fmt.Errorf("could not read the date %q: %w", date, err)
	}

	staged, _ := tc.Config["sarif_exceptions"].([]presets.Exception)
	var carried []string
	for _, e := range staged {
		if presets.Waives(e.Expires, day) {
			carried = append(carried, strings.ToUpper(e.CVE))
		}
	}
	tc.Config["ignore_file"] = carried
	return nil
}

func (tc *TestContext) ignoreFileShouldCarry(want bool) func(string) error {
	return func(cve string) error {
		carried, built := tc.Config["ignore_file"].([]string)
		if !built {
			return fmt.Errorf("no ignore file was built")
		}

		found := false
		for _, id := range carried {
			if strings.EqualFold(id, cve) {
				found = true
			}
		}
		if found != want {
			return fmt.Errorf("the ignore file carries %s = %v, expected %v (it carries %v)",
				cve, found, want, carried)
		}
		return nil
	}
}

// imageIsRepinned promotes one catalogue image to a new reference, carrying its
// scan results across: the findings a repin does not remove are exactly the ones
// whose alerts must survive it.
func (tc *TestContext) imageIsRepinned(from, to string) error {
	images := tc.catalogueImages()
	repinned := false
	for i, image := range images {
		if image == from {
			images[i] = to
			repinned = true
		}
	}
	if !repinned {
		return fmt.Errorf("the catalogue does not run %s", from)
	}
	tc.Config["catalogue_images"] = images

	findings := tc.exceptionFindings()
	if found, scanned := findings[from]; scanned {
		findings[to] = found
		delete(findings, from)
	}
	return nil
}

// publishToCodeScanning runs the render the audit's report job runs.
//
// An image staged with no findings entry at all is one no scanner answered for,
// which is the distinction the last rule of the feature turns on: it produces no
// alerts, and it is named rather than counted as clean.
//
// The previous run is kept, because "the alert is still the same alert" is a
// claim about two consecutive publications and nothing else can express it.
func (tc *TestContext) publishToCodeScanning() error {
	if previous, published := tc.Config["sarif_log"]; published {
		tc.Config["sarif_log_previous"] = previous
	}

	findings := tc.exceptionFindings()

	var alerts []presets.Alert
	var unscanned []string
	for _, image := range tc.catalogueImages() {
		found, scanned := findings[image]
		if !scanned {
			unscanned = append(unscanned, image)
			continue
		}
		alerts = append(alerts, presets.TriageAlerts(image, []string{"staged-preset"}, found, 0)...)
	}

	// One alert per repository and CVE. Since #303 a lapsed acceptance stops
	// filtering, so its finding reaches the triage above too — and it is the
	// alert about the entry that survives the merge.
	staged, _ := tc.Config["sarif_exceptions"].([]presets.Exception)
	alerts = presets.MergeAlerts(alerts, presets.ExpiredExceptionAlerts(staged, time.Now()))

	tc.Config["sarif_log"] = presets.SARIF(alerts)
	tc.Config["sarif_unscanned"] = unscanned
	return nil
}

func (tc *TestContext) publishedLog() (presets.SarifLog, error) {
	log, ok := tc.Config["sarif_log"].(presets.SarifLog)
	if !ok {
		return presets.SarifLog{}, fmt.Errorf("nothing was published to code scanning")
	}
	return log, nil
}

// alertsFor collects the results whose rule identifier names this vulnerability.
// The identifier is `<repository>/<CVE>`, or `expired/<repository>/<CVE>`, so
// matching on the suffix is what asks "did this one get published".
func (tc *TestContext) alertsFor(cve string) ([]presets.SarifResult, presets.SarifLog, error) {
	log, err := tc.publishedLog()
	if err != nil {
		return nil, log, err
	}

	return resultsFor(log, cve), log, nil
}

func resultsFor(log presets.SarifLog, cve string) []presets.SarifResult {
	var matched []presets.SarifResult
	for _, result := range log.Runs[0].Results {
		if strings.HasSuffix(result.RuleID, "/"+strings.ToUpper(cve)) {
			matched = append(matched, result)
		}
	}
	return matched
}

func (tc *TestContext) shouldBePublished(cve string) error {
	return tc.shouldBePublishedTimes(cve, 1)
}

func (tc *TestContext) shouldBePublishedTimes(cve string, want int) error {
	matched, _, err := tc.alertsFor(cve)
	if err != nil {
		return err
	}
	if len(matched) != want {
		return fmt.Errorf("expected %d alert(s) for %s, got %d", want, cve, len(matched))
	}
	return nil
}

func (tc *TestContext) shouldNotBePublished(cve string) error {
	matched, _, err := tc.alertsFor(cve)
	if err != nil {
		return err
	}
	if len(matched) != 0 {
		return fmt.Errorf("%s was published as %d alert(s) and should not have been", cve, len(matched))
	}
	return nil
}

// alertShouldSay reads the rule's help text as well as the result message: the
// alert page is both, and which half carries a given sentence is a rendering
// detail the scenario should not have to know.
func (tc *TestContext) alertShouldSay(cve, want string) error {
	matched, log, err := tc.alertsFor(cve)
	if err != nil {
		return err
	}
	if len(matched) == 0 {
		return fmt.Errorf("%s was not published at all", cve)
	}

	text := matched[0].Message.Text
	for _, rule := range log.Runs[0].Tool.Driver.Rules {
		if rule.ID == matched[0].RuleID {
			text += "\n" + rule.ShortDescription.Text + "\n" + rule.Help.Text
		}
	}
	if !strings.Contains(text, want) {
		return fmt.Errorf("the alert for %s does not say %q; it says:\n%s", cve, want, text)
	}
	return nil
}

func (tc *TestContext) alertShouldPointAt(cve, file string) error {
	matched, _, err := tc.alertsFor(cve)
	if err != nil {
		return err
	}
	if len(matched) == 0 {
		return fmt.Errorf("%s was not published at all", cve)
	}

	got := matched[0].Locations[0].PhysicalLocation.ArtifactLocation.URI
	if got != file {
		return fmt.Errorf("the alert for %s points at %q, expected %q", cve, got, file)
	}
	return nil
}

// alertShouldHaveKeptItsIdentity compares two consecutive publications on what
// GitHub matches an alert on: the rule identifier and the fingerprint. Anything
// else about the alert may move — the message names the new reference, the
// location follows the line that was rewritten — and a reader still sees the
// finding they saw yesterday rather than a new one dated today.
func (tc *TestContext) alertShouldHaveKeptItsIdentity(cve string) error {
	previous, published := tc.Config["sarif_log_previous"].(presets.SarifLog)
	if !published {
		return fmt.Errorf("nothing was published before this run, so there is no identity to keep")
	}

	before := resultsFor(previous, cve)
	after, _, err := tc.alertsFor(cve)
	if err != nil {
		return err
	}
	if len(before) != 1 || len(after) != 1 {
		return fmt.Errorf("%s was published %d time(s) then %d time(s), expected once either side",
			cve, len(before), len(after))
	}

	if before[0].RuleID != after[0].RuleID {
		return fmt.Errorf("the alert for %s changed rule: %q then %q", cve, before[0].RuleID, after[0].RuleID)
	}
	was, is := before[0].PartialFingerprints["primaryLocationLineHash"], after[0].PartialFingerprints["primaryLocationLineHash"]
	if was != is {
		return fmt.Errorf("the alert for %s changed fingerprint: %s then %s — GitHub would close it and open a new one", cve, was, is)
	}
	return nil
}

func (tc *TestContext) noAlertPublished() error {
	log, err := tc.publishedLog()
	if err != nil {
		return err
	}
	if n := len(log.Runs[0].Results); n != 0 {
		return fmt.Errorf("expected no alert, got %d", n)
	}
	return nil
}

func (tc *TestContext) shouldBeReportedUnscanned(image string) error {
	unscanned, _ := tc.Config["sarif_unscanned"].([]string)
	for _, name := range unscanned {
		if name == image {
			return nil
		}
	}
	return fmt.Errorf("%s was not reported as unscanned; unscanned: %v", image, unscanned)
}
