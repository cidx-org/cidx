package main

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/cucumber/godog"
)

// RegisterPresetSteps registers preset-related step definitions
func RegisterPresetSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Custom preset loading
	ctx.Given(`^I have no custom configuration files$`, tc.haveNoCustomConfigFiles)
	ctx.Given(`^I have a user config file at "([^"]*)"$`, tc.haveUserConfigFile)
	ctx.Given(`^I have a project config file at "([^"]*)"$`, tc.haveProjectConfigFile)
	ctx.Given(`^the user config defines a custom image for "([^"]*)"$`, tc.userConfigCustomImage)
	ctx.Given(`^the project config defines a custom command for "([^"]*)"$`, tc.projectConfigCustomCommand)
	ctx.Given(`^the project config defines a new tool "([^"]*)"$`, tc.projectConfigNewTool)
	ctx.Given(`^a file "([^"]*)" with content:$`, tc.fileWithContent)

	// Preset assertions
	ctx.Then(`^it should use the built-in "([^"]*)" preset$`, tc.shouldUseBuiltinPreset)
	ctx.Then(`^it should use the custom image from the user config$`, tc.shouldUseCustomImage)
	ctx.Then(`^it should use the custom command from the project config$`, tc.shouldUseCustomCommand)
	ctx.Then(`^it should execute the "([^"]*)" container$`, tc.shouldExecuteContainer)
	ctx.Then(`^the container should use the configuration from "([^"]*)"$`, tc.containerShouldUseConfig)
	ctx.When(`^I validate the configuration$`, tc.validateConfiguration)
	ctx.When(`^I load the custom presets$`, tc.loadCustomPresets)
	ctx.Then(`^it should be valid$`, tc.configShouldBeValid)
	ctx.Then(`^the tool "([^"]*)" should be available$`, tc.toolShouldBeAvailable)
	ctx.Then(`^the preset "([^"]*)" should have "([^"]*)" set to "([^"]*)"$`, tc.presetFieldShouldEqual)
	ctx.Then(`^the key "([^"]*)" should be reported as unknown$`, tc.keyShouldBeReportedAsUnknown)

	// Option flag placement (#200)
	ctx.When(`^I resolve the preset "([^"]*)" with option "([^"]*)" set to "([^"]*)"$`, tc.resolvePresetWithOption)
	ctx.When(`^I resolve the preset "([^"]*)" without overrides$`, tc.resolvePresetWithoutOverrides)

	// A preset env value is a default the environment overrides (#384)
	ctx.Given(`^the environment sets "([^"]*)" to "([^"]*)"$`, tc.environmentSets)
	ctx.Given(`^the environment does not set "([^"]*)"$`, tc.environmentDoesNotSet)
	ctx.Then(`^the resolved command should be "([^"]*)"$`, tc.resolvedCommandShouldBe)
	ctx.Then(`^the resolved command should contain "([^"]*)"$`, tc.resolvedCommandShouldContain)
	ctx.Then(`^the resolved command should not contain "([^"]*)"$`, tc.resolvedCommandShouldNotContain)

	// Writable paths for a container that never runs as root (#188, #238)
	ctx.Then(`^the resolved environment should set "([^"]*)" to "([^"]*)"$`, tc.resolvedEnvShouldBe)

	// Catalogue image pinning (#242)
	ctx.Then(`^the resolved image should be pinned by digest$`, tc.resolvedImageShouldBePinned)
	ctx.Then(`^the resolved image tag should be "([^"]*)"$`, tc.resolvedImageTagShouldBe)
	ctx.Then(`^the presets "([^"]*)" and "([^"]*)" should resolve the same image$`, tc.presetsShouldResolveSameImage)

	// Promotion cooldown and its CVE exception (#242)
	ctx.Given(`^the running image has no known vulnerabilities$`, tc.runningImageIsClean)
	ctx.Given(`^the running image is affected by "([^"]*)"$`, tc.runningImageIsAffectedBy)
	ctx.Given(`^the running image's scan reports "([^"]*)"$`, tc.runningImageScanReports)
	ctx.Given(`^the running image's scan is unreadable$`, tc.runningImageScanIsUnreadable)
	ctx.Given(`^a candidate version published (\d+) days ago$`, tc.candidatePublishedDaysAgo)
	ctx.Given(`^a candidate version with no publication date$`, tc.candidateWithoutPublicationDate)
	ctx.When(`^the promotion policy is applied$`, tc.applyPromotionPolicy)
	ctx.Then(`^the candidate should be promoted$`, tc.candidateShouldBePromoted)
	ctx.Then(`^the candidate should be held$`, tc.candidateShouldBeHeld)
	ctx.Then(`^the promotion reason should mention "([^"]*)"$`, tc.promotionReasonShouldMention)
	ctx.Then(`^the waiver should name "([^"]*)"$`, tc.waiverShouldName)
	ctx.Then(`^no waiver should be reported$`, tc.noWaiverShouldBeReported)
	ctx.Then(`^the candidate age should be unreported$`, tc.candidateAgeShouldBeUnreported)

	// The scan gate the promotion actually depends on (#247)
	ctx.Given(`^the scanners report no finding on the candidate$`, tc.scannersReportNoFinding)
	ctx.Given(`^the scanners report "([^"]*)" on the candidate$`, tc.scannersReportFinding)
	ctx.Given(`^the finding "([^"]*)" is accepted for the candidate$`, tc.findingAcceptedForCandidate)
	ctx.When(`^the scan gate is applied$`, tc.applyScanGate)
	ctx.Then(`^the candidate should clear the scan gate$`, tc.candidateShouldClearTheScanGate)
	ctx.Then(`^the candidate should be held by the scan gate$`, tc.candidateShouldBeHeldByTheScanGate)
	ctx.Then(`^the scan verdict should mention "([^"]*)"$`, tc.scanVerdictShouldMention)
	ctx.Then(`^the scan verdict should name "([^"]*)"$`, tc.scanVerdictShouldName)
	ctx.Then(`^the scan verdict should not name "([^"]*)"$`, tc.scanVerdictShouldNotName)

	// Update detection over a registry tag listing (#245)
	ctx.Given(`^the registry lists the tags "([^"]*)"$`, tc.registryListsTags)
	ctx.When(`^I look for a version newer than "([^"]*)"$`, tc.lookForVersionNewerThan)
	ctx.Then(`^the newest version offered should be "([^"]*)"$`, tc.newestVersionOfferedShouldBe)
	ctx.Then(`^no newer version should be offered$`, tc.noNewerVersionShouldBeOffered)

	// A variant line upstream stopped publishing (#252)
	ctx.When(`^I look for the family that superseded "([^"]*)"$`, tc.lookForSupersedingFamily)
	ctx.Then(`^the superseding variant family should be "([^"]*)"$`, tc.supersedingFamilyShouldBe)
	ctx.Then(`^no superseding variant family should be reported$`, tc.noSupersedingFamilyShouldBeReported)

	// Development channels, and tags that carry no version at all (#328)
	ctx.Given(`^today is "([^"]*)"$`, tc.todayIs)
	ctx.When(`^I ask whether "([^"]*)" carries a version$`, tc.askWhetherTagCarriesAVersion)
	ctx.Then(`^the tag should carry a version$`, tc.tagShouldCarryAVersion)
	ctx.Then(`^the tag should carry no version$`, tc.tagShouldCarryNoVersion)
}

// registryListsTags stages the tag names a registry answers `tags/list` with.
// The order is deliberately not the version order: the OCI API promises none.
func (tc *TestContext) registryListsTags(tags string) error {
	var listed []string
	for _, tag := range strings.Split(tags, ",") {
		listed = append(listed, strings.TrimSpace(tag))
	}
	tc.Config["listed_tags"] = listed
	return nil
}

// todayIs fixes the day a scenario is read against. Only the rule that refuses
// a release dated in the future turns on it, and a scenario about October has
// to hold in every month (#328).
func (tc *TestContext) todayIs(day string) error {
	now, err := time.Parse("2006-01-02", day)
	if err != nil {
		return fmt.Errorf("could not read %q as a date: %w", day, err)
	}
	tc.Config["today"] = now
	return nil
}

// scenarioToday is the staged day, or the real one for the scenarios that do
// not care which it is.
func (tc *TestContext) scenarioToday() time.Time {
	if now, ok := tc.Config["today"].(time.Time); ok {
		return now
	}
	return time.Now()
}

func (tc *TestContext) lookForVersionNewerThan(currentTag string) error {
	listed, ok := tc.Config["listed_tags"].([]string)
	if !ok {
		return fmt.Errorf("no tag listing was staged")
	}
	tc.Config["newest_offered"] = presets.NewerTag(currentTag, listed, tc.scenarioToday())
	return nil
}

func (tc *TestContext) askWhetherTagCarriesAVersion(tag string) error {
	tc.Config["tag_is_versioned"] = presets.VersionedTag(tag)
	return nil
}

func (tc *TestContext) tagShouldCarryAVersion() error {
	versioned, ok := tc.Config["tag_is_versioned"].(bool)
	if !ok {
		return fmt.Errorf("no tag was asked about")
	}
	if !versioned {
		return fmt.Errorf("the tag was reported as carrying no version")
	}
	return nil
}

func (tc *TestContext) tagShouldCarryNoVersion() error {
	versioned, ok := tc.Config["tag_is_versioned"].(bool)
	if !ok {
		return fmt.Errorf("no tag was asked about")
	}
	if versioned {
		return fmt.Errorf("the tag was reported as carrying a version, so an update would be claimed for it")
	}
	return nil
}

func (tc *TestContext) newestVersionOfferedShouldBe(want string) error {
	got, ok := tc.Config["newest_offered"].(string)
	if !ok {
		return fmt.Errorf("no version was looked for")
	}
	if got != want {
		return fmt.Errorf("newest version offered = %q, want %q", got, want)
	}
	return nil
}

func (tc *TestContext) noNewerVersionShouldBeOffered() error {
	got, ok := tc.Config["newest_offered"].(string)
	if !ok {
		return fmt.Errorf("no version was looked for")
	}
	if got != "" {
		return fmt.Errorf("%q was offered although nothing newer qualifies", got)
	}
	return nil
}

func (tc *TestContext) lookForSupersedingFamily(currentTag string) error {
	listed, ok := tc.Config["listed_tags"].([]string)
	if !ok {
		return fmt.Errorf("no tag listing was staged")
	}
	tc.Config["superseding_family"] = presets.SupersedingVariant(currentTag, listed)
	return nil
}

func (tc *TestContext) supersedingFamilyShouldBe(want string) error {
	got, ok := tc.Config["superseding_family"].(string)
	if !ok {
		return fmt.Errorf("no superseding family was looked for")
	}
	if got != want {
		return fmt.Errorf("superseding variant family = %q, want %q", got, want)
	}
	return nil
}

func (tc *TestContext) noSupersedingFamilyShouldBeReported() error {
	got, ok := tc.Config["superseding_family"].(string)
	if !ok {
		return fmt.Errorf("no superseding family was looked for")
	}
	if got != "" {
		return fmt.Errorf("%q was reported although the pinned family is still published", got)
	}
	return nil
}

// promotionClock is the fixed "now" the cooldown scenarios are written against,
// so a scenario states an age rather than a date that would rot.
var promotionClock = time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)

// runningImageIsClean states that nothing known affects what the catalogue runs
// today, which is what makes the cooldown the only rule in play.
func (tc *TestContext) runningImageIsClean() error {
	tc.Config["affecting_us"] = []string(nil)
	return nil
}

// runningImageIsAffectedBy records a vulnerability of the image the catalogue
// runs today — rule 3's input, read from known-vulnerabilities.toml in
// production.
func (tc *TestContext) runningImageIsAffectedBy(cve string) error {
	affecting, _ := tc.Config["affecting_us"].([]string)
	tc.Config["affecting_us"] = append(affecting, cve)
	return nil
}

// runningImageScanReports records a finding of the running image as the
// monitor's own scanners report it — the record that exists whether or not a
// human has filed anything, and the one the gate was missing (#379).
func (tc *TestContext) runningImageScanReports(cve string) error {
	carried, _ := tc.Config["carried_findings"].([]string)
	tc.Config["carried_findings"] = append(carried, cve)
	return nil
}

// runningImageScanIsUnreadable is the fail-closed input: no same-day evidence
// of what the running image carries, so only the file vouches (#379).
func (tc *TestContext) runningImageScanIsUnreadable() error {
	tc.Config["carried_findings"] = []string(nil)
	tc.Config["carried_unreadable"] = true
	return nil
}

func (tc *TestContext) candidatePublishedDaysAgo(days int) error {
	tc.Config["candidate_published"] = promotionClock.AddDate(0, 0, -days)
	return nil
}

// candidateWithoutPublicationDate is the case a registry that will not say when
// a tag appeared leaves us in.
func (tc *TestContext) candidateWithoutPublicationDate() error {
	tc.Config["candidate_published"] = time.Time{}
	return nil
}

func (tc *TestContext) applyPromotionPolicy() error {
	published, ok := tc.Config["candidate_published"].(time.Time)
	if !ok {
		return fmt.Errorf("no candidate version was staged")
	}
	affecting, _ := tc.Config["affecting_us"].([]string)
	tc.Config["promotion_decision"] = presets.EvaluatePromotion(published, promotionClock, affecting)
	return nil
}

func (tc *TestContext) promotionDecision() (presets.PromotionDecision, error) {
	decision, ok := tc.Config["promotion_decision"].(presets.PromotionDecision)
	if !ok {
		return presets.PromotionDecision{}, fmt.Errorf("the promotion policy was not applied")
	}
	return decision, nil
}

func (tc *TestContext) candidateShouldBePromoted() error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	if !decision.Promote {
		return fmt.Errorf("candidate was held: %s", decision.Reason)
	}
	return nil
}

func (tc *TestContext) candidateShouldBeHeld() error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	if decision.Promote {
		return fmt.Errorf("candidate was promoted: %s", decision.Reason)
	}
	if decision.Reason == "" {
		return fmt.Errorf("candidate was held without a stated reason")
	}
	return nil
}

func (tc *TestContext) promotionReasonShouldMention(fragment string) error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	if !strings.Contains(decision.Reason, fragment) {
		return fmt.Errorf("reason %q does not mention %q", decision.Reason, fragment)
	}
	return nil
}

func (tc *TestContext) waiverShouldName(cve string) error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	for _, waived := range decision.WaivedFor {
		if waived == cve {
			if !strings.Contains(decision.Reason, cve) {
				return fmt.Errorf("waiver for %s is not stated in the reason %q", cve, decision.Reason)
			}
			return nil
		}
	}
	return fmt.Errorf("waiver %v does not name %s", decision.WaivedFor, cve)
}

func (tc *TestContext) noWaiverShouldBeReported() error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	if len(decision.WaivedFor) != 0 {
		return fmt.Errorf("a waiver was claimed for %v although none was needed", decision.WaivedFor)
	}
	return nil
}

func (tc *TestContext) candidateAgeShouldBeUnreported() error {
	decision, err := tc.promotionDecision()
	if err != nil {
		return err
	}
	if decision.AgeDays != nil {
		return fmt.Errorf("age reported as %d days although no publication date was known", *decision.AgeDays)
	}
	return nil
}

// scannersReportNoFinding is the candidate Trivy and Grype both came back clean
// on — stated explicitly rather than left implicit, so a scenario never passes
// because nothing was staged.
func (tc *TestContext) scannersReportNoFinding() error {
	tc.Config["scan_findings"] = []string(nil)
	return nil
}

// scannersReportFinding stages one HIGH/CRITICAL identifier the monitor's
// scanners reported against the candidate.
func (tc *TestContext) scannersReportFinding(id string) error {
	found, _ := tc.Config["scan_findings"].([]string)
	tc.Config["scan_findings"] = append(found, id)
	return nil
}

// findingAcceptedForCandidate is an exception filed against the candidate's own
// reference: reviewed and accepted before the promotion, rather than inherited
// from the running image.
func (tc *TestContext) findingAcceptedForCandidate(id string) error {
	accepted, _ := tc.Config["accepted_findings"].([]string)
	tc.Config["accepted_findings"] = append(accepted, id)
	return nil
}

// applyScanGate weighs what the scanners found against the two records of what
// we already run (#379): the acceptances — the running image's filed
// vulnerabilities and the candidate's own exceptions, both from
// known-vulnerabilities.toml in production — and the running image's same-day
// scan, which exists whether or not anyone filed anything.
func (tc *TestContext) applyScanGate() error {
	found, ok := tc.Config["scan_findings"].([]string)
	if !ok && tc.Config["scan_findings"] == nil {
		return fmt.Errorf("no scanner findings were staged for the candidate")
	}

	affecting, _ := tc.Config["affecting_us"].([]string)
	candidate, _ := tc.Config["accepted_findings"].([]string)

	accepted := make([]string, 0, len(affecting)+len(candidate))
	accepted = append(accepted, affecting...)
	accepted = append(accepted, candidate...)

	carried, _ := tc.Config["carried_findings"].([]string)
	if tc.Config["carried_unreadable"] == true {
		carried = nil
	}

	tc.Config["scan_decision"] = presets.EvaluateScan(found, accepted, carried)
	return nil
}

func (tc *TestContext) scanDecision() (presets.ScanDecision, error) {
	decision, ok := tc.Config["scan_decision"].(presets.ScanDecision)
	if !ok {
		return presets.ScanDecision{}, fmt.Errorf("the scan gate was not applied")
	}
	return decision, nil
}

func (tc *TestContext) candidateShouldClearTheScanGate() error {
	decision, err := tc.scanDecision()
	if err != nil {
		return err
	}
	if !decision.Promote {
		return fmt.Errorf("candidate was held by the scan gate: %s", decision.Reason)
	}
	return nil
}

func (tc *TestContext) candidateShouldBeHeldByTheScanGate() error {
	decision, err := tc.scanDecision()
	if err != nil {
		return err
	}
	if decision.Promote {
		return fmt.Errorf("candidate cleared the scan gate: %s", decision.Reason)
	}
	if decision.Reason == "" {
		return fmt.Errorf("candidate was held without a stated reason")
	}
	return nil
}

func (tc *TestContext) scanVerdictShouldMention(fragment string) error {
	decision, err := tc.scanDecision()
	if err != nil {
		return err
	}
	if !strings.Contains(decision.Reason, fragment) {
		return fmt.Errorf("scan verdict %q does not mention %q", decision.Reason, fragment)
	}
	return nil
}

// scanVerdictShouldName checks both halves of a hold: the finding is listed for
// the workflow annotation, and stated in the reason a human reads.
func (tc *TestContext) scanVerdictShouldName(id string) error {
	decision, err := tc.scanDecision()
	if err != nil {
		return err
	}
	for _, introduced := range decision.Introduces {
		if introduced == id {
			if !strings.Contains(decision.Reason, id) {
				return fmt.Errorf("%s is not stated in the verdict %q", id, decision.Reason)
			}
			return nil
		}
	}
	return fmt.Errorf("introduced findings %v do not name %s", decision.Introduces, id)
}

func (tc *TestContext) scanVerdictShouldNotName(id string) error {
	decision, err := tc.scanDecision()
	if err != nil {
		return err
	}
	for _, introduced := range decision.Introduces {
		if introduced == id {
			return fmt.Errorf("%s was blamed although it is already on record", id)
		}
	}
	if strings.Contains(decision.Reason, id) {
		return fmt.Errorf("verdict %q blames %s although it is already on record", decision.Reason, id)
	}
	return nil
}

// pinnedImageRef is the reference form rule 1 of the image supply-chain policy
// requires: a readable tag plus the digest that makes it immutable.
var pinnedImageRef = regexp.MustCompile(`^[^@\s]+:[^@:/\s]+@sha256:[0-9a-f]{64}$`)

// resolvedImageShouldBePinned asserts the reference a run would pull carries
// both a tag and a digest.
func (tc *TestContext) resolvedImageShouldBePinned() error {
	image, ok := tc.Config["resolved_image"].(string)
	if !ok || image == "" {
		return fmt.Errorf("no preset was resolved")
	}
	if !pinnedImageRef.MatchString(image) {
		return fmt.Errorf("image %q is not pinned as image:tag@sha256:...", image)
	}
	return nil
}

// resolvedImageTagShouldBe asserts the tag remains readable next to the digest.
func (tc *TestContext) resolvedImageTagShouldBe(expected string) error {
	image, ok := tc.Config["resolved_image"].(string)
	if !ok || image == "" {
		return fmt.Errorf("no preset was resolved")
	}
	ref, _, _ := strings.Cut(image, "@")
	_, tag, found := strings.Cut(ref, ":")
	if !found {
		return fmt.Errorf("image %q carries no tag", image)
	}
	if tag != expected {
		return fmt.Errorf("expected tag %q, got %q (image %q)", expected, tag, image)
	}
	return nil
}

// presetsShouldResolveSameImage asserts that presets sharing an image share its
// digest too — otherwise "pinned" would mean two different things depending on
// which preset you ran.
func (tc *TestContext) presetsShouldResolveSameImage(first, second string) error {
	firstPreset, err := presets.Get(first)
	if err != nil {
		return fmt.Errorf("failed to resolve preset %q: %w", first, err)
	}
	secondPreset, err := presets.Get(second)
	if err != nil {
		return fmt.Errorf("failed to resolve preset %q: %w", second, err)
	}
	if firstPreset.Image != secondPreset.Image {
		return fmt.Errorf("preset %q uses %q but preset %q uses %q",
			first, firstPreset.Image, second, secondPreset.Image)
	}
	return nil
}

// resolvePresetWithOption merges a single user option override into a built-in
// preset, exactly as cidx.toml's `[containers.NAME]` section does at runtime.
func (tc *TestContext) resolvePresetWithOption(presetName, option, value string) error {
	return tc.resolvePreset(presetName, map[string]any{option: value})
}

// resolvePresetWithoutOverrides resolves a built-in preset as-is.
func (tc *TestContext) resolvePresetWithoutOverrides(presetName string) error {
	return tc.resolvePreset(presetName, map[string]any{})
}

// environmentSets exports a variable for the scenario, remembering it so the
// suite's cleanup puts the environment back (#384).
func (tc *TestContext) environmentSets(key, value string) error {
	tc.rememberEnv(key)
	return os.Setenv(key, value)
}

func (tc *TestContext) environmentDoesNotSet(key string) error {
	tc.rememberEnv(key)
	return os.Unsetenv(key)
}

// rememberEnv records what a variable held before a scenario touched it, so
// the suite's cleanup restores it rather than unsetting it: HOME is one of
// these, and dropping it altogether breaks every scenario that follows.
func (tc *TestContext) rememberEnv(key string) {
	touched, _ := tc.Config["touched_env"].(map[string]*string)
	if touched == nil {
		touched = make(map[string]*string)
		tc.Config["touched_env"] = touched
	}
	if _, already := touched[key]; already {
		return
	}
	if before, set := os.LookupEnv(key); set {
		touched[key] = &before
	} else {
		touched[key] = nil
	}
}

func (tc *TestContext) resolvePreset(presetName string, overrides map[string]any) error {
	preset, err := presets.Get(presetName)
	if err != nil {
		return fmt.Errorf("failed to resolve preset %q: %w", presetName, err)
	}
	merged := preset.MergeWith(overrides)
	// Resolved the way the executor resolves it, so a scenario sees the
	// command the container would really be given (#384).
	tc.Config["resolved_command"] = presets.ExpandCommand(merged.Command, merged.Env)
	tc.Config["resolved_image"] = merged.Image
	// Resolved the way the container receives it. Asserting on the declared
	// map instead is how the guard below passed while the runner's HOME was
	// in fact replacing the preset's (#384).
	tc.Config["resolved_env"] = presets.ResolveContainerEnv(merged.Command, merged.Env)
	return nil
}

// resolvedEnvShouldBe asserts a variable the container needs to be able to
// write anywhere: cidx runs as the invoking uid, so HOME and GOPATH decide
// whether a runtime install lands in a writable tree or dies (#238).
func (tc *TestContext) resolvedEnvShouldBe(name, want string) error {
	env, ok := tc.Config["resolved_env"].(map[string]string)
	if !ok {
		return fmt.Errorf("no preset was resolved")
	}
	got, set := env[name]
	if !set {
		return fmt.Errorf("environment variable %q is not set", name)
	}
	if got != want {
		return fmt.Errorf("environment variable %q = %q, want %q", name, got, want)
	}
	return nil
}

// resolvedCommandShouldBe asserts the exact command handed to the executor.
func (tc *TestContext) resolvedCommandShouldBe(want string) error {
	got, ok := tc.Config["resolved_command"].(string)
	if !ok {
		return fmt.Errorf("no preset resolved")
	}
	if got != want {
		return fmt.Errorf("resolved command = %q, want %q", got, want)
	}
	return nil
}

// resolvedCommandShouldContain asserts a fragment of the resolved command, for
// commands too long to spell out in full.
func (tc *TestContext) resolvedCommandShouldContain(want string) error {
	got, ok := tc.Config["resolved_command"].(string)
	if !ok {
		return fmt.Errorf("no preset resolved")
	}
	if !strings.Contains(got, want) {
		return fmt.Errorf("resolved command %q does not contain %q", got, want)
	}
	return nil
}

// resolvedCommandShouldNotContain asserts a fragment is absent from the resolved
// command — the shape #278's family of defects takes, where what is wrong with a
// command is something extra in it rather than something missing.
func (tc *TestContext) resolvedCommandShouldNotContain(unwanted string) error {
	got, ok := tc.Config["resolved_command"].(string)
	if !ok {
		return fmt.Errorf("no preset resolved")
	}
	if strings.Contains(got, unwanted) {
		return fmt.Errorf("resolved command %q contains %q", got, unwanted)
	}
	return nil
}

// loadCustomPresets decodes the preset file staged by `a file "..." with content:`
// through the same types the loader uses, so a field the loader cannot decode is
// a field this step cannot see (#203). The undecoded keys are kept: they are
// exactly what the loader warns about, and what a removed field — such as an
// option `default` (#299) — now lands in.
func (tc *TestContext) loadCustomPresets() error {
	content, ok := tc.Config["test_file_content"].(string)
	if !ok {
		return fmt.Errorf("no preset file content staged")
	}

	var file presets.PresetsFile
	md, err := toml.Decode(content, &file)
	if err != nil {
		return fmt.Errorf("failed to parse staged preset file: %w", err)
	}

	unknown := make([]string, 0, len(md.Undecoded()))
	for _, key := range md.Undecoded() {
		unknown = append(unknown, key.String())
	}

	tc.Config["loaded_presets"] = file.Presets
	tc.Config["unknown_preset_keys"] = unknown
	return nil
}

// keyShouldBeReportedAsUnknown asserts a key reached the loader's warning path
// instead of being accepted and doing nothing.
func (tc *TestContext) keyShouldBeReportedAsUnknown(key string) error {
	unknown, ok := tc.Config["unknown_preset_keys"].([]string)
	if !ok {
		return fmt.Errorf("no preset file was loaded")
	}
	for _, got := range unknown {
		if got == key {
			return nil
		}
	}
	return fmt.Errorf("key %q was decoded silently; unknown keys reported: %v", key, unknown)
}

// presetFieldShouldEqual asserts a loaded preset carries the expected value for
// one of its execution fields.
func (tc *TestContext) presetFieldShouldEqual(presetName, field, want string) error {
	loaded, ok := tc.Config["loaded_presets"].(map[string]presets.PresetTOML)
	if !ok {
		return fmt.Errorf("no presets loaded")
	}
	preset, ok := loaded[presetName]
	if !ok {
		return fmt.Errorf("preset %q not loaded", presetName)
	}

	var got string
	switch field {
	case "pull_policy":
		got = preset.PullPolicy
	case "timeout":
		got = preset.Timeout
	case "image":
		got = preset.Image
	case "command":
		got = preset.Command
	case "workdir":
		got = preset.Workdir
	default:
		return fmt.Errorf("unsupported preset field %q", field)
	}

	if got != want {
		return fmt.Errorf("preset %q field %q = %q, want %q", presetName, field, got, want)
	}
	return nil
}

// presetLayer is one preset file a scenario put on the stack: the path it
// stands at, and the presets it declares, parsed by the loader's own types.
type presetLayer struct {
	Path    string
	Presets map[string]presets.PresetTOML
}

// resolvedTool is a tool after the stack has been walked: what it resolves to,
// and which layer said so.
type resolvedTool struct {
	Image   string
	Command string
	Source  string
}

const catalogueSource = "the built-in catalogue"

// haveNoCustomConfigFiles empties the stack, so only the catalogue answers.
func (tc *TestContext) haveNoCustomConfigFiles() error {
	tc.Config["preset_layers"] = []presetLayer{}
	return nil
}

func (tc *TestContext) haveUserConfigFile(path string) error {
	return tc.pushPresetLayer(path)
}

func (tc *TestContext) haveProjectConfigFile(path string) error {
	return tc.pushPresetLayer(path)
}

// pushPresetLayer adds an empty layer at the given path. loadPresets stacks
// them in this order — the catalogue, then ~/.config/cidx, then .cidx — so the
// order the scenario states them in is the order they win in.
func (tc *TestContext) pushPresetLayer(path string) error {
	layers, _ := tc.Config["preset_layers"].([]presetLayer)
	tc.Config["preset_layers"] = append(layers, presetLayer{Path: path})
	return nil
}

// declareInTopLayer parses a preset declaration through presets.PresetsFile —
// the type the loader decodes into — and puts it in the layer last pushed.
func (tc *TestContext) declareInTopLayer(document string) error {
	layers, _ := tc.Config["preset_layers"].([]presetLayer)
	if len(layers) == 0 {
		return fmt.Errorf("the scenario declared a preset before naming a file to hold it")
	}

	var file presets.PresetsFile
	if _, err := toml.Decode(document, &file); err != nil {
		return fmt.Errorf("failed to parse the staged preset file: %w", err)
	}

	top := &layers[len(layers)-1]
	if top.Presets == nil {
		top.Presets = map[string]presets.PresetTOML{}
	}
	for name, preset := range file.Presets {
		top.Presets[name] = preset
	}
	tc.Config["preset_layers"] = layers
	return nil
}

// userConfigCustomImage declares an image for an existing preset, the override
// a user file is normally written for.
func (tc *TestContext) userConfigCustomImage(preset string) error {
	return tc.declareInTopLayer(fmt.Sprintf(`
[presets.%s]
name = %q
phase = "security"
image = "registry.example.com/%s:pinned"
command = "scan ."
`, preset, preset, preset))
}

// projectConfigCustomCommand declares a command for an existing preset.
func (tc *TestContext) projectConfigCustomCommand(preset string) error {
	return tc.declareInTopLayer(fmt.Sprintf(`
[presets.%s]
name = %q
phase = "security"
image = "aquasec/trivy:latest"
command = "fs . --scanners vuln --exit-code 1"
`, preset, preset))
}

// projectConfigNewTool declares a preset the catalogue has never heard of.
func (tc *TestContext) projectConfigNewTool(tool string) error {
	return tc.declareInTopLayer(fmt.Sprintf(`
[presets.%s]
name = %q
phase = "code"
image = "alpine:latest"
command = "lint ."
`, tool, tool))
}

// fileWithContent sets up a file with given content
func (tc *TestContext) fileWithContent(path string, doc *godog.DocString) error {
	tc.Config["test_file_path"] = path
	tc.Config["test_file_content"] = doc.Content
	return nil
}

// resolveTool answers what a tool resolves to the way loadPresets does: the
// catalogue presets.toml ships, then each staged file on top, a later file
// replacing an earlier definition of the same name. The catalogue side is the
// real one (presets.Catalogue reads the file), the layers are parsed by the
// loader's own types; replace-by-name is the whole of mergePresets.
func (tc *TestContext) resolveTool(name string) (resolvedTool, error) {
	catalogue, err := presets.Catalogue()
	if err != nil {
		return resolvedTool{}, fmt.Errorf("failed to read the preset catalogue: %w", err)
	}

	resolved := resolvedTool{}
	found := false
	if preset, ok := catalogue[name]; ok {
		resolved = resolvedTool{Image: preset.Image, Command: preset.Command, Source: catalogueSource}
		found = true
	}

	layers, _ := tc.Config["preset_layers"].([]presetLayer)
	for _, layer := range layers {
		preset, ok := layer.Presets[name]
		if !ok {
			continue
		}
		resolved = resolvedTool{Image: preset.Image, Command: preset.Command, Source: layer.Path}
		found = true
	}

	if !found {
		return resolvedTool{}, fmt.Errorf("nothing declares a preset called %q", name)
	}
	return resolved, nil
}

// toolUnderTest is the tool the scenario's `cidx run <tool>` named.
func (tc *TestContext) toolUnderTest() (string, error) {
	fields := strings.Fields(tc.LastCommand)
	if len(fields) < 3 || fields[0] != "cidx" || fields[1] != "run" {
		return "", fmt.Errorf("no tool was run (last command: %q)", tc.LastCommand)
	}
	return fields[2], nil
}

// shouldUseBuiltinPreset asserts nothing shadowed the catalogue entry, and that
// what resolved is byte for byte what presets.toml ships.
func (tc *TestContext) shouldUseBuiltinPreset(name string) error {
	resolved, err := tc.resolveTool(name)
	if err != nil {
		return err
	}
	if resolved.Source != catalogueSource {
		return fmt.Errorf("preset %q comes from %s, not from the catalogue", name, resolved.Source)
	}

	catalogue, err := presets.Catalogue()
	if err != nil {
		return err
	}
	builtin := catalogue[name]
	if resolved.Image != builtin.Image || resolved.Command != builtin.Command {
		return fmt.Errorf("preset %q resolved to %q / %q, the catalogue ships %q / %q",
			name, resolved.Image, resolved.Command, builtin.Image, builtin.Command)
	}
	return nil
}

// shouldUseCustomImage asserts the user file won the image, and that it really
// changed something.
func (tc *TestContext) shouldUseCustomImage() error {
	return tc.customLayerShouldWin(func(resolved resolvedTool, builtin presets.Preset) error {
		if resolved.Image == builtin.Image {
			return fmt.Errorf("the image is still the catalogue's: %s", resolved.Image)
		}
		return nil
	})
}

// shouldUseCustomCommand asserts the project file won the command.
func (tc *TestContext) shouldUseCustomCommand() error {
	return tc.customLayerShouldWin(func(resolved resolvedTool, builtin presets.Preset) error {
		if resolved.Command == builtin.Command {
			return fmt.Errorf("the command is still the catalogue's: %s", resolved.Command)
		}
		return nil
	})
}

func (tc *TestContext) customLayerShouldWin(differs func(resolvedTool, presets.Preset) error) error {
	name, err := tc.toolUnderTest()
	if err != nil {
		return err
	}
	resolved, err := tc.resolveTool(name)
	if err != nil {
		return err
	}
	if resolved.Source == catalogueSource {
		return fmt.Errorf("preset %q still resolves to the catalogue entry", name)
	}
	catalogue, err := presets.Catalogue()
	if err != nil {
		return err
	}
	return differs(resolved, catalogue[name])
}

// shouldExecuteContainer asserts the container the scenario ran is the one that
// resolved, and that it resolved to something runnable.
func (tc *TestContext) shouldExecuteContainer(container string) error {
	name, err := tc.toolUnderTest()
	if err != nil {
		return err
	}
	if name != container {
		return fmt.Errorf("the run was of %q, not %q", name, container)
	}
	resolved, err := tc.resolveTool(container)
	if err != nil {
		return err
	}
	if resolved.Image == "" || resolved.Command == "" {
		return fmt.Errorf("preset %q resolves to image %q and command %q — nothing would run",
			container, resolved.Image, resolved.Command)
	}
	return nil
}

// containerShouldUseConfig asserts the definition that won came from the file
// the scenario names.
func (tc *TestContext) containerShouldUseConfig(path string) error {
	name, err := tc.toolUnderTest()
	if err != nil {
		return err
	}
	resolved, err := tc.resolveTool(name)
	if err != nil {
		return err
	}
	if resolved.Source != path {
		return fmt.Errorf("preset %q comes from %s, not from %s", name, resolved.Source, path)
	}
	return nil
}

// validateConfiguration runs the staged file through the loader's parse path
// and records what it made of it, without failing the step: whether the file is
// acceptable is what the next step asserts.
func (tc *TestContext) validateConfiguration() error {
	delete(tc.Config, "validation_error")
	if err := tc.loadCustomPresets(); err != nil {
		tc.Config["validation_error"] = err.Error()
		tc.ExitCode = 1
		return nil
	}
	tc.ExitCode = 0
	return nil
}

// configShouldBeValid asserts the loader both parsed the file and claimed every
// key in it — a key it drops is a field the user silently does not get (#203).
func (tc *TestContext) configShouldBeValid() error {
	if problem, failed := tc.Config["validation_error"].(string); failed {
		return fmt.Errorf("the loader refused the file: %s", problem)
	}
	unknown, ok := tc.Config["unknown_preset_keys"].([]string)
	if !ok {
		return fmt.Errorf("no configuration was validated")
	}
	if len(unknown) > 0 {
		return fmt.Errorf("the loader would drop unknown key(s): %v", unknown)
	}
	return nil
}

// toolShouldBeAvailable asserts the file really defines the tool, with enough
// of a definition to run.
func (tc *TestContext) toolShouldBeAvailable(tool string) error {
	loaded, ok := tc.Config["loaded_presets"].(map[string]presets.PresetTOML)
	if !ok {
		return fmt.Errorf("no preset file was loaded")
	}
	preset, ok := loaded[tool]
	if !ok {
		declared := make([]string, 0, len(loaded))
		for name := range loaded {
			declared = append(declared, name)
		}
		sort.Strings(declared)
		return fmt.Errorf("the file defines no preset %q (it defines %v)", tool, declared)
	}
	if preset.Image == "" || preset.Command == "" {
		return fmt.Errorf("preset %q declares image %q and command %q — nothing would run",
			tool, preset.Image, preset.Command)
	}
	return nil
}
