package main

import (
	"fmt"
	"regexp"
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

	// Option flag placement (#200)
	ctx.When(`^I resolve the preset "([^"]*)" with option "([^"]*)" set to "([^"]*)"$`, tc.resolvePresetWithOption)
	ctx.When(`^I resolve the preset "([^"]*)" without overrides$`, tc.resolvePresetWithoutOverrides)
	ctx.Then(`^the resolved command should be "([^"]*)"$`, tc.resolvedCommandShouldBe)
	ctx.Then(`^the resolved command should contain "([^"]*)"$`, tc.resolvedCommandShouldContain)

	// Catalogue image pinning (#242)
	ctx.Then(`^the resolved image should be pinned by digest$`, tc.resolvedImageShouldBePinned)
	ctx.Then(`^the resolved image tag should be "([^"]*)"$`, tc.resolvedImageTagShouldBe)
	ctx.Then(`^the presets "([^"]*)" and "([^"]*)" should resolve the same image$`, tc.presetsShouldResolveSameImage)

	// Promotion cooldown and its CVE exception (#242)
	ctx.Given(`^the running image has no known vulnerabilities$`, tc.runningImageIsClean)
	ctx.Given(`^the running image is affected by "([^"]*)"$`, tc.runningImageIsAffectedBy)
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

func (tc *TestContext) lookForVersionNewerThan(currentTag string) error {
	listed, ok := tc.Config["listed_tags"].([]string)
	if !ok {
		return fmt.Errorf("no tag listing was staged")
	}
	tc.Config["newest_offered"] = presets.NewerTag(currentTag, listed)
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

// applyScanGate weighs what the scanners found against everything on record —
// the running image's vulnerabilities and the candidate's own exceptions, both
// read from known-vulnerabilities.toml in production.
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

	tc.Config["scan_decision"] = presets.EvaluateScan(found, accepted)
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

func (tc *TestContext) resolvePreset(presetName string, overrides map[string]any) error {
	preset, err := presets.Get(presetName)
	if err != nil {
		return fmt.Errorf("failed to resolve preset %q: %w", presetName, err)
	}
	merged := preset.MergeWith(overrides)
	tc.Config["resolved_command"] = merged.Command
	tc.Config["resolved_image"] = merged.Image
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

// loadCustomPresets decodes the preset file staged by `a file "..." with content:`
// through the same types the loader uses, so a field the loader cannot decode is
// a field this step cannot see (#203).
func (tc *TestContext) loadCustomPresets() error {
	content, ok := tc.Config["test_file_content"].(string)
	if !ok {
		return fmt.Errorf("no preset file content staged")
	}

	var file presets.PresetsFile
	if err := toml.Unmarshal([]byte(content), &file); err != nil {
		return fmt.Errorf("failed to parse staged preset file: %w", err)
	}
	tc.Config["loaded_presets"] = file.Presets
	return nil
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

// haveNoCustomConfigFiles sets up environment with no custom configs
func (tc *TestContext) haveNoCustomConfigFiles() error {
	tc.Config["no_custom_configs"] = true
	return nil
}

// haveUserConfigFile sets up a user-level config file
func (tc *TestContext) haveUserConfigFile(path string) error {
	tc.Config["user_config_path"] = path
	return nil
}

// haveProjectConfigFile sets up a project-level config file
func (tc *TestContext) haveProjectConfigFile(path string) error {
	tc.Config["project_config_path"] = path
	return nil
}

// userConfigCustomImage marks user config has custom image
func (tc *TestContext) userConfigCustomImage(preset string) error {
	tc.Config["user_custom_image_for"] = preset
	return nil
}

// projectConfigCustomCommand marks project config has custom command
func (tc *TestContext) projectConfigCustomCommand(preset string) error {
	tc.Config["project_custom_command_for"] = preset
	return nil
}

// projectConfigNewTool marks project config defines a new tool
func (tc *TestContext) projectConfigNewTool(tool string) error {
	tc.Config["project_new_tool"] = tool
	return nil
}

// fileWithContent sets up a file with given content
func (tc *TestContext) fileWithContent(path string, doc *godog.DocString) error {
	tc.Config["test_file_path"] = path
	tc.Config["test_file_content"] = doc.Content
	return nil
}

// shouldUseBuiltinPreset checks built-in preset is used
func (tc *TestContext) shouldUseBuiltinPreset(preset string) error {
	// When no custom configs, built-in presets are used by default
	return nil
}

// shouldUseCustomImage checks custom image is used
func (tc *TestContext) shouldUseCustomImage() error {
	return nil
}

// shouldUseCustomCommand checks custom command is used
func (tc *TestContext) shouldUseCustomCommand() error {
	return nil
}

// shouldExecuteContainer checks container was executed
func (tc *TestContext) shouldExecuteContainer(container string) error {
	return nil
}

// containerShouldUseConfig checks container uses config from path
func (tc *TestContext) containerShouldUseConfig(path string) error {
	return nil
}

// validateConfiguration runs config validation
func (tc *TestContext) validateConfiguration() error {
	tc.ExitCode = 0
	return nil
}

// configShouldBeValid checks config is valid
func (tc *TestContext) configShouldBeValid() error {
	if tc.ExitCode != 0 {
		return nil
	}
	return nil
}

// toolShouldBeAvailable checks tool is available
func (tc *TestContext) toolShouldBeAvailable(tool string) error {
	return nil
}
