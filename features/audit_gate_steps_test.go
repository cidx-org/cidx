package features

import (
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/cucumber/godog"
)

// Steps for features/security/audit_gate.feature (#524). They run the real
// verdict: the staged findings go through the same triage as the status page,
// and Failing is the call the audit gate makes.

// RegisterAuditGateSteps registers the audit gate step definitions.
func RegisterAuditGateSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the audit gives a finding (\d+) days$`, tc.auditGivesDays)
	ctx.Given(`^"([^"]*)" on "([^"]*)" was first reported (\d+) days ago$`, tc.findingFirstReported)
	ctx.Given(`^the first-seen source answered with nothing$`, tc.firstSeenAnsweredNothing)
	ctx.Given(`^the first-seen source could not be read$`, tc.firstSeenUnreadable)
	ctx.Given(`^the scanners report "([^"]*)" on "([^"]*)" as actively exploited$`, tc.scannersReportKEV)
	ctx.When(`^the audit gate is evaluated$`, tc.auditGateIsEvaluated)
	ctx.Then(`^the audit should (pass|fail)$`, tc.auditShould)
	ctx.Then(`^the summary should say (\d+) findings? (?:is|are) past the (\d+)-day window$`, tc.summaryPastWindow)
}

const (
	gateDaysKey      = "gate_days"
	gateFirstSeenKey = "gate_first_seen"
)

func (tc *TestContext) auditGivesDays(days int) error {
	tc.Config[gateDaysKey] = days
	return nil
}

func (tc *TestContext) firstSeenMap() map[string]time.Time {
	seen, ok := tc.Config[gateFirstSeenKey].(map[string]time.Time)
	if !ok {
		seen = map[string]time.Time{}
		tc.Config[gateFirstSeenKey] = seen
	}
	return seen
}

func (tc *TestContext) findingFirstReported(cve, image string, daysAgo int) error {
	tc.firstSeenMap()[presets.AlertKey(image, cve)] = summaryToday.AddDate(0, 0, -daysAgo)
	return nil
}

func (tc *TestContext) firstSeenAnsweredNothing() error {
	tc.firstSeenMap()
	return nil
}

// firstSeenUnreadable leaves no map at all: the source could not answer.
func (tc *TestContext) firstSeenUnreadable() error {
	delete(tc.Config, gateFirstSeenKey)
	return nil
}

func (tc *TestContext) scannersReportKEV(cve, image string) error {
	return tc.stageFinding(image, presets.Finding{ID: cve, Severity: "HIGH", KEV: true})
}

func (tc *TestContext) auditGateIsEvaluated() error {
	if err := tc.summariseCatalogueStatus(); err != nil {
		return err
	}
	summary, _, err := tc.summary()
	if err != nil {
		return err
	}

	for _, image := range tc.catalogueImages() {
		found, scanned := tc.exceptionFindings()[image]
		if !scanned {
			continue
		}
		for _, group := range presets.Actionable(found) {
			kev := false
			for _, f := range group {
				kev = kev || f.KEV
			}
			summary.Findings = append(summary.Findings, presets.UnansweredFinding{Repository: image, ID: group[0].ID, KEV: kev})
		}
	}

	if days, windowed := tc.Config[gateDaysKey].(int); windowed {
		grace := &presets.Grace{Days: days}
		// A map only when the source answered: otherwise no age can be known.
		if seen, ok := tc.Config[gateFirstSeenKey].(map[string]time.Time); ok {
			grace.FirstSeen = seen
		}
		summary.Grace = grace
	}

	tc.Config["summary"] = summary
	tc.Config["summary_page"] = presets.RenderSummary(summary)
	return nil
}

func (tc *TestContext) auditShould(verdict string) error {
	summary, _, err := tc.summary()
	if err != nil {
		return err
	}
	if failing := summary.Failing(); failing != (verdict == "fail") {
		return fmt.Errorf("the audit should %s, but its verdict is failing=%v", verdict, failing)
	}
	return nil
}

func (tc *TestContext) summaryPastWindow(want, days int) error {
	summary, page, err := tc.summary()
	if err != nil {
		return err
	}
	if got := len(summary.PastGrace()); got != want {
		return fmt.Errorf("%d findings are past the window, expected %d", got, want)
	}
	row := fmt.Sprintf("| Findings past the %d-day window (the audit fails on these) | %d |", days, want)
	if !strings.Contains(page, row) {
		return fmt.Errorf("the page does not say %q:\n%s", row, page)
	}
	return nil
}
