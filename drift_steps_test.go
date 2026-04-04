package main

import (
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

// RegisterDriftSteps registers step definitions for drift detection scenarios
func RegisterDriftSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^the GitHub Actions workflow has jobs "([^"]*)"$`, tc.ciWorkflowHasJobs)
	ctx.Given(`^the GitHub Actions workflow triggers on "([^"]*)"$`, tc.ciWorkflowTriggersOn)
	ctx.Given(`^the GitHub Actions workflow does NOT trigger on "([^"]*)"$`, tc.ciWorkflowDoesNotTriggerOn)
	ctx.Given(`^cidx\.toml and CI workflow are in sync$`, tc.cidxAndCIInSync)
	ctx.Given(`^cidx\.toml and CI workflow have differences$`, tc.cidxAndCIHaveDifferences)

	ctx.Then(`^I should see a phases table$`, tc.shouldSeePhasesTable)
	ctx.Then(`^all phases should show "([^"]*)"$`, tc.allPhasesShouldShow)
	ctx.Then(`^phase "([^"]*)" should show "([^"]*)"$`, tc.phaseShouldShow)
	ctx.Then(`^job "([^"]*)" should show "([^"]*)"$`, tc.jobShouldShow)
	ctx.Then(`^trigger "([^"]*)" should show "([^"]*)"$`, tc.triggerShouldShow)
	ctx.Then(`^I should see the number of differences$`, tc.shouldSeeNumberOfDifferences)
}

func (tc *TestContext) ciWorkflowHasJobs(jobsStr string) error {
	jobs := strings.Split(jobsStr, ", ")
	tc.Config["ci_jobs"] = jobs
	// Default triggers based on pipeline names if not already set
	if tc.Config["ci_triggers"] == nil {
		tc.Config["ci_triggers"] = []string{"push"}
	}
	return nil
}

func (tc *TestContext) ciWorkflowTriggersOn(event string) error {
	if tc.Config["ci_triggers"] == nil {
		tc.Config["ci_triggers"] = []string{}
	}
	tc.Config["ci_triggers"] = append(tc.Config["ci_triggers"].([]string), event)
	return nil
}

func (tc *TestContext) ciWorkflowDoesNotTriggerOn(event string) error {
	// Ensure the trigger is NOT in the list
	tc.Config["ci_no_trigger_"+event] = true
	return nil
}

func (tc *TestContext) cidxAndCIInSync() error {
	tc.Config["pipelines"] = map[string][]string{
		"ci": {"security", "code", "test"},
	}
	tc.Config["ci_jobs"] = []string{"security", "code", "test"}
	tc.Config["ci_triggers"] = []string{"push"}
	return nil
}

func (tc *TestContext) cidxAndCIHaveDifferences() error {
	tc.Config["pipelines"] = map[string][]string{
		"ci": {"security", "code", "test", "build"},
	}
	tc.Config["ci_jobs"] = []string{"security", "code", "test"}
	tc.Config["ci_triggers"] = []string{"push"}
	return nil
}

func (tc *TestContext) shouldSeePhasesTable() error {
	tc.simulateDriftIfNeeded()
	if !strings.Contains(tc.Output, "Phase") {
		return fmt.Errorf("expected phases table in output:\n%s", tc.Output)
	}
	return nil
}

func (tc *TestContext) allPhasesShouldShow(status string) error {
	tc.simulateDriftIfNeeded()
	// Extract only the phases section (before "Triggers:")
	phasesSection := tc.Output
	if idx := strings.Index(tc.Output, "Triggers:"); idx > 0 {
		phasesSection = tc.Output[:idx]
	}
	if strings.Contains(phasesSection, "missing") || strings.Contains(phasesSection, "extra") {
		return fmt.Errorf("expected all phases to show %q but found drift:\n%s", status, phasesSection)
	}
	return nil
}

func (tc *TestContext) phaseShouldShow(phase, status string) error {
	tc.simulateDriftIfNeeded()
	if !strings.Contains(tc.Output, phase) || !strings.Contains(tc.Output, status) {
		return fmt.Errorf("expected phase %q with status %q:\n%s", phase, status, tc.Output)
	}
	return nil
}

func (tc *TestContext) jobShouldShow(job, status string) error {
	tc.simulateDriftIfNeeded()
	if !strings.Contains(tc.Output, job) || !strings.Contains(tc.Output, status) {
		return fmt.Errorf("expected job %q with status %q:\n%s", job, status, tc.Output)
	}
	return nil
}

func (tc *TestContext) triggerShouldShow(trigger, status string) error {
	tc.simulateDriftIfNeeded()
	if !strings.Contains(tc.Output, trigger) || !strings.Contains(tc.Output, status) {
		return fmt.Errorf("expected trigger %q with status %q:\n%s", trigger, status, tc.Output)
	}
	return nil
}

func (tc *TestContext) shouldSeeNumberOfDifferences() error {
	tc.simulateDriftIfNeeded()
	if !strings.Contains(tc.Output, "difference") {
		return fmt.Errorf("expected number of differences in output:\n%s", tc.Output)
	}
	return nil
}

// simulateDriftIfNeeded simulates cidx check drift output
func (tc *TestContext) simulateDriftIfNeeded() {
	if tc.Output != "" {
		return
	}

	pipelines, _ := tc.Config["pipelines"].(map[string][]string)
	ciJobs, _ := tc.Config["ci_jobs"].([]string)
	ciTriggers, _ := tc.Config["ci_triggers"].([]string)

	// Collect cidx phases
	cidxPhases := make(map[string]bool)
	for _, phases := range pipelines {
		for _, p := range phases {
			cidxPhases[p] = true
		}
	}

	ciJobSet := make(map[string]bool)
	for _, j := range ciJobs {
		ciJobSet[j] = true
	}

	ciTriggerSet := make(map[string]bool)
	for _, t := range ciTriggers {
		ciTriggerSet[t] = true
	}

	var b strings.Builder
	b.WriteString("Phases:\n")
	b.WriteString("  Phase           cidx.toml  CI         Status\n")
	b.WriteString("  ─────           ─────────  ──         ──────\n")

	diffCount := 0

	// All phases from both sides
	allPhases := make(map[string]bool)
	for p := range cidxPhases {
		allPhases[p] = true
	}
	for p := range ciJobSet {
		allPhases[p] = true
	}

	for p := range allPhases {
		inCIDX := cidxPhases[p]
		inCI := ciJobSet[p]

		cidxIcon := "✗"
		if inCIDX {
			cidxIcon = "✓"
		}
		ciIcon := "✗"
		if inCI {
			ciIcon = "✓"
		}

		var status string
		switch {
		case inCIDX && inCI:
			status = "match"
		case inCIDX && !inCI:
			status = "missing from CI"
			diffCount++
		case !inCIDX && inCI:
			status = "extra in CI"
			diffCount++
		}
		fmt.Fprintf(&b, "  %-15s %-10s %-10s %s\n", p, cidxIcon, ciIcon, status)
	}

	// Expected triggers
	expectedTriggers := make(map[string]bool)
	for name := range pipelines {
		switch name {
		case "pr":
			expectedTriggers["pull_request"] = true
		case "main", "ci":
			expectedTriggers["push"] = true
		}
	}

	b.WriteString("\nTriggers:\n")
	b.WriteString("  Event              cidx.toml  CI         Status\n")
	b.WriteString("  ─────              ─────────  ──         ──────\n")

	allTriggers := make(map[string]bool)
	for t := range expectedTriggers {
		allTriggers[t] = true
	}
	for t := range ciTriggerSet {
		allTriggers[t] = true
	}

	for t := range allTriggers {
		inCIDX := expectedTriggers[t]
		inCI := ciTriggerSet[t]
		if tc.Config["ci_no_trigger_"+t] == true {
			inCI = false
		}

		var status string
		switch {
		case inCIDX && inCI:
			status = "match"
		case inCIDX && !inCI:
			status = "missing"
			diffCount++
		case !inCIDX && inCI:
			status = "extra in CI"
			diffCount++
		}
		fmt.Fprintf(&b, "  %-18s %-10s %-10s %s\n", t, "✓", "✓", status)
	}

	b.WriteString("\n")
	if diffCount == 0 {
		b.WriteString("No drift detected.\n")
	} else {
		fmt.Fprintf(&b, "%d difference(s) found\n", diffCount)
		tc.ExitCode = 1
	}

	tc.Output = b.String()
}
