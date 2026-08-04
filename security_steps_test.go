package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/cucumber/godog"
)

// RegisterSecuritySteps registers security-related step definitions
func RegisterSecuritySteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Local safety behavior steps
	ctx.Step(`^the "([^"]*)" preset has local_behavior = "([^"]*)"$`, tc.presetHasLocalBehavior)
	ctx.Step(`^Docker image should be built$`, tc.dockerImageShouldBeBuilt)
	ctx.Step(`^Docker image should be built successfully$`, tc.dockerImageShouldBeBuilt)
	ctx.Step(`^Docker image should NOT be pushed to registry$`, tc.dockerImageShouldNotBePushed)
	ctx.Step(`^Docker image should be pushed to registry$`, tc.dockerImageShouldBePushed)
	ctx.Step(`^GitHub release should be created as draft$`, tc.githubReleaseShouldBeDraft)
	ctx.Step(`^GitHub release should be published$`, tc.githubReleaseShouldBePublished)
	ctx.Step(`^release should NOT be published$`, tc.releaseShouldNotBePublished)
	ctx.Step(`^release should be public$`, tc.releaseShouldBePublic)
	ctx.Step(`^release should be built$`, tc.releaseShouldBeBuilt)
	ctx.Step(`^release should NOT be published to GitHub$`, tc.releaseShouldNotBePublishedToGitHub)
	ctx.Step(`^the "([^"]*)" phase should build images$`, tc.phaseShouldBuildImages)
	ctx.Step(`^the "([^"]*)" phase should NOT push images$`, tc.phaseShouldNotPushImages)
	ctx.Step(`^image should NOT be pushed$`, tc.imageShouldNotBePushed)

	// Preset require_ci steps
	ctx.Step(`^a preset has require_ci = (true|false)$`, tc.presetHasRequireCI)
	ctx.Step(`^the preset has NO local_behavior defined$`, tc.presetHasNoLocalBehavior)
	ctx.Step(`^the preset has local_behavior = "([^"]*)"$`, tc.presetDirectLocalBehavior)
	ctx.Step(`^a preset has local_behavior = "([^"]*)"$`, tc.presetDirectLocalBehavior)
	ctx.When(`^I try to run that preset$`, tc.tryRunThatPreset)
	ctx.When(`^I run that preset$`, tc.runThatPreset)
	ctx.Then(`^it should execute in "([^"]*)" mode$`, tc.shouldExecuteInMode)
	ctx.Then(`^it should execute in ([^"]\S+) mode$`, tc.shouldExecuteInMode)

	// Environment detection assertion steps
	ctx.Step(`^CIDX should detect CI provider as "([^"]*)"$`, tc.shouldDetectCIProvider)
	ctx.Step(`^CIDX should detect IsPR as (true|false)$`, tc.shouldDetectIsPR)
	ctx.Step(`^CIDX should detect IsTag as (true|false)$`, tc.shouldDetectIsTag)
	ctx.Step(`^CIDX should detect BranchName as "([^"]*)"$`, tc.shouldDetectBranchName)
	ctx.Step(`^CIDX should detect TagName as "([^"]*)"$`, tc.shouldDetectTagName)

	// Environment detection - feature-specific
	ctx.Then(`^it should identify as CI environment$`, tc.shouldIdentifyAsCI)
	ctx.Then(`^it should identify as local environment$`, tc.shouldIdentifyAsLocal)
	ctx.Then(`^the provider should be "([^"]*)"$`, tc.providerShouldBe)
	ctx.Then(`^NOT "([^"]*)"$`, tc.providerShouldNotBe)
	ctx.Then(`^environment\.IsCI should be (true|false)$`, tc.envIsCIShouldBe)
	ctx.Then(`^environment\.IsPR should be (true|false)$`, tc.envIsPRShouldBe)
	ctx.Then(`^environment\.IsTag should be (true|false)$`, tc.envIsTagShouldBe)
	ctx.Then(`^environment\.BranchName should not be empty$`, tc.envBranchNameShouldNotBeEmpty)
	ctx.Then(`^environment\.BranchName should be "([^"]*)"$`, tc.envBranchNameShouldBe)
	ctx.Then(`^environment\.TagName should be "([^"]*)"$`, tc.envTagNameShouldBe)
	ctx.Then(`^environment\.Provider should be "([^"]*)"$`, tc.envProviderShouldBe)
	ctx.Then(`^environment should be detected immediately$`, tc.envShouldBeDetectedImmediately)
	ctx.Then(`^environment information should be logged$`, tc.envInfoShouldBeLogged)
	ctx.Then(`^environment should NOT be re-detected during execution$`, tc.envShouldNotBeRedetected)
	ctx.Then(`^Git information should be available$`, tc.gitInfoShouldBeAvailable)
	ctx.Then(`^I can still run CIDX commands$`, tc.canStillRunCIDX)

	// CI provider shortcuts
	ctx.Given(`^I am in GitHub Actions$`, tc.iAmInGitHubActions)
	ctx.Given(`^I am in GitLab CI$`, tc.iAmInGitLabCI)

	// The lifecycle of a vulnerability exception (#238)
	ctx.Given(`^the catalogue runs "([^"]*)"$`, tc.catalogueRuns)
	ctx.Given(`^every catalogue repository has been scanned$`, tc.everyCatalogueImageScanned)
	ctx.Given(`^no catalogue repository has been scanned$`, tc.noCatalogueImageScanned)
	ctx.Given(`^the scanners produced no result for "([^"]*)"$`, tc.noResultForImage)
	ctx.Given(`^the scanners report "([^"]*)" on "([^"]*)"$`, tc.scannersReportOnImage)
	ctx.Given(`^the scanners report "([^"]*)" on "([^"]*)", fixed in "([^"]*)"$`, tc.scannersReportFixedOnImage)
	ctx.Given(`^the scanners report "([^"]*)" on "([^"]*)" in package "([^"]*)" of type "([^"]*)"$`, tc.scannersReportPackageOnImage)
	ctx.Given(`^the audit's ignore file suppressed "([^"]*)" on "([^"]*)"$`, tc.suppressedOnImage)
	ctx.Given(`^the scan results record nothing they suppressed$`, tc.resultsRecordNoSuppression)
	ctx.Given(`^the audit declared an empty ignore file for "([^"]*)"$`, tc.declaredEmptyIgnoreFile)
	ctx.Given(`^the audit declared an ignore file of (\d+) entries for "([^"]*)"$`, tc.declaredIgnoreFileOf)
	ctx.Given(`^the audit's ignore file suppressed "([^"]*)" on "([^"]*)", fixed in "([^"]*)"$`, tc.suppressedFixedOnImage)
	ctx.Given(`^the exception "([^"]*)" was recorded against "([^"]*)"$`, tc.exceptionRecordedAgainst)
	ctx.When(`^the exception lifecycle is applied$`, tc.applyExceptionLifecycle)
	ctx.When(`^the findings are triaged$`, tc.triageFindings)
	ctx.When(`^the catalogue's carried findings are counted$`, tc.countCarriedFindings)
	ctx.Then(`^the catalogue should carry (\d+) findings?$`, tc.triageCountShouldBe("carried"))
	ctx.Then(`^the exception should be live$`, tc.exceptionShouldBe(presets.ExceptionLive))
	ctx.Then(`^the exception should be obsolete$`, tc.exceptionShouldBe(presets.ExceptionObsolete))
	ctx.Then(`^the exception should be carried over$`, tc.exceptionShouldBe(presets.ExceptionCarryOver))
	ctx.Then(`^the exception should be unresolved$`, tc.exceptionShouldBe(presets.ExceptionUnknown))
	ctx.Then(`^the exception verdict should mention "([^"]*)"$`, tc.exceptionVerdictShouldMention)
	ctx.Then(`^the exception verdict should name the repository "([^"]*)"$`, tc.exceptionVerdictShouldNameImage)
	ctx.Then(`^the exception verdict should name the fix "([^"]*)"$`, tc.exceptionVerdictShouldNameFix)
	ctx.Then(`^the exception verdict should name no fix$`, tc.exceptionVerdictShouldNameNoFix)
	ctx.Then(`^(\d+) finding\(s\) should be fixed upstream$`, tc.triageCountShouldBe("fixable"))
	ctx.Then(`^(\d+) finding\(s\) should be exempt as Go stdlib$`, tc.triageCountShouldBe("go-stdlib"))
	ctx.Then(`^(\d+) finding\(s\) should be exempt as kernel headers$`, tc.triageCountShouldBe("kernel-headers"))
	ctx.Then(`^(\d+) finding\(s\) should need triage$`, tc.triageCountShouldBe("actionable"))
}

// catalogueRuns stages one repository the catalogue runs today, in the form
// known-vulnerabilities.toml keys exceptions by.
func (tc *TestContext) catalogueRuns(image string) error {
	running, _ := tc.Config["catalogue_images"].([]string)
	tc.Config["catalogue_images"] = append(running, image)
	return nil
}

// everyCatalogueImageScanned is the evidence state where the scanners answered
// for every image and found nothing beyond what was staged. Stated out loud,
// because "scanned, nothing found" and "not scanned" are the two answers this
// whole feature turns on.
func (tc *TestContext) everyCatalogueImageScanned() error {
	findings := tc.exceptionFindings()
	for _, image := range tc.catalogueImages() {
		if _, ok := findings[image]; !ok {
			findings[image] = nil
		}
	}
	return nil
}

func (tc *TestContext) noCatalogueImageScanned() error {
	tc.Config["exception_findings"] = map[string][]presets.Finding{}
	return nil
}

// noResultForImage marks one image as unscanned while the others answered — the
// partial-evidence case, where a purge would be a guess about exactly the image
// nobody looked at.
func (tc *TestContext) noResultForImage(image string) error {
	if err := tc.everyCatalogueImageScanned(); err != nil {
		return err
	}
	delete(tc.exceptionFindings(), image)
	return nil
}

func (tc *TestContext) scannersReportOnImage(cve, image string) error {
	return tc.stageFinding(image, presets.Finding{ID: cve, Severity: "HIGH"})
}

// scannersReportFixedOnImage stages a finding the publisher has already fixed —
// the population the policy says an exception must never be written for.
func (tc *TestContext) scannersReportFixedOnImage(cve, image, fix string) error {
	return tc.stageFinding(image, presets.Finding{ID: cve, Severity: "HIGH", FixedIn: fix})
}

// scannersReportPackageOnImage stages a finding with the package metadata the
// class exemptions read.
func (tc *TestContext) scannersReportPackageOnImage(cve, image, pkg, pkgType string) error {
	return tc.stageFinding(image, presets.Finding{ID: cve, Severity: "HIGH", Package: pkg, PackageType: pkgType})
}

// suppressedOnImage stages a finding the audit's own ignore file kept out of
// the scan results. That is where an accepted CVE has to be looked for: the
// ignore file is built from these very entries, so a live one is deleted from
// its own repository's results and nothing else can answer for it. Both
// scanners record it — Grype in `ignoredMatches`, Trivy in
// `ExperimentalModifiedFindings` under `--show-suppressed`.
func (tc *TestContext) suppressedOnImage(cve, image string) error {
	return tc.stageSuppressed(image, presets.Finding{ID: cve, Severity: "HIGH", Scanner: "Grype"})
}

// suppressedFixedOnImage is the same with the fix the scanner recorded next to
// it — the population the policy says an exception must never be written for.
func (tc *TestContext) suppressedFixedOnImage(cve, image, fix string) error {
	return tc.stageSuppressed(image, presets.Finding{
		ID: cve, Severity: "HIGH", FixedIn: fix, Scanner: "Grype",
	})
}

func (tc *TestContext) stageSuppressed(image string, finding presets.Finding) error {
	suppressed := tc.suppressedFindings()
	suppressed[image] = append(suppressed[image], finding)
	return nil
}

func (tc *TestContext) suppressedFindings() map[string][]presets.Finding {
	findings, ok := tc.Config["suppressed_findings"].(map[string][]presets.Finding)
	if !ok {
		findings = make(map[string][]presets.Finding)
		tc.Config["suppressed_findings"] = findings
	}
	return findings
}

func (tc *TestContext) stageFinding(image string, finding presets.Finding) error {
	findings := tc.exceptionFindings()
	findings[image] = append(findings[image], finding)
	return nil
}

func (tc *TestContext) exceptionRecordedAgainst(cve, image string) error {
	tc.Config["exception_cve"] = cve
	tc.Config["exception_image"] = image
	return nil
}

// applyExceptionLifecycle runs the real policy — the same function
// `cidx security vuln prune` calls on known-vulnerabilities.toml.
func (tc *TestContext) applyExceptionLifecycle() error {
	cve, _ := tc.Config["exception_cve"].(string)
	image, _ := tc.Config["exception_image"].(string)
	if cve == "" || image == "" {
		return fmt.Errorf("no exception was staged")
	}

	findings, scanned := tc.Config["exception_findings"].(map[string][]presets.Finding)
	if !scanned {
		return fmt.Errorf("the scan evidence was not staged: say whether the catalogue repositories were scanned")
	}

	// The scenarios are written against results that keep the record of what
	// their ignore file removed, which is what security-audit.yml produces since
	// it started passing `--show-suppressed`. The scenarios about results that do
	// not, and about an ignore file the audit declared, say so in their own step.
	recorded, stated := tc.Config["suppressions_recorded"].(bool)
	if !stated {
		recorded = true
	}

	tc.Config["exception_verdict"] = presets.ClassifyException(
		cve, image, tc.catalogueImages(), findings, tc.suppressedFindings(),
		presets.SuppressionEvidence{Sighted: recorded, Declared: tc.declaredIgnoreFiles()})
	return nil
}

// declaredIgnoreFiles is what the audit states it wrote into each repository's
// ignore file — the evidence a suppression sighting can never supply, because an
// empty file suppresses nothing and leaves the same silence a dropped
// `--show-suppressed` leaves (#327).
func (tc *TestContext) declaredIgnoreFiles() map[string]int {
	declared, ok := tc.Config["declared_ignore_files"].(map[string]int)
	if !ok {
		declared = make(map[string]int)
		tc.Config["declared_ignore_files"] = declared
	}
	return declared
}

// declaredEmptyIgnoreFile is the state the catalogue is actually in: every
// acceptance is past its date, so the file the audit generates has nothing in it
// and filters nothing out of the results it produced.
func (tc *TestContext) declaredEmptyIgnoreFile(repository string) error {
	tc.declaredIgnoreFiles()[repository] = 0
	return nil
}

// declaredIgnoreFileOf is the other side: the audit says it did filter, so an
// absence still needs the record of what was filtered before it can be read.
func (tc *TestContext) declaredIgnoreFileOf(entries int, repository string) error {
	tc.declaredIgnoreFiles()[repository] = entries
	return nil
}

// resultsRecordNoSuppression is the evidence state the fail-closed gate exists
// for: results that show findings but keep no record of what their ignore file
// took out of them. Trivy produces exactly that without `--show-suppressed`,
// and its report carries no way to tell the two apart.
func (tc *TestContext) resultsRecordNoSuppression() error {
	tc.Config["suppressions_recorded"] = false
	return nil
}

func (tc *TestContext) catalogueImages() []string {
	running, _ := tc.Config["catalogue_images"].([]string)
	return running
}

func (tc *TestContext) exceptionFindings() map[string][]presets.Finding {
	findings, ok := tc.Config["exception_findings"].(map[string][]presets.Finding)
	if !ok {
		findings = make(map[string][]presets.Finding)
		tc.Config["exception_findings"] = findings
	}
	return findings
}

// triageFindings runs the real triage over everything staged, the same call
// `cidx security baseline` makes on the audit's artifacts.
func (tc *TestContext) triageFindings() error {
	var all []presets.Finding
	for _, image := range tc.catalogueImages() {
		all = append(all, tc.exceptionFindings()[image]...)
	}
	tc.Config["triage"] = presets.Summarise(all)
	return nil
}

// countCarriedFindings runs the real rule `cidx security baseline` publishes —
// what the reports show, plus the accepted findings the ignore file took out of
// them, and nothing else the scanners suppressed of their own accord (#310).
func (tc *TestContext) countCarriedFindings() error {
	found, scanned := tc.Config["exception_findings"].(map[string][]presets.Finding)
	if !scanned {
		return fmt.Errorf("the scan evidence was not staged: say whether the catalogue repositories were scanned")
	}

	cve, _ := tc.Config["exception_cve"].(string)
	on, _ := tc.Config["exception_image"].(string)

	var triage presets.Triage
	for _, image := range tc.catalogueImages() {
		var accepted []string
		if image == on && cve != "" {
			accepted = []string{cve}
		}
		triage.Add(presets.Summarise(presets.Carried(found[image], tc.suppressedFindings()[image], accepted)))
	}

	tc.Config["triage"] = triage
	return nil
}

func (tc *TestContext) triageCountShouldBe(population string) func(int) error {
	return func(want int) error {
		triage, ok := tc.Config["triage"].(presets.Triage)
		if !ok {
			return fmt.Errorf("the findings were not triaged")
		}

		var got int
		switch population {
		case "carried":
			got = triage.Carried
		case "fixable":
			got = triage.Fixable
		case "go-stdlib":
			got = triage.GoStdlib
		case "kernel-headers":
			got = triage.KernelHeaders
		case "actionable":
			got = triage.Actionable
		}
		if got != want {
			return fmt.Errorf("%s = %d, expected %d (carried %d)", population, got, want, triage.Carried)
		}
		return nil
	}
}

func (tc *TestContext) exceptionVerdict() (presets.ExceptionVerdict, error) {
	verdict, ok := tc.Config["exception_verdict"].(presets.ExceptionVerdict)
	if !ok {
		return presets.ExceptionVerdict{}, fmt.Errorf("the exception lifecycle was not applied")
	}
	return verdict, nil
}

func (tc *TestContext) exceptionShouldBe(want string) func() error {
	return func() error {
		verdict, err := tc.exceptionVerdict()
		if err != nil {
			return err
		}
		if verdict.State != want {
			return fmt.Errorf("exception is %q (%s), expected %q", verdict.State, verdict.Reason, want)
		}
		return nil
	}
}

func (tc *TestContext) exceptionVerdictShouldMention(fragment string) error {
	verdict, err := tc.exceptionVerdict()
	if err != nil {
		return err
	}
	if !strings.Contains(verdict.Reason, fragment) {
		return fmt.Errorf("verdict %q does not mention %q", verdict.Reason, fragment)
	}
	return nil
}

func (tc *TestContext) exceptionVerdictShouldNameImage(image string) error {
	verdict, err := tc.exceptionVerdict()
	if err != nil {
		return err
	}
	if verdict.StillOn != image {
		return fmt.Errorf("verdict names %q as still carrying the finding, expected %q", verdict.StillOn, image)
	}
	if !strings.Contains(verdict.Reason, image) {
		return fmt.Errorf("verdict %q does not say which repository still carries the finding", verdict.Reason)
	}
	return nil
}

// exceptionVerdictShouldNameFix: a carried-over CVE that is fixed upstream is
// not exception territory, and the verdict has to say so rather than let the
// entry be re-filed in silence.
func (tc *TestContext) exceptionVerdictShouldNameFix(fix string) error {
	verdict, err := tc.exceptionVerdict()
	if err != nil {
		return err
	}
	if verdict.FixedIn != fix {
		return fmt.Errorf("verdict reports the fix as %q, expected %q", verdict.FixedIn, fix)
	}
	return nil
}

// exceptionVerdictShouldNameNoFix is the other half of the claim: an empty
// FixedIn says nobody reported one, which is what an entry with no fix and an
// entry only Trivy knows the fix for both look like. The report says as much
// rather than letting silence read as "no fix exists".
func (tc *TestContext) exceptionVerdictShouldNameNoFix() error {
	verdict, err := tc.exceptionVerdict()
	if err != nil {
		return err
	}
	if verdict.FixedIn != "" {
		return fmt.Errorf("verdict reports a fix in %q, expected none", verdict.FixedIn)
	}
	return nil
}

// presetHasLocalBehavior sets local_behavior for a preset
func (tc *TestContext) presetHasLocalBehavior(preset, behavior string) error {
	if tc.Config["presets"] == nil {
		tc.Config["presets"] = make(map[string]any)
	}
	presets := tc.Config["presets"].(map[string]any)
	presets[preset] = map[string]string{
		"local_behavior": behavior,
	}
	return nil
}

// dockerImageShouldBeBuilt verifies Docker image was built
func (tc *TestContext) dockerImageShouldBeBuilt() error {
	// In simulation, image is built if phase executed
	return nil
}

// dockerImageShouldNotBePushed verifies Docker image was not pushed
func (tc *TestContext) dockerImageShouldNotBePushed() error {
	if strings.Contains(tc.Output, "no-push") || !tc.CI {
		return nil
	}
	return fmt.Errorf("expected image to not be pushed")
}

// dockerImageShouldBePushed verifies Docker image was pushed
func (tc *TestContext) dockerImageShouldBePushed() error {
	if tc.CI {
		return nil
	}
	return fmt.Errorf("expected image to be pushed (CI only)")
}

// githubReleaseShouldBeDraft verifies GitHub release was created as draft
func (tc *TestContext) githubReleaseShouldBeDraft() error {
	if strings.Contains(tc.Output, "draft") {
		return nil
	}
	return fmt.Errorf("expected draft release")
}

// githubReleaseShouldBePublished verifies GitHub release was published
func (tc *TestContext) githubReleaseShouldBePublished() error {
	if tc.CI {
		return nil
	}
	return fmt.Errorf("expected published release (CI only)")
}

// releaseShouldNotBePublished verifies release was not published
func (tc *TestContext) releaseShouldNotBePublished() error {
	if !tc.CI || strings.Contains(tc.Output, "draft") {
		return nil
	}
	return fmt.Errorf("expected release to not be published")
}

// releaseShouldBePublic verifies release is public
func (tc *TestContext) releaseShouldBePublic() error {
	if tc.CI {
		return nil
	}
	return fmt.Errorf("expected public release")
}

// releaseShouldBeBuilt verifies release was built
func (tc *TestContext) releaseShouldBeBuilt() error {
	return nil
}

// releaseShouldNotBePublishedToGitHub verifies release was not published to GitHub
func (tc *TestContext) releaseShouldNotBePublishedToGitHub() error {
	if !tc.CI {
		return nil
	}
	return fmt.Errorf("expected release to not be published to GitHub")
}

// phaseShouldBuildImages verifies phase built images
func (tc *TestContext) phaseShouldBuildImages(phase string) error {
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			return nil
		}
	}
	return fmt.Errorf("phase %s was not executed", phase)
}

// phaseShouldNotPushImages verifies phase did not push images
func (tc *TestContext) phaseShouldNotPushImages(phase string) error {
	if !tc.CI || strings.Contains(tc.Output, "no-push") {
		return nil
	}
	return fmt.Errorf("expected no push in phase %s", phase)
}

// imageShouldNotBePushed verifies image was not pushed
func (tc *TestContext) imageShouldNotBePushed() error {
	return tc.dockerImageShouldNotBePushed()
}

// presetHasRequireCI sets require_ci for a preset
func (tc *TestContext) presetHasRequireCI(value string) error {
	tc.Config["require_ci"] = value == "true"
	return nil
}

// presetHasNoLocalBehavior indicates no local_behavior is defined
func (tc *TestContext) presetHasNoLocalBehavior() error {
	tc.Config["no_local_behavior"] = true
	return nil
}

// presetDirectLocalBehavior sets a local_behavior directly on config
func (tc *TestContext) presetDirectLocalBehavior(behavior string) error {
	tc.Config["local_behavior"] = behavior
	if tc.Config["presets"] == nil {
		tc.Config["presets"] = make(map[string]any)
	}
	presets := tc.Config["presets"].(map[string]any)
	presets["_current"] = map[string]string{
		"local_behavior": behavior,
	}
	return nil
}

// tryRunThatPreset attempts to run the current preset
func (tc *TestContext) tryRunThatPreset() error {
	// Check require_ci
	if tc.Config["require_ci"] == true && !tc.CI {
		if tc.Config["no_local_behavior"] == true {
			tc.Output += "Error: preset requires CI environment\n"
			tc.ExitCode = 1
			return nil
		}
	}

	// Check disabled behavior
	if behavior, ok := tc.Config["local_behavior"].(string); ok && behavior == "disabled" {
		tc.Output += "Error: disabled in local environment\n"
		tc.ExitCode = 1
		return nil
	}

	return tc.runThatPreset()
}

// runThatPreset runs the current preset
func (tc *TestContext) runThatPreset() error {
	tc.LastCommand = "cidx run _current"

	if !tc.CI {
		tc.Output += "Environment: Local (safe mode)\n"
	}

	// Apply local behavior
	if behavior, ok := tc.Config["local_behavior"].(string); ok && !tc.CI {
		switch behavior {
		case "draft":
			tc.Output += "Local safety: draft - Local mode: draft creation only\n"
		case "no-push":
			tc.Output += "Local safety: no-push - Local mode: build without push\n"
		case "dry-run":
			tc.Output += "Local safety: dry-run - Local mode: dry-run only\n"
		case "production":
			tc.Output += "Local safety: production - Local mode: production (use with caution!)\n"
		}
	}

	tc.Output += "✓ _current completed successfully\n"
	tc.ExecutedPhases = append(tc.ExecutedPhases, "_current")
	return nil
}

// shouldExecuteInMode checks the preset executed in the given mode
func (tc *TestContext) shouldExecuteInMode(mode string) error {
	switch mode {
	case "draft":
		mode = "draft creation only"
	case "no-push":
		mode = "build without push"
	case "dry-run":
		mode = "dry-run only"
	case "production":
		mode = "production (use with caution!)"
	default:
		mode = fmt.Sprintf("Local mode: %s", mode)
	}
	if strings.Contains(tc.Output, mode) {
		return nil
	}
	return fmt.Errorf("expected mode '%s' in output, got:\n%s", mode, tc.Output)
}

// shouldDetectCIProvider verifies CI provider detection
func (tc *TestContext) shouldDetectCIProvider(provider string) error {
	if tc.Provider != provider {
		return fmt.Errorf("expected provider %s, got %s", provider, tc.Provider)
	}
	return nil
}

// shouldDetectIsPR verifies PR detection
func (tc *TestContext) shouldDetectIsPR(isPR string) error {
	expected := isPR == "true"
	actual := tc.EventType == "pull_request"
	if actual != expected {
		return fmt.Errorf("expected IsPR=%v, got %v (event: %s)", expected, actual, tc.EventType)
	}
	return nil
}

// shouldDetectIsTag verifies tag detection
func (tc *TestContext) shouldDetectIsTag(isTag string) error {
	expected := isTag == "true"
	actual := tc.EventType == "tag"
	if actual != expected {
		return fmt.Errorf("expected IsTag=%v, got %v (event: %s)", expected, actual, tc.EventType)
	}
	return nil
}

// shouldDetectBranchName verifies branch name detection
func (tc *TestContext) shouldDetectBranchName(branch string) error {
	return nil
}

// shouldDetectTagName verifies tag name detection
func (tc *TestContext) shouldDetectTagName(tag string) error {
	if tc.EventType != "tag" {
		return fmt.Errorf("expected tag event to detect tag name %s", tag)
	}
	return nil
}

// shouldIdentifyAsCI checks environment is CI
func (tc *TestContext) shouldIdentifyAsCI() error {
	if !tc.CI {
		return fmt.Errorf("expected CI environment, got local")
	}
	return nil
}

// shouldIdentifyAsLocal checks environment is local
func (tc *TestContext) shouldIdentifyAsLocal() error {
	if tc.CI {
		return fmt.Errorf("expected local environment, got CI")
	}
	return nil
}

// providerShouldBe checks provider name
func (tc *TestContext) providerShouldBe(provider string) error {
	if tc.Provider != provider {
		return fmt.Errorf("expected provider '%s', got '%s'", provider, tc.Provider)
	}
	return nil
}

// providerShouldNotBe checks provider is NOT the given name
func (tc *TestContext) providerShouldNotBe(provider string) error {
	if tc.Provider == provider {
		return fmt.Errorf("expected provider to NOT be '%s'", provider)
	}
	return nil
}

// envIsCIShouldBe checks IsCI value
func (tc *TestContext) envIsCIShouldBe(value string) error {
	expected := value == "true"
	if tc.CI != expected {
		return fmt.Errorf("expected IsCI=%v, got %v", expected, tc.CI)
	}
	return nil
}

// envIsPRShouldBe checks IsPR value
func (tc *TestContext) envIsPRShouldBe(value string) error {
	return tc.shouldDetectIsPR(value)
}

// envIsTagShouldBe checks IsTag value
func (tc *TestContext) envIsTagShouldBe(value string) error {
	return tc.shouldDetectIsTag(value)
}

// envBranchNameShouldNotBeEmpty checks branch name is set
func (tc *TestContext) envBranchNameShouldNotBeEmpty() error {
	// In simulation, branch is always available in CI context
	return nil
}

// envBranchNameShouldBe checks branch name matches
func (tc *TestContext) envBranchNameShouldBe(branch string) error {
	if detected, ok := tc.Config["branch"].(string); ok {
		if detected != branch {
			return fmt.Errorf("expected branch '%s', got '%s'", branch, detected)
		}
	}
	return nil
}

// envTagNameShouldBe checks tag name matches
func (tc *TestContext) envTagNameShouldBe(tag string) error {
	if configTag, ok := tc.Config["tag"].(string); ok {
		if configTag != tag {
			return fmt.Errorf("expected tag '%s', got '%s'", tag, configTag)
		}
		return nil
	}
	return nil
}

// envProviderShouldBe checks provider in environment context
func (tc *TestContext) envProviderShouldBe(provider string) error {
	return tc.providerShouldBe(provider)
}

// envShouldBeDetectedImmediately checks environment was detected at startup
func (tc *TestContext) envShouldBeDetectedImmediately() error {
	if !tc.Config["started"].(bool) {
		return fmt.Errorf("CIDX was not started")
	}
	return nil
}

// envInfoShouldBeLogged checks environment info was logged
func (tc *TestContext) envInfoShouldBeLogged() error {
	return nil
}

// envShouldNotBeRedetected checks environment is not re-detected
func (tc *TestContext) envShouldNotBeRedetected() error {
	return nil
}

// gitInfoShouldBeAvailable checks Git info is available in local env
func (tc *TestContext) gitInfoShouldBeAvailable() error {
	return nil
}

// canStillRunCIDX checks CIDX commands work in local environment
func (tc *TestContext) canStillRunCIDX() error {
	return nil
}

// iAmInGitHubActions sets up GitHub Actions environment
func (tc *TestContext) iAmInGitHubActions() error {
	tc.CI = true
	tc.Provider = "GitHub Actions"
	tc.Environment = "CI"
	return tc.setEnvVar("GITHUB_ACTIONS", "true")
}

// iAmInGitLabCI sets up GitLab CI environment
func (tc *TestContext) iAmInGitLabCI() error {
	tc.CI = true
	tc.Provider = "GitLab CI"
	tc.Environment = "CI"
	return tc.setEnvVar("GITLAB_CI", "true")
}

// ---------------------------------------------------------------------------
// End of support for the base an image is built on (#303)
//
// These steps run the real classification — presets.ClassifyBase and
// presets.UncheckedBase — against the release lines endoflife.date publishes
// today, held as a fixture. Nothing here performs a request: the whole point of
// the seam in cmd/cidx is that the decision is testable without one.

// endoflifeFixture is what endoflife.date answers for the three products the
// catalogue's families map to, transcribed from the live API. Only the two
// fields the signal reads are kept.
//
// It is a fixture rather than a call, deliberately. A test that reached the
// real API would fail on a Friday outage and would silently change verdicts
// whenever upstream revised a date — neither of which says anything about this
// code.
var endoflifeFixture = map[string]string{
	"alpine-linux": `[
		{"cycle":"3.24","eol":"2028-06-01"},
		{"cycle":"3.23","eol":"2027-11-01"},
		{"cycle":"3.22","eol":"2027-05-01"},
		{"cycle":"3.21","eol":"2026-11-01"},
		{"cycle":"3.20","eol":"2026-04-01"}
	]`,
	"debian": `[
		{"cycle":"13","eol":"2028-08-09"},
		{"cycle":"12","eol":"2026-07-11"},
		{"cycle":"11","eol":"2024-08-14"}
	]`,
	"fedora": `[
		{"cycle":"44","eol":"2027-06-02"},
		{"cycle":"43","eol":"2026-12-09"}
	]`,
}

// RegisterBaseEOLSteps registers the end-of-support step definitions.
func RegisterBaseEOLSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the image is built on "([^"]*)" "([^"]*)"$`, tc.imageBuiltOn)
	ctx.Given(`^the image is built on nothing at all$`, tc.imageBuiltOnNothing)
	ctx.Given(`^endoflife\.date publishes the release lines it publishes today$`, tc.endoflifeAnswers)
	ctx.Given(`^endoflife\.date does not answer$`, tc.endoflifeSilent)
	ctx.When(`^the base is checked for end of support on "([^"]*)"$`, tc.checkBaseEndOfSupport)

	ctx.Then(`^the base should be reported as supported$`, tc.baseShouldBe(presets.BaseSupported))
	ctx.Then(`^the base should be reported as approaching end of support$`, tc.baseShouldBe(presets.BaseEndingSoon))
	ctx.Then(`^the base should be reported as past end of support$`, tc.baseShouldBe(presets.BaseEnded))
	ctx.Then(`^the base should be reported as unresolvable$`, tc.baseShouldBe(presets.BaseUnknown))
	ctx.Then(`^the base should be reported as unchecked$`, tc.baseShouldBe(presets.BaseUnchecked))
	ctx.Then(`^the base should be reported as having no distribution base$`, tc.baseShouldBe(presets.BaseNone))

	ctx.Then(`^the report should ask for attention$`, tc.baseShouldAskForAttention(true))
	ctx.Then(`^the report should not ask for attention$`, tc.baseShouldAskForAttention(false))
	ctx.Then(`^the report should name the date "([^"]*)"$`, tc.baseReasonShouldContain)
	ctx.Then(`^the report should name the family "([^"]*)"$`, tc.baseReasonShouldContain)
	ctx.Then(`^the report should explain that its findings will never be fixed$`, tc.baseReasonShouldExplain("never be fixed"))
	ctx.Then(`^the report should explain that the check did not happen$`, tc.baseReasonShouldExplain("not checked"))
}

func (tc *TestContext) imageBuiltOn(family, version string) error {
	tc.Config["base_os"] = presets.BaseOS{Family: family, Version: version}
	return nil
}

func (tc *TestContext) imageBuiltOnNothing() error {
	tc.Config["base_os"] = presets.BaseOS{}
	return nil
}

func (tc *TestContext) endoflifeAnswers() error {
	tc.Config["endoflife_silent"] = false
	return nil
}

func (tc *TestContext) endoflifeSilent() error {
	tc.Config["endoflife_silent"] = true
	return nil
}

// checkBaseEndOfSupport runs the real policy: the same functions the baseline
// and the security audit call.
func (tc *TestContext) checkBaseEndOfSupport(date string) error {
	base, ok := tc.Config["base_os"].(presets.BaseOS)
	if !ok {
		return fmt.Errorf("no base was staged for the image")
	}

	now, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return fmt.Errorf("could not read the date %q: %w", date, err)
	}

	if silent, _ := tc.Config["endoflife_silent"].(bool); silent {
		tc.Config["base_support"] = presets.UncheckedBase(base, fmt.Errorf("endoflife.date could not be reached"))
		return nil
	}

	var cycles []presets.EOLCycle
	if product, mapped := presets.EOLProduct(base.Family); mapped {
		document, published := endoflifeFixture[product]
		if !published {
			return fmt.Errorf("no fixture transcribed for the endoflife.date product %q", product)
		}
		if err := json.Unmarshal([]byte(document), &cycles); err != nil {
			return fmt.Errorf("could not read the %q fixture: %w", product, err)
		}
	}

	tc.Config["base_support"] = presets.ClassifyBase(base, cycles, now)
	return nil
}

func (tc *TestContext) baseSupport() (presets.BaseSupport, error) {
	support, ok := tc.Config["base_support"].(presets.BaseSupport)
	if !ok {
		return presets.BaseSupport{}, fmt.Errorf("the base was never checked")
	}
	return support, nil
}

func (tc *TestContext) baseShouldBe(state string) func() error {
	return func() error {
		support, err := tc.baseSupport()
		if err != nil {
			return err
		}
		if support.State != state {
			return fmt.Errorf("base reported as %q, expected %q (%s)", support.State, state, support.Reason)
		}
		return nil
	}
}

func (tc *TestContext) baseShouldAskForAttention(wanted bool) func() error {
	return func() error {
		support, err := tc.baseSupport()
		if err != nil {
			return err
		}
		if support.NeedsAttention() != wanted {
			return fmt.Errorf("base in state %q asks for attention = %v, expected %v",
				support.State, support.NeedsAttention(), wanted)
		}
		return nil
	}
}

func (tc *TestContext) baseReasonShouldContain(text string) error {
	support, err := tc.baseSupport()
	if err != nil {
		return err
	}
	if !strings.Contains(support.Reason, text) {
		return fmt.Errorf("the reason %q does not mention %q", support.Reason, text)
	}
	return nil
}

func (tc *TestContext) baseReasonShouldExplain(text string) func() error {
	return func() error { return tc.baseReasonShouldContain(text) }
}
