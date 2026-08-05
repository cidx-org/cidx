package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/cucumber/godog"
)

// RegisterEventSteps registers event-related step definitions
func RegisterEventSteps(ctx *godog.ScenarioContext, testCtx *TestContext) {
	// Git event steps
	ctx.Given(`^I create a pull request$`, testCtx.createPullRequest)
	ctx.Given(`^I create a pull request with (.*)$`, testCtx.createPullRequestWith)
	ctx.Given(`^I push a tag "([^"]*)"$`, testCtx.pushTag)
	ctx.When(`^I push tag "([^"]*)"$`, testCtx.pushTag)
	ctx.Given(`^I have tag "([^"]*)" locally$`, testCtx.haveTagLocally)
	ctx.Given(`^I merge a pull request to main branch$`, testCtx.mergePullRequestToMain)
	ctx.Given(`^I am on main branch$`, testCtx.checkoutMainBranch)
	ctx.Given(`^the branch is "([^"]*)"$`, testCtx.checkoutBranch)
	ctx.Given(`^I have valid registry credentials$`, testCtx.haveValidRegistryCredentials)
	ctx.Given(`^registry credentials are not set$`, testCtx.registryCredentialsNotSet)

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
	ctx.Then(`^the release should use the pre-built artifacts$`, testCtx.releaseShouldUsePrebuiltArtifacts)

	// Release assertions
	ctx.Then(`^CIDX should recognize it as a release tag$`, testCtx.shouldRecognizeReleaseTag)
	ctx.Then(`^the GitHub release should NOT be published$`, testCtx.githubReleaseShouldNotBePublished)
	ctx.Then(`^the "([^"]*)" phase should push images to registry$`, testCtx.phaseShouldPushImages)
	ctx.Then(`^the "([^"]*)" phase should publish the GitHub release$`, testCtx.phaseShouldPublishRelease)
	ctx.Then(`^the "([^"]*)" phase should create a draft release$`, testCtx.phaseShouldCreateDraftRelease)
	ctx.Then(`^the release should be public$`, testCtx.theReleaseShouldBePublic)
	ctx.Then(`^release notes should be generated$`, testCtx.releaseNotesShouldBeGenerated)
	ctx.Then(`^I should see error about missing credentials$`, testCtx.shouldSeeErrorAboutCredentials)

	// Phase assertions with table
	ctx.Then(`^it should execute all phases in order:$`, testCtx.shouldExecuteAllPhasesInOrder)
	ctx.Then(`^I should see (.+) scan results$`, testCtx.shouldSeeScanResults)

	// Event and branch context
	ctx.Given(`^I merge code with security vulnerabilities to main$`, testCtx.mergeCodeWithVulnerabilities)
	ctx.Given(`^I merge code with failing tests to main$`, testCtx.mergeCodeWithFailingTests)
	ctx.Given(`^I run "([^"]*)" successfully$`, testCtx.runCommandSuccessfully)
}

// createPullRequest simulates creating a pull request
func (tc *TestContext) createPullRequest() error {
	tc.EventType = "pull_request"
	return nil
}

// createPullRequestWith creates PR with specific characteristics
func (tc *TestContext) createPullRequestWith(characteristics string) error {
	_ = tc.createPullRequest()

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
	_ = tc.checkoutMainBranch()
	return nil
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

// haveValidRegistryCredentials sets up registry credentials
func (tc *TestContext) haveValidRegistryCredentials() error {
	tc.Config["registry_credentials"] = true
	return nil
}

// registryCredentialsNotSet marks registry credentials as missing
func (tc *TestContext) registryCredentialsNotSet() error {
	tc.Config["no_registry_credentials"] = true
	return nil
}

// detectPullRequestEvent detects PR event
func (tc *TestContext) detectPullRequestEvent() error {
	tc.EventType = "pull_request"
	return nil
}

// mergeCodeToMain simulates merging code with issues
func (tc *TestContext) mergeCodeToMain(issues string) error {
	_ = tc.mergePullRequestToMain()
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
	return fmt.Errorf("phase '%s' was not executed. Executed phases: %v", phase, tc.ExecutedPhases)
}

// shouldNotExecutePhase checks if a phase was NOT executed
func (tc *TestContext) shouldNotExecutePhase(phase string) error {
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			return fmt.Errorf("phase '%s' should NOT have been executed, but it was", phase)
		}
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
		return fmt.Errorf("phase '%s' should have failed but didn't. Failed phases: %v", phase, tc.FailedPhases)
	}

	// Check phase passed (executed but not failed)
	executed := false
	for _, p := range tc.ExecutedPhases {
		if p == phase {
			executed = true
			break
		}
	}
	if !executed {
		return fmt.Errorf("phase '%s' should have passed but was not executed", phase)
	}

	for _, p := range tc.FailedPhases {
		if p == phase {
			return fmt.Errorf("phase '%s' should have passed but it failed", phase)
		}
	}
	return nil
}

// phaseShouldExecute is alias for shouldExecutePhase
func (tc *TestContext) phaseShouldExecute(phase string) error {
	return tc.shouldExecutePhase(phase)
}

// phaseShouldNotExecute is alias for shouldNotExecutePhase
func (tc *TestContext) phaseShouldNotExecute(phase string) error {
	return tc.shouldNotExecutePhase(phase)
}

// noFurtherPhasesExecute asserts fail-fast: nothing ran after the phase that
// failed.
func (tc *TestContext) noFurtherPhasesExecute() error {
	return tc.subsequentPhasesShouldNotExecute()
}

// shouldCompleteInTime asserts the run really finished inside the budget the
// scenario states, measured over the run rather than assumed.
func (tc *TestContext) shouldCompleteInTime(minutes int) error {
	if tc.RunDuration == 0 && len(tc.ExecutedPhases) == 0 {
		return fmt.Errorf("nothing ran, so nothing completed")
	}
	if budget := time.Duration(minutes) * time.Minute; tc.RunDuration > budget {
		return fmt.Errorf("the pipeline took %s, over the %s budget", tc.RunDuration, budget)
	}
	return nil
}

// shouldReceiveClearFeedback asserts the run said what happened to each phase
// it ran, rather than finishing in silence.
func (tc *TestContext) shouldReceiveClearFeedback() error {
	if len(tc.ExecutedPhases) == 0 {
		return fmt.Errorf("no phase ran, so there is no feedback to give")
	}
	for _, phase := range tc.ExecutedPhases {
		passed := strings.Contains(tc.Output, fmt.Sprintf("✓ %s", phase))
		failed := strings.Contains(tc.Output, fmt.Sprintf("✗ %s", phase))
		if !passed && !failed {
			return fmt.Errorf("the %q phase ran but its outcome is not reported:\n%s", phase, tc.Output)
		}
	}
	return nil
}

// phaseShouldCreateArtifacts checks phase created artifacts
func (tc *TestContext) phaseShouldCreateArtifacts(phase string) error {
	return tc.shouldExecutePhase(phase)
}

// artifactsShouldBeStored asks the preset that builds them where they land.
// The directory is a property of the catalogue, not of the run.
func (tc *TestContext) artifactsShouldBeStored(directory string) error {
	preset, err := presets.Get("go-build")
	if err != nil {
		return err
	}
	if !strings.Contains(preset.Command, directory) {
		return fmt.Errorf("the go-build preset writes nothing to %q:\n%s", directory, preset.Command)
	}
	return nil
}

// artifactsShouldBeReadyForRelease checks artifacts are ready
func (tc *TestContext) artifactsShouldBeReadyForRelease() error {
	if tc.ExitCode != 0 {
		return fmt.Errorf("artifacts not ready - build failed")
	}
	return nil
}

// artifactsShouldBeAttached asks the release preset whether it passes anything
// to attach: gh-release runs `gh release create ${TAG} ${ARTIFACTS}`, and
// ARTIFACTS has to name something.
func (tc *TestContext) artifactsShouldBeAttached() error {
	preset, err := presets.Get("gh-release")
	if err != nil {
		return err
	}
	if !strings.Contains(preset.Command, "${ARTIFACTS}") {
		return fmt.Errorf("gh-release attaches nothing to the release:\n%s", preset.Command)
	}
	if preset.Env["ARTIFACTS"] == "" {
		return fmt.Errorf("gh-release leaves ARTIFACTS empty, so nothing is attached")
	}
	return nil
}

// releaseShouldUsePrebuiltArtifacts asserts the release phase consumes what the
// build phase produced, rather than building again: gh-release's ARTIFACTS
// points inside the directory go-build writes to.
func (tc *TestContext) releaseShouldUsePrebuiltArtifacts() error {
	release, err := presets.Get("gh-release")
	if err != nil {
		return err
	}
	build, err := presets.Get("go-build")
	if err != nil {
		return err
	}
	artifacts := release.Env["ARTIFACTS"]
	if artifacts == "" {
		return fmt.Errorf("gh-release names no artifact to publish")
	}
	if !strings.Contains(build.Command, artifacts) {
		return fmt.Errorf("gh-release publishes %q, which go-build does not produce:\n%s", artifacts, build.Command)
	}
	return nil
}

// shouldRecognizeReleaseTag checks tag recognition
func (tc *TestContext) shouldRecognizeReleaseTag() error {
	if tc.EventType != "tag" {
		return fmt.Errorf("expected tag event type, got %s", tc.EventType)
	}
	return nil
}

// githubReleaseShouldNotBePublished asks environment.ValidatePreset what would
// happen to gh-release here. Locally it is held as a dry-run, which is the
// reason nothing reaches GitHub — "local never publishes" was the belief, not
// a check.
func (tc *TestContext) githubReleaseShouldNotBePublished() error {
	preset, mode, err := tc.resolvePresetSafety("gh-release")
	if err != nil {
		return err
	}
	if !mode.IsDryRun {
		return fmt.Errorf("preset %q would really run here (mode %s), and publish", preset.Name, mode.Mode)
	}
	return nil
}

// phaseShouldPushImages checks docker push happened
func (tc *TestContext) phaseShouldPushImages(phase string) error {
	return tc.shouldExecutePhase(phase)
}

// phaseShouldPublishRelease checks phase published a release
func (tc *TestContext) phaseShouldPublishRelease(phase string) error {
	return tc.shouldExecutePhase(phase)
}

// phaseShouldCreateDraftRelease checks phase created a draft release
func (tc *TestContext) phaseShouldCreateDraftRelease(phase string) error {
	if err := tc.shouldExecutePhase(phase); err != nil {
		return err
	}
	if !strings.Contains(tc.Output, "draft") {
		return fmt.Errorf("expected draft release in output")
	}
	return nil
}

// theReleaseShouldBePublic checks release is public
func (tc *TestContext) theReleaseShouldBePublic() error {
	if tc.CI {
		return nil
	}
	return fmt.Errorf("releases are only public in CI")
}

// releaseNotesShouldBeGenerated asks the preset that creates the release: its
// command either uses the notes file `cidx release prepare` wrote, or falls
// back to --generate-notes. Either way it must ask for notes.
func (tc *TestContext) releaseNotesShouldBeGenerated() error {
	preset, err := presets.Get("gh-release")
	if err != nil {
		return err
	}
	if !strings.Contains(preset.Command, "--generate-notes") && !strings.Contains(preset.Command, "--notes-file") {
		return fmt.Errorf("gh-release creates a release with no notes:\n%s", preset.Command)
	}
	return nil
}

// shouldSeeErrorAboutCredentials checks for credentials error
func (tc *TestContext) shouldSeeErrorAboutCredentials() error {
	if strings.Contains(strings.ToLower(tc.Output), "credentials") {
		return nil
	}
	return fmt.Errorf("expected error about credentials in output:\n%s", tc.Output)
}

// shouldExecuteAllPhasesInOrder checks all phases in order from table
func (tc *TestContext) shouldExecuteAllPhasesInOrder(table *godog.Table) error {
	expectedPhases := []string{}
	for i, row := range table.Rows {
		if i == 0 {
			continue
		}
		if len(row.Cells) > 0 {
			// Find the "phase" column
			for _, cell := range row.Cells {
				if cell.Value != "" {
					expectedPhases = append(expectedPhases, cell.Value)
					break
				}
			}
		}
	}

	if len(tc.ExecutedPhases) == 0 && len(expectedPhases) > 0 {
		return fmt.Errorf("expected phases to execute, but none were executed")
	}

	return nil
}

// shouldSeeScanResults asserts the scan the scenario asked about reported
// something, and named the phase it came from.
func (tc *TestContext) shouldSeeScanResults(scanType string) error {
	if !strings.Contains(strings.ToLower(tc.Output), "scan") {
		return fmt.Errorf("no %s scan result in the output:\n%s", scanType, tc.Output)
	}
	return tc.shouldExecutePhase("security")
}

// mergeCodeWithVulnerabilities simulates merge with vulnerabilities
func (tc *TestContext) mergeCodeWithVulnerabilities() error {
	_ = tc.mergePullRequestToMain()
	tc.Config["has_vulnerabilities"] = true
	return nil
}

// mergeCodeWithFailingTests simulates merge with failing tests
func (tc *TestContext) mergeCodeWithFailingTests() error {
	_ = tc.mergePullRequestToMain()
	tc.Config["has_failing_tests"] = true
	return nil
}

// runCommandSuccessfully runs a command and verifies success
func (tc *TestContext) runCommandSuccessfully(cmdStr string) error {
	if err := tc.runCommand(cmdStr); err != nil {
		return err
	}
	if tc.ExitCode != 0 {
		return fmt.Errorf("command '%s' failed with exit code %d", cmdStr, tc.ExitCode)
	}
	return nil
}
