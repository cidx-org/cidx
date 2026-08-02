package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/cucumber/godog"
)

// Steps for features/security/vulnerability_status_summary.feature.
//
// They run the real thing: the staged findings go through the same triage, the
// staged acceptances through the same expiry test, and the page is the one
// `cidx security summary` writes and the audit publishes. A step asserting
// against a rendering of its own would pin the rendering, not the page (#270).

// RegisterSummarySteps registers the vulnerability status page step definitions.
func RegisterSummarySteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the base of "([^"]*)" stopped being supported$`, tc.baseStoppedBeingSupported)
	ctx.When(`^the catalogue status is summarised$`, tc.summariseCatalogueStatus)

	ctx.Then(`^the summary should say (\d+) findings? needs? a judgement$`, tc.summaryNeedingJudgement)
	ctx.Then(`^the summary should say (\d+) acceptances? (?:is|are) past (?:its|their) date$`, tc.summaryExpiredAcceptances)
	ctx.Then(`^the summary should say (\d+) bases? (?:is|are) no longer supported$`, tc.summaryUnsupportedBases)
	ctx.Then(`^the summary should say nothing is waiting for a decision$`, tc.summaryNothingWaiting)
	ctx.Then(`^the summary should report (\d+) findings? as fixed upstream$`, tc.summaryFixedUpstream)
	ctx.Then(`^the summary should report (\d+) findings? as carried$`, tc.summaryCarried)
	ctx.Then(`^the summary should name "([^"]*)"$`, tc.summaryShouldName)
	ctx.Then(`^the summary should not name "([^"]*)"$`, tc.summaryShouldNotName)
	ctx.Then(`^the summary should name "([^"]*)" as unscanned$`, tc.summaryShouldNameUnscanned)
	ctx.Then(`^the summary should point at the Security tab$`, tc.summaryPointsAtSecurityTab)
	ctx.Then(`^the machine-readable block should report (\d+) findings? needing triage$`, tc.digestNeedingTriage)
	ctx.Then(`^the machine-readable block should agree with the page$`, tc.digestAgreesWithPage)
}

// baseStoppedBeingSupported stages an image whose base is past its end of
// support, through the real classification rather than by writing the verdict
// down: the reason the page prints is the one ClassifyBase produces.
func (tc *TestContext) baseStoppedBeingSupported(image string) error {
	support := presets.ClassifyBase(
		presets.BaseOS{Family: "alpine", Version: "3.20.10"},
		[]presets.EOLCycle{{Cycle: "3.20", EOL: json.RawMessage(`"2026-04-01"`)}},
		summaryToday,
	)
	if support.State != presets.BaseEnded {
		return fmt.Errorf("the staged base classified as %q, expected %q", support.State, presets.BaseEnded)
	}

	staged, _ := tc.Config["summary_bases"].([]presets.BaseNote)
	tc.Config["summary_bases"] = append(staged, presets.BaseNote{
		Image:  image,
		State:  support.State,
		Reason: support.Reason,
	})
	return nil
}

// summaryToday is the day the scenarios are read on. Fixed, because "expired on
// 2020-01-01" has to mean the same thing in a year's time, and because the page
// carries the date it was written.
var summaryToday = time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)

// summariseCatalogueStatus renders the page the audit publishes.
//
// The staged findings and acceptances travel through the same functions the
// command calls: presets.Summarise for the partition, presets.ExpiredExceptions
// for the dates, presets.RenderSummary for the page.
func (tc *TestContext) summariseCatalogueStatus() error {
	findings := tc.exceptionFindings()

	var (
		triage    presets.Triage
		unscanned []string
	)
	for _, image := range tc.catalogueImages() {
		found, scanned := findings[image]
		if !scanned {
			unscanned = append(unscanned, image)
			continue
		}
		triage.Add(presets.Summarise(found))
	}

	accepted, _ := tc.Config["sarif_exceptions"].([]presets.Exception)
	bases, _ := tc.Config["summary_bases"].([]presets.BaseNote)

	summary := presets.CatalogueSummary{
		Images:    len(tc.catalogueImages()),
		Unscanned: unscanned,
		Triage:    triage,
		Accepted:  len(accepted),
		Expired:   presets.ExpiredExceptions(accepted, summaryToday),
		Bases:     bases,
		Day:       summaryToday,
		Links:     presets.SummaryLinks{Repo: "https://github.com/cidx-org/cidx"},
	}

	tc.Config["summary"] = summary
	tc.Config["summary_page"] = presets.RenderSummary(summary)
	return nil
}

func (tc *TestContext) summary() (presets.CatalogueSummary, string, error) {
	summary, ok := tc.Config["summary"].(presets.CatalogueSummary)
	if !ok {
		return summary, "", fmt.Errorf("the catalogue status was never summarised")
	}
	page, _ := tc.Config["summary_page"].(string)
	return summary, page, nil
}

// countShouldBe checks a number against the page as well as against the value
// behind it: a count the page never prints is a count nobody reads.
//
// More than one wording can carry the same statement, and the page uses that
// deliberately: with nothing waiting it says so in a sentence rather than
// printing a table of zeros somebody has to add up. Any of the forms satisfies
// the step; none of them being present does not.
func (tc *TestContext) countShouldBe(want int, got func(presets.CatalogueSummary) int, what string, printed ...string) error {
	summary, page, err := tc.summary()
	if err != nil {
		return err
	}
	if actual := got(summary); actual != want {
		return fmt.Errorf("%s: %d, expected %d", what, actual, want)
	}
	for _, form := range printed {
		if strings.Contains(page, form) {
			return nil
		}
	}
	return fmt.Errorf("the page never states %s in any of the forms %q:\n%s", what, printed, page)
}

func (tc *TestContext) summaryNeedingJudgement(want int) error {
	return tc.countShouldBe(want,
		func(s presets.CatalogueSummary) int { return s.Triage.Actionable },
		"findings needing a judgement",
		fmt.Sprintf("| **Needing triage** | **%d** |", want))
}

func (tc *TestContext) summaryExpiredAcceptances(want int) error {
	return tc.countShouldBe(want,
		func(s presets.CatalogueSummary) int { return len(s.Expired) },
		"acceptances past their date",
		fmt.Sprintf("| Acceptances past their expiry date | %d |", want),
		"no acceptance is past its date")
}

func (tc *TestContext) summaryUnsupportedBases(want int) error {
	return tc.countShouldBe(want,
		func(s presets.CatalogueSummary) int { return s.Digest().BasesEnded },
		"bases no longer supported",
		fmt.Sprintf("| Bases no longer supported | %d |", want),
		"every base with a known support window keeps it")
}

func (tc *TestContext) summaryFixedUpstream(want int) error {
	return tc.countShouldBe(want,
		func(s presets.CatalogueSummary) int { return s.Triage.Fixable },
		"findings fixed upstream",
		fmt.Sprintf("| Fixed upstream | %d |", want))
}

func (tc *TestContext) summaryCarried(want int) error {
	return tc.countShouldBe(want,
		func(s presets.CatalogueSummary) int { return s.Triage.Carried },
		"carried findings",
		fmt.Sprintf("**%d** HIGH/CRITICAL findings", want))
}

// summaryNothingWaiting asserts the page says so in words. "0, 0, 0" in a table
// is a state a reader has to add up; the headline is what makes it readable in
// the ten seconds the page exists for.
func (tc *TestContext) summaryNothingWaiting() error {
	summary, page, err := tc.summary()
	if err != nil {
		return err
	}
	if summary.Waiting() {
		return fmt.Errorf("the summary reports something waiting:\n%s", page)
	}
	if !strings.Contains(page, "**Nothing is waiting for a decision.**") {
		return fmt.Errorf("the page does not say that nothing is waiting:\n%s", page)
	}
	return nil
}

func (tc *TestContext) summaryShouldName(text string) error {
	_, page, err := tc.summary()
	if err != nil {
		return err
	}
	if !strings.Contains(page, text) {
		return fmt.Errorf("the page does not name %q:\n%s", text, page)
	}
	return nil
}

func (tc *TestContext) summaryShouldNotName(text string) error {
	_, page, err := tc.summary()
	if err != nil {
		return err
	}
	if strings.Contains(page, text) {
		return fmt.Errorf("the page names %q, which belongs in the Security tab rather than here:\n%s", text, page)
	}
	return nil
}

func (tc *TestContext) summaryShouldNameUnscanned(image string) error {
	summary, page, err := tc.summary()
	if err != nil {
		return err
	}
	for _, name := range summary.Unscanned {
		if name == image {
			if !strings.Contains(page, "produced no scan result") {
				return fmt.Errorf("the page does not say the image was not scanned:\n%s", page)
			}
			return tc.summaryShouldName(image)
		}
	}
	return fmt.Errorf("%s is not reported unscanned; unscanned: %v", image, summary.Unscanned)
}

func (tc *TestContext) summaryPointsAtSecurityTab() error {
	return tc.summaryShouldName("/security/code-scanning")
}

// digestBlock reads the JSON the page carries for a machine, by parsing what is
// published rather than by re-deriving it: the point of the block is that a
// consumer can find it in the body, and a test reading the struct instead would
// not notice it never being written.
func (tc *TestContext) digestBlock() (presets.SummaryDigest, error) {
	_, page, err := tc.summary()
	if err != nil {
		return presets.SummaryDigest{}, err
	}

	matched := regexp.MustCompile("(?s)```json\n(.*?)\n```").FindStringSubmatch(page)
	if matched == nil {
		return presets.SummaryDigest{}, fmt.Errorf("the page carries no machine-readable block:\n%s", page)
	}

	var digest presets.SummaryDigest
	if err := json.Unmarshal([]byte(matched[1]), &digest); err != nil {
		return digest, fmt.Errorf("the machine-readable block is not JSON: %w\n%s", err, matched[1])
	}
	return digest, nil
}

func (tc *TestContext) digestNeedingTriage(want int) error {
	digest, err := tc.digestBlock()
	if err != nil {
		return err
	}
	if digest.NeedingTriage != want {
		return fmt.Errorf("the block reports %d finding(s) needing triage, expected %d", digest.NeedingTriage, want)
	}
	return nil
}

// digestAgreesWithPage is the whole contract of carrying both forms: two
// renderings of one state, never two states.
func (tc *TestContext) digestAgreesWithPage() error {
	summary, _, err := tc.summary()
	if err != nil {
		return err
	}
	published, err := tc.digestBlock()
	if err != nil {
		return err
	}

	// Compared as JSON rather than field by field: the block is a document, and
	// what a consumer reads is the bytes, including the keys.
	want, err := json.Marshal(summary.Digest())
	if err != nil {
		return err
	}
	got, err := json.Marshal(published)
	if err != nil {
		return err
	}
	if string(want) != string(got) {
		return fmt.Errorf("the block reads\n  %s\nand the page was rendered from\n  %s", got, want)
	}
	return nil
}
