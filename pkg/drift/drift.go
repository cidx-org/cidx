// Package drift compares cidx.toml declarations with actual CI platform configuration.
package drift

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"gopkg.in/yaml.v3"
)

// Status represents the drift status of an item.
type Status string

const (
	StatusMatch   Status = "match"
	StatusMissing Status = "missing from CI"
	StatusExtra   Status = "extra in CI"
)

// PhaseDiff represents a phase comparison result.
type PhaseDiff struct {
	Name   string
	CIDX   bool   // present in cidx.toml
	CI     bool   // present in CI workflow
	Status Status
}

// TriggerDiff represents a trigger comparison result.
type TriggerDiff struct {
	Event  string
	CIDX   bool   // expected from cidx.toml pipeline names
	CI     bool   // present in CI workflow
	Status Status
}

// Result holds the complete drift analysis.
type Result struct {
	Phases   []PhaseDiff
	Triggers []TriggerDiff
}

// HasDrift returns true if any differences were found.
func (r *Result) HasDrift() bool {
	for _, p := range r.Phases {
		if p.Status != StatusMatch {
			return true
		}
	}
	for _, t := range r.Triggers {
		if t.Status != StatusMatch {
			return true
		}
	}
	return false
}

// DiffCount returns the number of differences found.
func (r *Result) DiffCount() int {
	n := 0
	for _, p := range r.Phases {
		if p.Status != StatusMatch {
			n++
		}
	}
	for _, t := range r.Triggers {
		if t.Status != StatusMatch {
			n++
		}
	}
	return n
}

// Compare analyzes drift between cidx.toml config and a GitHub Actions workflow file.
func Compare(cfg *config.Config, workflowPath string) (*Result, error) {
	workflow, err := parseGitHubWorkflow(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow %s: %w", workflowPath, err)
	}

	result := &Result{}
	result.Phases = comparePhases(cfg, workflow)
	result.Triggers = compareTriggers(cfg, workflow)
	return result, nil
}

// CompareFromData analyzes drift using raw YAML data (for testing).
func CompareFromData(cfg *config.Config, workflowData []byte) (*Result, error) {
	workflow, err := parseGitHubWorkflowData(workflowData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow data: %w", err)
	}

	result := &Result{}
	result.Phases = comparePhases(cfg, workflow)
	result.Triggers = compareTriggers(cfg, workflow)
	return result, nil
}

// githubWorkflow is a minimal representation of a GitHub Actions workflow.
type githubWorkflow struct {
	On   workflowTriggers          `yaml:"on"`
	Jobs map[string]workflowJob    `yaml:"jobs"`
}

type workflowTriggers struct {
	Push        *triggerConfig `yaml:"push"`
	PullRequest *triggerConfig `yaml:"pull_request"`
}

type triggerConfig struct {
	Branches []string `yaml:"branches"`
	Tags     []string `yaml:"tags"`
}

type workflowJob struct {
	Name  string   `yaml:"name"`
	Needs []string `yaml:"needs"`
}

// Custom unmarshaler for workflowTriggers to handle both map and string forms.
func (t *workflowTriggers) UnmarshalYAML(value *yaml.Node) error {
	// Handle map form: on: { push: ..., pull_request: ... }
	if value.Kind == yaml.MappingNode {
		type raw workflowTriggers
		return value.Decode((*raw)(t))
	}
	return nil
}

func parseGitHubWorkflow(path string) (*githubWorkflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseGitHubWorkflowData(data)
}

func parseGitHubWorkflowData(data []byte) (*githubWorkflow, error) {
	var workflow githubWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// comparePhases compares cidx.toml phases with workflow jobs.
func comparePhases(cfg *config.Config, workflow *githubWorkflow) []PhaseDiff {
	// Collect all unique phases from cidx.toml pipelines
	cidxPhases := make(map[string]bool)
	for _, pipeline := range cfg.Pipelines {
		for _, phase := range pipeline.Phases {
			cidxPhases[phase] = true
		}
	}

	// Collect CI job names (exclude infrastructure jobs like bootstrap)
	ciJobs := make(map[string]bool)
	infraJobs := map[string]bool{"bootstrap": true}
	for name := range workflow.Jobs {
		if !infraJobs[name] {
			ciJobs[name] = true
		}
	}

	// Build diff list
	allNames := make(map[string]bool)
	for name := range cidxPhases {
		allNames[name] = true
	}
	for name := range ciJobs {
		allNames[name] = true
	}

	sorted := sortedKeys(allNames)
	var diffs []PhaseDiff

	for _, name := range sorted {
		inCIDX := cidxPhases[name]
		inCI := ciJobs[name]

		diff := PhaseDiff{
			Name: name,
			CIDX: inCIDX,
			CI:   inCI,
		}

		switch {
		case inCIDX && inCI:
			diff.Status = StatusMatch
		case inCIDX && !inCI:
			diff.Status = StatusMissing
		case !inCIDX && inCI:
			diff.Status = StatusExtra
		}

		diffs = append(diffs, diff)
	}

	return diffs
}

// compareTriggers compares expected triggers from pipeline names with actual workflow triggers.
func compareTriggers(cfg *config.Config, workflow *githubWorkflow) []TriggerDiff {
	// Determine expected triggers from pipeline names
	expectedTriggers := make(map[string]bool)
	for name := range cfg.Pipelines {
		switch name {
		case "pr":
			expectedTriggers["pull_request"] = true
		case "main", "ci":
			expectedTriggers["push"] = true
		case "release":
			expectedTriggers["push"] = true // tags are push events
		}
	}

	// Determine actual triggers
	actualTriggers := make(map[string]bool)
	if workflow.On.Push != nil {
		actualTriggers["push"] = true
	}
	if workflow.On.PullRequest != nil {
		actualTriggers["pull_request"] = true
	}

	// Build diff
	allTriggers := make(map[string]bool)
	for t := range expectedTriggers {
		allTriggers[t] = true
	}
	for t := range actualTriggers {
		allTriggers[t] = true
	}

	sorted := sortedKeys(allTriggers)
	var diffs []TriggerDiff

	for _, trigger := range sorted {
		inCIDX := expectedTriggers[trigger]
		inCI := actualTriggers[trigger]

		diff := TriggerDiff{
			Event: trigger,
			CIDX:  inCIDX,
			CI:    inCI,
		}

		switch {
		case inCIDX && inCI:
			diff.Status = StatusMatch
		case inCIDX && !inCI:
			diff.Status = "missing"
		case !inCIDX && inCI:
			diff.Status = "extra in CI"
		}

		diffs = append(diffs, diff)
	}

	return diffs
}

// Format renders the drift result as a human-readable table.
func Format(result *Result) string {
	var b strings.Builder

	// Phases table
	b.WriteString("Phases:\n")
	b.WriteString(fmt.Sprintf("  %-15s %-10s %-10s %s\n", "Phase", "cidx.toml", "CI", "Status"))
	b.WriteString(fmt.Sprintf("  %-15s %-10s %-10s %s\n", "─────", "─────────", "──", "──────"))

	for _, p := range result.Phases {
		cidx := icon(p.CIDX)
		ci := icon(p.CI)
		status := formatStatus(p.Status)
		b.WriteString(fmt.Sprintf("  %-15s %-10s %-10s %s\n", p.Name, cidx, ci, status))
	}

	// Triggers table
	b.WriteString("\nTriggers:\n")
	b.WriteString(fmt.Sprintf("  %-18s %-10s %-10s %s\n", "Event", "cidx.toml", "CI", "Status"))
	b.WriteString(fmt.Sprintf("  %-18s %-10s %-10s %s\n", "─────", "─────────", "──", "──────"))

	for _, t := range result.Triggers {
		cidx := icon(t.CIDX)
		ci := icon(t.CI)
		status := formatStatus(Status(t.Status))
		b.WriteString(fmt.Sprintf("  %-18s %-10s %-10s %s\n", t.Event, cidx, ci, status))
	}

	return b.String()
}

func icon(present bool) string {
	if present {
		return "✓"
	}
	return "✗"
}

func formatStatus(s Status) string {
	switch s {
	case StatusMatch:
		return "\033[32m✓ match\033[0m"
	case StatusMissing:
		return "\033[31m✗ missing from CI\033[0m"
	case StatusExtra:
		return "\033[33m⚠ extra in CI\033[0m"
	case "missing":
		return "\033[31m✗ missing\033[0m"
	default:
		return string(s)
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
