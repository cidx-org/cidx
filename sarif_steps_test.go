package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
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
	ctx.When(`^the catalogue findings are published to code scanning$`, tc.publishToCodeScanning)
	ctx.Then(`^"([^"]*)" should be published as an alert$`, tc.shouldBePublished)
	ctx.Then(`^"([^"]*)" should not be published as an alert$`, tc.shouldNotBePublished)
	ctx.Then(`^"([^"]*)" should be published as (\d+) alert$`, tc.shouldBePublishedTimes)
	ctx.Then(`^the alert for "([^"]*)" should say "([^"]*)"$`, tc.alertShouldSay)
	ctx.Then(`^the alert for "([^"]*)" should point at "([^"]*)"$`, tc.alertShouldPointAt)
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

// publishToCodeScanning runs the render the audit's report job runs.
//
// An image staged with no findings entry at all is one no scanner answered for,
// which is the distinction the last rule of the feature turns on: it produces no
// alerts, and it is named rather than counted as clean.
func (tc *TestContext) publishToCodeScanning() error {
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

	staged, _ := tc.Config["sarif_exceptions"].([]presets.Exception)
	alerts = append(alerts, presets.ExpiredExceptionAlerts(staged, time.Now())...)

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

	var matched []presets.SarifResult
	for _, result := range log.Runs[0].Results {
		if strings.HasSuffix(result.RuleID, "/"+strings.ToUpper(cve)) {
			matched = append(matched, result)
		}
	}
	return matched, log, nil
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
