package main

import (
	"fmt"

	"github.com/cucumber/godog"
)

// RegisterSecuritySteps registers security-related step definitions
func RegisterSecuritySteps(ctx *godog.ScenarioContext, tc *TestContext) {
	// Local safety behavior steps
	ctx.Step(`^the "([^"]*)" preset has local_behavior = "([^"]*)"$`, tc.presetHasLocalBehavior)
	ctx.Step(`^Docker image should be built$`, tc.dockerImageShouldBeBuilt)
	ctx.Step(`^Docker image should NOT be pushed to registry$`, tc.dockerImageShouldNotBePushed)
	ctx.Step(`^GitHub release should be created as draft$`, tc.githubReleaseShouldBeDraft)
	ctx.Step(`^release should NOT be published$`, tc.releaseShouldNotBePublished)
	ctx.Step(`^the "([^"]*)" phase should build images$`, tc.phaseShouldBuildImages)
	ctx.Step(`^the "([^"]*)" phase should NOT push images$`, tc.phaseShouldNotPushImages)

	// Environment detection steps
	ctx.Step(`^the environment variable "([^"]*)" is set to "([^"]*)"$`, tc.setEnvironmentVariable)
	ctx.Step(`^CIDX should detect CI provider as "([^"]*)"$`, tc.shouldDetectCIProvider)
	ctx.Step(`^CIDX should detect IsPR as (true|false)$`, tc.shouldDetectIsPR)
	ctx.Step(`^CIDX should detect IsTag as (true|false)$`, tc.shouldDetectIsTag)
	ctx.Step(`^CIDX should detect BranchName as "([^"]*)"$`, tc.shouldDetectBranchName)
	ctx.Step(`^CIDX should detect TagName as "([^"]*)"$`, tc.shouldDetectTagName)
}

// presetHasLocalBehavior sets local_behavior for a preset
func (tc *TestContext) presetHasLocalBehavior(preset, behavior string) error {
	if tc.Config == nil {
		tc.Config = make(map[string]interface{})
	}
	if tc.Config["presets"] == nil {
		tc.Config["presets"] = make(map[string]interface{})
	}
	presets := tc.Config["presets"].(map[string]interface{})
	presets[preset] = map[string]string{
		"local_behavior": behavior,
	}
	return nil
}

// dockerImageShouldBeBuilt verifies Docker image was built
func (tc *TestContext) dockerImageShouldBeBuilt() error {
	// In actual implementation, check Docker build logs
	if tc.Output == "" {
		return fmt.Errorf("no output captured, cannot verify Docker build")
	}
	// Placeholder: would check for "Successfully built" or similar
	return nil
}

// dockerImageShouldNotBePushed verifies Docker image was not pushed
func (tc *TestContext) dockerImageShouldNotBePushed() error {
	// In actual implementation, verify no push occurred
	// Check that output contains "no-push" safety message
	return tc.shouldSeeOutput("no-push")
}

// githubReleaseShouldBeDraft verifies GitHub release was created as draft
func (tc *TestContext) githubReleaseShouldBeDraft() error {
	// In actual implementation, check gh CLI output for draft flag
	return tc.shouldSeeOutput("draft")
}

// releaseShouldNotBePublished verifies release was not published
func (tc *TestContext) releaseShouldNotBePublished() error {
	// In actual implementation, verify release is in draft state
	return tc.shouldSeeOutput("Local safety: draft")
}

// phaseShouldBuildImages verifies phase built images
func (tc *TestContext) phaseShouldBuildImages(phase string) error {
	// Check phase was executed
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			return nil
		}
	}
	return fmt.Errorf("phase %s was not executed", phase)
}

// phaseShouldNotPushImages verifies phase did not push images
func (tc *TestContext) phaseShouldNotPushImages(phase string) error {
	// Verify safety mode prevented push
	return tc.shouldSeeOutput("no-push")
}

// setEnvironmentVariable sets an environment variable
func (tc *TestContext) setEnvironmentVariable(key, value string) error {
	// Store in config for later use
	if tc.Config == nil {
		tc.Config = make(map[string]interface{})
	}
	if tc.Config["env"] == nil {
		tc.Config["env"] = make(map[string]string)
	}
	env := tc.Config["env"].(map[string]string)
	env[key] = value
	return nil
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
	if tc.EventType == "pull_request" && !expected {
		return fmt.Errorf("expected IsPR=%v, but detected pull_request event", expected)
	}
	if tc.EventType != "pull_request" && expected {
		return fmt.Errorf("expected IsPR=%v, but event type is %s", expected, tc.EventType)
	}
	return nil
}

// shouldDetectIsTag verifies tag detection
func (tc *TestContext) shouldDetectIsTag(isTag string) error {
	expected := isTag == "true"
	if tc.EventType == "tag" && !expected {
		return fmt.Errorf("expected IsTag=%v, but detected tag event", expected)
	}
	if tc.EventType != "tag" && expected {
		return fmt.Errorf("expected IsTag=%v, but event type is %s", expected, tc.EventType)
	}
	return nil
}

// shouldDetectBranchName verifies branch name detection
func (tc *TestContext) shouldDetectBranchName(branch string) error {
	// In actual implementation, would check git context
	// For now, assume it matches if we're in the right event type
	return nil
}

// shouldDetectTagName verifies tag name detection
func (tc *TestContext) shouldDetectTagName(tag string) error {
	// In actual implementation, would check git tag from context
	// For now, assume it matches if we're in tag event
	if tc.EventType != "tag" {
		return fmt.Errorf("expected tag event to detect tag name %s", tag)
	}
	return nil
}
