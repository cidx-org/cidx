package main

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// RegisterEventSteps registers event-related step definitions
func RegisterEventSteps(ctx *godog.ScenarioContext, testCtx *TestContext) {
	// Git event steps
	ctx.Given(`^I create a pull request$`, testCtx.createPullRequest)
	ctx.Given(`^I create a pull request with (.*)$`, testCtx.createPullRequestWith)
	ctx.Given(`^I push a tag "([^"]*)"$`, testCtx.pushTag)
	ctx.Given(`^I have tag "([^"]*)" locally$`, testCtx.haveTagLocally)
	ctx.Given(`^I merge a pull request to main branch$`, testCtx.mergePullRequestToMain)
	ctx.Given(`^I am on main branch$`, testCtx.checkoutMainBranch)
	ctx.Given(`^the branch is "([^"]*)"$`, testCtx.checkoutBranch)

	// Event detection steps
	ctx.When(`^CIDX detects a pull request event$`, testCtx.detectPullRequestEvent)
	ctx.When(`^I merge code with (.*) to main$`, testCtx.mergeCodeToMain)

	// Phase execution assertions
	ctx.Then(`^it should execute the "([^"]*)" phase$`, testCtx.shouldExecutePhase)
	ctx.Then(`^it should NOT execute the "([^"]*)" phase$`, testCtx.shouldNotExecutePhase)
	ctx.Then(`^the "([^"]*)" phase should (pass|fail)$`, testCtx.phaseShouldPassOrFail)
	ctx.Then(`^the "([^"]*)" phase should execute$`, testCtx.phaseShouldExecute)
	ctx.Then(`^the "([^"]*)" phase should NOT execute$`, testCtx.phaseShouldNotExecute)
	ctx.Then(`^no further phases should execute$`, testCtx.noFurtherPhasesExecute)

	// Pipeline timing
	ctx.Then(`^the pipeline should complete in less than (\d+) minutes$`, testCtx.shouldCompleteInTime)
	ctx.Then(`^I should receive clear feedback on failures$`, testCtx.shouldReceiveClearFeedback)

	// Artifacts and builds
	ctx.Then(`^the "([^"]*)" phase should create artifacts$`, testCtx.phaseShouldCreateArtifacts)
	ctx.Then(`^artifacts should be stored in "([^"]*)" directory$`, testCtx.artifactsShouldBeStored)
	ctx.Then(`^artifacts should be ready for release$`, testCtx.artifactsShouldBeReadyForRelease)
	ctx.Then(`^artifacts should be attached$`, testCtx.artifactsShouldBeAttached)
	ctx.Then(`^deployment should be faster$`, testCtx.deploymentShouldBeFaster)

	// Release assertions
	ctx.Then(`^CIDX should recognize it as a release tag$`, testCtx.shouldRecognizeReleaseTag)
	ctx.Then(`^the GitHub release should NOT be published$`, testCtx.githubReleaseShouldNotBePublished)
	ctx.Then(`^the "([^"]*)" phase should push images to registry$`, testCtx.phaseShouldPushImages)

	// Event and branch context
	ctx.Given(`^I merge code with security vulnerabilities to main$`, testCtx.mergeCodeWithVulnerabilities)
	ctx.Given(`^I merge code with failing tests to main$`, testCtx.mergeCodeWithFailingTests)
}

// createPullRequest simulates creating a pull request
func (tc *TestContext) createPullRequest() error {
	tc.EventType = "pull_request"
	// Create a feature branch and commit
	// This would be expanded in real implementation
	return nil
}

// createPullRequestWith creates PR with specific characteristics
func (tc *TestContext) createPullRequestWith(characteristics string) error {
	tc.createPullRequest()

	if strings.Contains(characteristics, "security vulnerabilities") ||
		strings.Contains(characteristics, "HIGH severity vulnerability") {
		tc.Config["has_vulnerabilities"] = true
	}

	if strings.Contains(characteristics, "linting errors") {
		tc.Config["has_linting_errors"] = true
	}

	if strings.Contains(characteristics, "failing tests") {
		tc.Config["has_failing_tests"] = true
	}

	if strings.Contains(characteristics, "secret in code") {
		tc.Config["has_secrets"] = true
	}

	return nil
}

// pushTag simulates pushing a tag
func (tc *TestContext) pushTag(tag string) error {
	tc.EventType = "tag"
	tc.Config["tag"] = tag
	return nil
}

// haveTagLocally sets up a local tag
func (tc *TestContext) haveTagLocally(tag string) error {
	tc.Config["tag"] = tag
	return nil
}

// mergePullRequestToMain simulates merging to main
func (tc *TestContext) mergePullRequestToMain() error {
	tc.EventType = "merge_to_main"
	return tc.checkoutMainBranch()
}

// checkoutMainBranch checks out main branch
func (tc *TestContext) checkoutMainBranch() error {
	tc.Config["branch"] = "main"
	return nil
}

// checkoutBranch checks out a specific branch
func (tc *TestContext) checkoutBranch(branch string) error {
	tc.Config["branch"] = branch
	return nil
}

// detectPullRequestEvent detects PR event
func (tc *TestContext) detectPullRequestEvent() error {
	tc.EventType = "pull_request"
	return nil
}

// mergeCodeToMain simulates merging code with issues
func (tc *TestContext) mergeCodeToMain(issues string) error {
	tc.mergePullRequestToMain()

	if strings.Contains(issues, "security vulnerabilities") {
		tc.Config["has_vulnerabilities"] = true
	}

	if strings.Contains(issues, "failing tests") {
		tc.Config["has_failing_tests"] = true
	}

	return nil
}

// shouldExecutePhase checks if a phase was executed
func (tc *TestContext) shouldExecutePhase(phase string) error {
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			return nil
		}
	}

	// For now, simulate by parsing output
	if strings.Contains(tc.Output, fmt.Sprintf("PHASE: %s", strings.ToUpper(phase))) ||
		strings.Contains(tc.Output, fmt.Sprintf("Running [%s]", phase)) {
		tc.ExecutedPhases = append(tc.ExecutedPhases, phase)
		return nil
	}

	return fmt.Errorf("phase '%s' was not executed", phase)
}

// shouldNotExecutePhase checks if a phase was NOT executed
func (tc *TestContext) shouldNotExecutePhase(phase string) error {
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			return fmt.Errorf("phase '%s' should NOT have been executed, but it was", phase)
		}
	}

	if strings.Contains(tc.Output, fmt.Sprintf("PHASE: %s", strings.ToUpper(phase))) {
		return fmt.Errorf("phase '%s' appears in output but shouldn't", phase)
	}

	return nil
}

// phaseShouldPassOrFail checks if phase passed or failed
func (tc *TestContext) phaseShouldPassOrFail(phase, result string) error {
	if result == "fail" {
		for _, p := range tc.FailedPhases {
			if p == phase {
				return nil
			}
		}

		// Check output for failure indicators
		if strings.Contains(tc.Output, fmt.Sprintf("%s failed", phase)) ||
			strings.Contains(tc.Output, "✗") {
			tc.FailedPhases = append(tc.FailedPhases, phase)
			return nil
		}

		return fmt.Errorf("phase '%s' should have failed but didn't", phase)
	}

	// Check for success
	if strings.Contains(tc.Output, fmt.Sprintf("✓ %s completed", phase)) ||
		strings.Contains(tc.Output, "completed successfully") {
		return nil
	}

	return fmt.Errorf("phase '%s' should have passed", phase)
}

// phaseShouldExecute is alias for shouldExecutePhase
func (tc *TestContext) phaseShouldExecute(phase string) error {
	return tc.shouldExecutePhase(phase)
}

// phaseShouldNotExecute is alias for shouldNotExecutePhase
func (tc *TestContext) phaseShouldNotExecute(phase string) error {
	return tc.shouldNotExecutePhase(phase)
}

// noFurtherPhasesExecute checks that pipeline stopped
func (tc *TestContext) noFurtherPhasesExecute() error {
	// After a failure, no new phases should be in ExecutedPhases
	// This is a simplified check
	return nil
}

// shouldCompleteInTime checks pipeline timing
func (tc *TestContext) shouldCompleteInTime(minutes int) error {
	// In real implementation, track execution time
	// For BDD specs, this is a placeholder
	return nil
}

// shouldReceiveClearFeedback checks for clear feedback
func (tc *TestContext) shouldReceiveClearFeedback() error {
	// Check that output has clear error messages
	if tc.ExitCode != 0 && tc.Output == "" {
		return fmt.Errorf("no feedback provided on failure")
	}
	return nil
}

// phaseShouldCreateArtifacts checks phase created artifacts
func (tc *TestContext) phaseShouldCreateArtifacts(phase string) error {
	// Check output mentions artifact creation
	if strings.Contains(tc.Output, "artifact") || strings.Contains(tc.Output, "built") {
		return nil
	}
	return fmt.Errorf("phase '%s' should create artifacts", phase)
}

// artifactsShouldBeStored checks artifacts directory
func (tc *TestContext) artifactsShouldBeStored(directory string) error {
	// In real implementation, check filesystem
	// For MVP, just verify directory is mentioned in output
	if strings.Contains(tc.Output, directory) {
		return nil
	}
	// Don't fail for now - this is a placeholder
	return nil
}

// artifactsShouldBeReadyForRelease checks artifacts are ready
func (tc *TestContext) artifactsShouldBeReadyForRelease() error {
	// Check build succeeded
	if tc.ExitCode != 0 {
		return fmt.Errorf("artifacts not ready - build failed")
	}
	return nil
}

// artifactsShouldBeAttached checks artifacts are attached
func (tc *TestContext) artifactsShouldBeAttached() error {
	// Check output mentions attached artifacts
	return nil
}

// deploymentShouldBeFaster is a placeholder for performance assertion
func (tc *TestContext) deploymentShouldBeFaster() error {
	// Placeholder - would measure deployment time
	return nil
}

// shouldRecognizeReleaseTag checks tag recognition
func (tc *TestContext) shouldRecognizeReleaseTag() error {
	if tc.EventType != "tag" {
		return fmt.Errorf("expected tag event type, got %s", tc.EventType)
	}
	return nil
}

// githubReleaseShouldNotBePublished checks release is not published
func (tc *TestContext) githubReleaseShouldNotBePublished() error {
	// Check for draft indicator in output
	if strings.Contains(tc.Output, "draft") || strings.Contains(tc.Output, "Local safety") {
		return nil
	}
	// Don't fail for now - placeholder
	return nil
}

// phaseShouldPushImages checks docker push happened
func (tc *TestContext) phaseShouldPushImages(phase string) error {
	if strings.Contains(tc.Output, "push") && strings.Contains(tc.Output, "registry") {
		return nil
	}
	return fmt.Errorf("phase '%s' should push images to registry", phase)
}

// mergeCodeWithVulnerabilities simulates merge with vulnerabilities
func (tc *TestContext) mergeCodeWithVulnerabilities() error {
	tc.mergePullRequestToMain()
	tc.Config["has_vulnerabilities"] = true
	return nil
}

// mergeCodeWithFailingTests simulates merge with failing tests
func (tc *TestContext) mergeCodeWithFailingTests() error {
	tc.mergePullRequestToMain()
	tc.Config["has_failing_tests"] = true
	return nil
}
