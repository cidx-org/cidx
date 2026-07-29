package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/doctor"
	"github.com/cucumber/godog"
)

// RegisterDoctorSteps registers step definitions for doctor scenarios.
//
// The checks run for real (doctor.Run) against the directory the scenario
// staged: its Git repository, its cidx.toml, and the container runtime this
// machine actually has — the runtime steps live in executor_steps_test.go and
// already probe it (issue #265).
func RegisterDoctorSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Then(`^I should see a passing check for "([^"]*)"$`, tc.shouldSeePassingCheck)
	ctx.Then(`^I should see a failing check for "([^"]*)"$`, tc.shouldSeeFailingCheck)
	ctx.Then(`^I should see a warning check for "([^"]*)"$`, tc.shouldSeeWarningCheck)
	ctx.Then(`^the check should show the Docker version$`, tc.checkShouldShowDockerVersion)
	ctx.Then(`^I should see a suggestion to install Docker or Podman$`, tc.shouldSeeSuggestionInstallDocker)
	ctx.Then(`^I should see a suggestion to run "([^"]*)"$`, tc.shouldSeeSuggestionToRun)
	ctx.Then(`^I should see the number of issues found$`, tc.shouldSeeIssueCount)

	ctx.Given(`^a valid "([^"]*)" exists$`, tc.aValidConfigExists)
	ctx.Given(`^no "([^"]*)" exists$`, tc.noConfigExists)
	ctx.Given(`^I am NOT in a Git repository$`, tc.notInGitRepo)
}

// runDoctor runs the real environment checks from the scenario's project
// directory and keeps both what `cidx doctor` prints and the checks behind it.
func (tc *TestContext) runDoctor() error {
	return tc.inScenarioDir(func() error {
		if tc.Config["no_config"] != true {
			if _, err := tc.writeStagedConfig(); err != nil {
				return err
			}
		}

		result := doctor.Run()
		tc.Config["doctor_result"] = result
		tc.Output = doctor.Format(result) + "\n" + doctor.Summary(result) + "\n"
		if result.Issues() > 0 {
			tc.ExitCode = 1
		}
		return nil
	})
}

func (tc *TestContext) doctorCheck(name string) (doctor.Check, error) {
	result, ok := tc.Config["doctor_result"].(*doctor.Result)
	if !ok {
		return doctor.Check{}, fmt.Errorf("cidx doctor did not run in this scenario")
	}
	var names []string
	for _, check := range result.Checks {
		if check.Name == name {
			return check, nil
		}
		names = append(names, check.Name)
	}
	return doctor.Check{}, fmt.Errorf("no check named %q (checks: %s)", name, strings.Join(names, ", "))
}

// checkWithStatus asserts a check reached the given status and that the user
// can see it — the status is the diagnosis, the output is what is delivered.
func (tc *TestContext) checkWithStatus(name string, want doctor.Status) (doctor.Check, error) {
	check, err := tc.doctorCheck(name)
	if err != nil {
		return check, err
	}
	if check.Status != want {
		return check, fmt.Errorf("check %q is %s, want %s (detail: %s)",
			name, statusName(check.Status), statusName(want), check.Detail)
	}
	if !strings.Contains(tc.Output, name) || !strings.Contains(tc.Output, statusIcon(want)) {
		return check, fmt.Errorf("check %q is not reported as %s in the output:\n%s",
			name, statusName(want), tc.Output)
	}
	return check, nil
}

func (tc *TestContext) shouldSeePassingCheck(name string) error {
	_, err := tc.checkWithStatus(name, doctor.StatusPass)
	return err
}

func (tc *TestContext) shouldSeeFailingCheck(name string) error {
	_, err := tc.checkWithStatus(name, doctor.StatusFail)
	return err
}

func (tc *TestContext) shouldSeeWarningCheck(name string) error {
	_, err := tc.checkWithStatus(name, doctor.StatusWarn)
	return err
}

// runtimeVersion is the version number the runtime check reports next to the
// engine name ("Docker 29.6.1").
var runtimeVersion = regexp.MustCompile(`\d+\.\d+`)

func (tc *TestContext) checkShouldShowDockerVersion() error {
	check, err := tc.doctorCheck("Container runtime")
	if err != nil {
		return err
	}
	if !strings.Contains(check.Detail, "Docker") || !runtimeVersion.MatchString(check.Detail) {
		return fmt.Errorf("expected a Docker version in the runtime detail, got %q", check.Detail)
	}
	if !strings.Contains(tc.Output, check.Detail) {
		return fmt.Errorf("the runtime detail %q is missing from the output:\n%s", check.Detail, tc.Output)
	}
	return nil
}

func (tc *TestContext) shouldSeeSuggestionInstallDocker() error {
	check, err := tc.doctorCheck("Container runtime")
	if err != nil {
		return err
	}
	if !strings.Contains(check.Suggestion, "Docker") || !strings.Contains(check.Suggestion, "Podman") {
		return fmt.Errorf("expected a suggestion naming Docker and Podman, got %q", check.Suggestion)
	}
	if !strings.Contains(tc.Output, check.Suggestion) {
		return fmt.Errorf("the suggestion %q is missing from the output:\n%s", check.Suggestion, tc.Output)
	}
	return nil
}

// shouldSeeSuggestionToRun looks for a command a failing or warning check tells
// the user to run.
func (tc *TestContext) shouldSeeSuggestionToRun(command string) error {
	result, ok := tc.Config["doctor_result"].(*doctor.Result)
	if !ok {
		return fmt.Errorf("cidx doctor did not run in this scenario")
	}
	for _, check := range result.Checks {
		if strings.Contains(check.Suggestion, command) {
			if !strings.Contains(tc.Output, check.Suggestion) {
				return fmt.Errorf("the suggestion %q is missing from the output:\n%s", check.Suggestion, tc.Output)
			}
			return nil
		}
	}
	return fmt.Errorf("no check suggests running %q", command)
}

func (tc *TestContext) shouldSeeIssueCount() error {
	result, ok := tc.Config["doctor_result"].(*doctor.Result)
	if !ok {
		return fmt.Errorf("cidx doctor did not run in this scenario")
	}
	if result.Issues() == 0 {
		return fmt.Errorf("expected issues to be counted, doctor found none")
	}
	expected := fmt.Sprintf("%d issue(s) found", result.Issues())
	if !strings.Contains(tc.Output, expected) {
		return fmt.Errorf("expected %q in the output:\n%s", expected, tc.Output)
	}
	return nil
}

// aValidConfigExists stages a real, loadable cidx.toml in the scenario's
// project directory. The commands write it when they run, so the pipelines a
// scenario declares afterwards are part of it.
func (tc *TestContext) aValidConfigExists(filename string) error {
	tc.Config["config_file"] = filename
	_, err := tc.scenarioDir()
	return err
}

// noConfigExists leaves the scenario's project directory without a cidx.toml,
// which is what config.FindConfig then fails to find.
func (tc *TestContext) noConfigExists(string) error {
	tc.Config["no_config"] = true
	_, err := tc.scenarioDir()
	return err
}

// notInGitRepo leaves the scenario in a plain temporary directory: no .git of
// its own and none above it, so `git rev-parse` fails the way the scenario says.
func (tc *TestContext) notInGitRepo() error {
	_, err := tc.scenarioDir()
	return err
}

func statusName(status doctor.Status) string {
	switch status {
	case doctor.StatusPass:
		return "passing"
	case doctor.StatusWarn:
		return "a warning"
	default:
		return "failing"
	}
}

func statusIcon(status doctor.Status) string {
	switch status {
	case doctor.StatusPass:
		return "✓"
	case doctor.StatusWarn:
		return "⚠"
	default:
		return "✗"
	}
}
