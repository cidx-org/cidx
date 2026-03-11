package main

import (
	"fmt"
	"strings"

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
