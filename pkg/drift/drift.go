// Package drift compares cidx.toml declarations with actual CI platform configuration.
package drift

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/config"
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
	CIDX   bool // present in cidx.toml
	CI     bool // present in CI workflow
	Status Status
}

// TriggerDiff represents a trigger comparison result.
type TriggerDiff struct {
	Event  string
	CIDX   bool // expected from cidx.toml pipeline names
	CI     bool // present in CI workflow
	Status Status
}

// Result holds the complete drift analysis.
type Result struct {
	Workflow string // workflow file the config was compared against
	Pipeline string // pipeline the workflow implements ("" = every pipeline)
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
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow %s: %w", workflowPath, err)
	}
	return CompareFromData(cfg, workflowPath, data)
}

// CompareFromData analyzes drift using raw YAML data. workflowPath is not read;
// it names the workflow so the comparison can be scoped to the pipeline that
// workflow implements (see pipelineFor).
func CompareFromData(cfg *config.Config, workflowPath string, workflowData []byte) (*Result, error) {
	workflow, err := parseGitHubWorkflowData(workflowData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow %s: %w", workflowPath, err)
	}

	pipeline := pipelineFor(cfg, workflowPath)
	return &Result{
		Workflow: workflowPath,
		Pipeline: pipeline,
		Phases:   comparePhases(expectedPhases(cfg, pipeline), workflow),
		Triggers: compareTriggers(cfg, workflow),
	}, nil
}

// pipelineFor returns the name of the pipeline a workflow file implements, or
// "" when it implements every declared pipeline.
//
// The mapping is the workflow's basename: ci.yml ↔ [pipelines.ci], release.yml
// ↔ [pipelines.release] — the same file↔pipeline convention
// validator.ValidateAllWorkflows uses. A basename that names no declared
// pipeline covers them all: `cidx generate github` writes a single cidx.yml
// holding one job per phase of every pipeline (see generate.collectPhases), so
// that file legitimately implements all of them.
func pipelineFor(cfg *config.Config, workflowPath string) string {
	base := filepath.Base(workflowPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if _, ok := cfg.Pipelines[name]; ok {
		return name
	}
	return ""
}

// expectedPhases returns the phases the workflow is expected to run as jobs:
// those of the named pipeline, or of every pipeline when pipeline is "".
//
// Scoping this to one pipeline is what keeps a repo with a separate release
// workflow drift-clean: phases declared only by [pipelines.release] run in
// release.yml and are not supposed to appear in the CI workflow (issue #178).
func expectedPhases(cfg *config.Config, pipeline string) map[string]bool {
	phases := make(map[string]bool)
	if pipeline != "" {
		for _, phase := range cfg.Pipelines[pipeline].Phases {
			phases[phase] = true
		}
		return phases
	}
	for _, p := range cfg.Pipelines {
		for _, phase := range p.Phases {
			phases[phase] = true
		}
	}
	return phases
}

// githubWorkflow is a minimal representation of a GitHub Actions workflow.
type githubWorkflow struct {
	On   workflowTriggers       `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
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

func parseGitHubWorkflowData(data []byte) (*githubWorkflow, error) {
	var workflow githubWorkflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, err
	}
	return &workflow, nil
}

// comparePhases compares the phases expected from cidx.toml with workflow jobs.
func comparePhases(cidxPhases map[string]bool, workflow *githubWorkflow) []PhaseDiff {
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

	// State what was compared against what — the comparison is scoped to one
	// pipeline, so the user has to be able to see which one.
	if result.Workflow != "" {
		scope := "all pipelines"
		if result.Pipeline != "" {
			scope = fmt.Sprintf("pipeline '%s'", result.Pipeline)
		}
		fmt.Fprintf(&b, "Comparing %s with %s\n\n", scope, result.Workflow)
	}

	// Phases table
	b.WriteString("Phases:\n")
	fmt.Fprintf(&b, "  %-15s %-10s %-10s %s\n", "Phase", "cidx.toml", "CI", "Status")
	fmt.Fprintf(&b, "  %-15s %-10s %-10s %s\n", "─────", "─────────", "──", "──────")

	for _, p := range result.Phases {
		cidx := icon(p.CIDX)
		ci := icon(p.CI)
		status := formatStatus(p.Status)
		fmt.Fprintf(&b, "  %-15s %-10s %-10s %s\n", p.Name, cidx, ci, status)
	}

	// Triggers table
	b.WriteString("\nTriggers:\n")
	fmt.Fprintf(&b, "  %-18s %-10s %-10s %s\n", "Event", "cidx.toml", "CI", "Status")
	fmt.Fprintf(&b, "  %-18s %-10s %-10s %s\n", "─────", "─────────", "──", "──────")

	for _, t := range result.Triggers {
		cidx := icon(t.CIDX)
		ci := icon(t.CI)
		status := formatStatus(Status(t.Status))
		fmt.Fprintf(&b, "  %-18s %-10s %-10s %s\n", t.Event, cidx, ci, status)
	}

	return b.String()
}

// Summary is the verdict `cidx check drift` closes with: the reassurance when
// the workflow matches the config, or the count that becomes the command's
// error otherwise. The error branch carries no colour of its own — the CLI
// renders errors — which is why only the clean verdict is styled here.
func Summary(result *Result) string {
	if !result.HasDrift() {
		return "\033[32mNo drift detected.\033[0m"
	}
	return fmt.Sprintf("%d difference(s) found", result.DiffCount())
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
