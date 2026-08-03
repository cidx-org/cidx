package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

// WorkflowDefinition represents a GitHub Actions workflow
type WorkflowDefinition struct {
	Name   string         // Workflow name (e.g., "ci", "release")
	File   string         // Workflow file path
	Jobs   map[string]Job // Jobs defined in the workflow
	Phases []string       // Extracted phases from "cidx run <phase>" commands
}

// Job represents a GitHub Actions job
type Job struct {
	Name  string   // Job name
	Needs []string // Dependencies (needs: [job1, job2])
	Steps []Step   // Steps in the job
}

// Step represents a GitHub Actions step
type Step struct {
	Name string // Step name
	Run  string // Command to run
}

// ValidationResult contains the comparison result between a pipeline and workflow
type ValidationResult struct {
	Pipeline       string   // Pipeline name (e.g., "ci")
	WorkflowFile   string   // Workflow file path
	Success        bool     // Whether validation passed
	MissingInGH    []string // Phases in cidx.toml but not in GitHub workflow
	MissingInLocal []string // Phases in GitHub workflow but not in cidx.toml
	OrderMismatch  bool     // Whether phase order differs
	LocalOrder     []string // Order in cidx.toml
	GitHubOrder    []string // Order in GitHub workflow
}

// WorkflowYAML represents the structure of a GitHub Actions workflow file
type WorkflowYAML struct {
	Name string                     `yaml:"name"`
	Jobs map[string]WorkflowJobYAML `yaml:"jobs"`
}

// WorkflowJobYAML represents a job in the workflow YAML
type WorkflowJobYAML struct {
	Name  string             `yaml:"name"`
	Needs interface{}        `yaml:"needs"` // Can be string or []string
	Steps []WorkflowStepYAML `yaml:"steps"`
}

// WorkflowStepYAML represents a step in a job
type WorkflowStepYAML struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// ParseWorkflow parses a GitHub Actions workflow file and extracts phase
// information. The command tree comes from the running app, so the phases are
// read off the same command line the CLI would parse (issue #233).
func ParseWorkflow(app *cli.App, workflowPath string) (*WorkflowDefinition, error) {
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var wf WorkflowYAML
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	// Extract workflow name from filename (e.g., "ci.yml" → "ci")
	workflowName := strings.TrimSuffix(filepath.Base(workflowPath), filepath.Ext(workflowPath))

	jobs := make(map[string]Job)
	jobPhases := make(map[string][]string) // jobID -> phases it runs, in order

	for jobID, jobYAML := range wf.Jobs {
		// Parse needs field (can be string or []string)
		var needs []string
		switch v := jobYAML.Needs.(type) {
		case string:
			needs = []string{v}
		case []interface{}:
			for _, n := range v {
				if nStr, ok := n.(string); ok {
					needs = append(needs, nStr)
				}
			}
		case []string:
			needs = v
		}

		// Parse steps
		steps := make([]Step, 0, len(jobYAML.Steps))
		for _, stepYAML := range jobYAML.Steps {
			steps = append(steps, Step(stepYAML))

			// Extract the phases from the `cidx run <phase>` invocations of the
			// step. This used to match the substring "cidx run ", which any flag
			// between the binary and the subcommand defeated — ci.yml runs
			// `./bin/cidx --verbose run test` and the phase went missing (#233).
			// Every invocation counts: a job that runs two phases used to report
			// only the last one, losing the other exactly the same way.
			for _, inv := range ExtractInvocations(stepYAML.Run) {
				if target := RunTarget(app, inv.Args); target != "" {
					jobPhases[jobID] = append(jobPhases[jobID], target)
				}
			}
		}

		jobs[jobID] = Job{
			Name:  jobYAML.Name,
			Needs: needs,
			Steps: steps,
		}
	}

	// Perform topological sort to get execution order
	phases := topologicalSort(jobs, jobPhases, jobOrder(data))

	return &WorkflowDefinition{
		Name:   workflowName,
		File:   workflowPath,
		Jobs:   jobs,
		Phases: phases,
	}, nil
}

// ValidateWorkflow compares a pipeline definition with a GitHub Actions workflow
func ValidateWorkflow(app *cli.App, cfg *config.Config, pipelineName string, workflowPath string) (*ValidationResult, error) {
	// Get pipeline from config
	pipeline, exists := cfg.Pipelines[pipelineName]
	if !exists {
		return nil, fmt.Errorf("pipeline '%s' not found in configuration", pipelineName)
	}

	// Parse GitHub workflow
	workflow, err := ParseWorkflow(app, workflowPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow: %w", err)
	}

	// Compare phases
	localPhases := pipeline.Phases
	ghPhases := workflow.Phases

	// Find missing phases
	missingInGH := difference(localPhases, ghPhases)
	missingInLocal := difference(ghPhases, localPhases)

	// Check order mismatch (only if both have same phases)
	orderMismatch := false
	if len(missingInGH) == 0 && len(missingInLocal) == 0 {
		orderMismatch = !equalOrder(localPhases, ghPhases)
	}

	success := len(missingInGH) == 0 && len(missingInLocal) == 0 && !orderMismatch

	return &ValidationResult{
		Pipeline:       pipelineName,
		WorkflowFile:   workflowPath,
		Success:        success,
		MissingInGH:    missingInGH,
		MissingInLocal: missingInLocal,
		OrderMismatch:  orderMismatch,
		LocalOrder:     localPhases,
		GitHubOrder:    ghPhases,
	}, nil
}

// WorkflowPath returns the path of the workflow file that implements the named
// pipeline, or "" when the pipeline is not implemented by a workflow — either
// because it declares `workflow = "none"`, or because the file it points at
// does not exist.
//
// This is the single place the pipeline ↔ workflow pairing is decided, so
// `check workflow <pipeline>` and `check workflow` cannot answer differently.
func WorkflowPath(cfg *config.Config, pipelineName, workflowDir string) string {
	pipeline, exists := cfg.Pipelines[pipelineName]
	if !exists {
		return ""
	}

	file := pipeline.WorkflowFile(pipelineName)
	if file == "" {
		return ""
	}

	path := filepath.Join(workflowDir, file)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// ValidateAllWorkflows validates every pipeline that a workflow implements.
//
// A pipeline with no workflow is skipped rather than compared against a file
// that merely shares its name: `release.yml` publishes a release natively and
// delegates a single phase to cidx, so it never was `[pipelines.release]`'s
// mirror (issue #233). Which pipelines have a workflow is read from the config
// — see config.Pipeline.WorkflowFile.
func ValidateAllWorkflows(app *cli.App, cfg *config.Config, workflowDir string) ([]*ValidationResult, error) {
	results := []*ValidationResult{}

	// Sorted, because a Go map would report the same repository in a different
	// order on every run (#233).
	names := make([]string, 0, len(cfg.Pipelines))
	for name := range cfg.Pipelines {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, pipelineName := range names {
		workflowPath := WorkflowPath(cfg, pipelineName, workflowDir)
		if workflowPath == "" {
			continue
		}

		result, err := ValidateWorkflow(app, cfg, pipelineName, workflowPath)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, nil
}

// difference returns elements in a that are not in b
func difference(a, b []string) []string {
	bMap := make(map[string]bool)
	for _, item := range b {
		bMap[item] = true
	}

	diff := []string{}
	for _, item := range a {
		if !bMap[item] {
			diff = append(diff, item)
		}
	}
	return diff
}

// equalOrder checks if two slices have the same elements in the same order
func equalOrder(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// jobOrder returns the job IDs in the order the YAML document declares them.
// The parsed jobs live in a Go map, whose iteration order is randomised, so
// the declaration order is the only stable tie-break between jobs that no
// dependency separates (issue #233).
func jobOrder(data []byte) []string {
	var doc struct {
		Jobs yaml.Node `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil
	}

	order := make([]string, 0, len(doc.Jobs.Content)/2)
	for i := 0; i+1 < len(doc.Jobs.Content); i += 2 {
		order = append(order, doc.Jobs.Content[i].Value)
	}
	return order
}

// topologicalSort performs a topological sort of jobs based on dependencies
// and returns the phases in execution order. Jobs are visited in declaration
// order, so jobs that run in parallel keep the order the workflow lists them
// in and the result is the same on every run (issue #233).
func topologicalSort(jobs map[string]Job, jobPhases map[string][]string, order []string) []string {
	// Fall back to a sorted job list rather than a randomised map iteration
	// when the declaration order is unavailable.
	if len(order) != len(jobs) {
		order = make([]string, 0, len(jobs))
		for jobID := range jobs {
			order = append(order, jobID)
		}
		sort.Strings(order)
	}

	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	// Initialize all jobs with in-degree 0
	for _, jobID := range order {
		inDegree[jobID] = 0
		graph[jobID] = []string{}
	}

	// Build graph and calculate in-degrees
	for _, jobID := range order {
		for _, dep := range jobs[jobID].Needs {
			graph[dep] = append(graph[dep], jobID)
			inDegree[jobID]++
		}
	}

	// Find all jobs with in-degree 0 (no dependencies)
	queue := []string{}
	for _, jobID := range order {
		if inDegree[jobID] == 0 {
			queue = append(queue, jobID)
		}
	}

	// Process jobs in topological order
	sortedJobs := []string{}
	for len(queue) > 0 {
		// Get next job
		jobID := queue[0]
		queue = queue[1:]
		sortedJobs = append(sortedJobs, jobID)

		// Reduce in-degree for dependent jobs
		for _, neighbor := range graph[jobID] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Extract phases in execution order, removing duplicates
	phases := []string{}
	seenPhases := make(map[string]bool)
	for _, jobID := range sortedJobs {
		for _, phase := range jobPhases[jobID] {
			if phase != "" && !seenPhases[phase] {
				phases = append(phases, phase)
				seenPhases[phase] = true
			}
		}
	}

	return phases
}

// FormatResult formats a validation result for display
func FormatResult(result *ValidationResult) string {
	var sb strings.Builder

	if result.Success {
		fmt.Fprintf(&sb, "✅ Pipeline '%s' ↔ Workflow %s\n", result.Pipeline, filepath.Base(result.WorkflowFile))
		fmt.Fprintf(&sb, "   Both execute phases: [%s]\n", strings.Join(result.LocalOrder, ", "))
		sb.WriteString("   Status: In sync ✓\n")
	} else {
		fmt.Fprintf(&sb, "⚠️  Pipeline '%s' ↔ Workflow %s\n", result.Pipeline, filepath.Base(result.WorkflowFile))
		sb.WriteString("   Status: Out of sync ✗\n\n")

		// Show what's in each
		fmt.Fprintf(&sb, "   📄 cidx.toml [pipelines.%s]:\n", result.Pipeline)
		fmt.Fprintf(&sb, "      phases = [%s]\n\n", strings.Join(result.LocalOrder, ", "))

		fmt.Fprintf(&sb, "   🔧 GitHub Actions [%s]:\n", filepath.Base(result.WorkflowFile))
		fmt.Fprintf(&sb, "      executes = [%s]\n\n", strings.Join(result.GitHubOrder, ", "))

		// Show differences
		sb.WriteString("   Differences:\n")

		if len(result.MissingInGH) > 0 {
			fmt.Fprintf(&sb, "      • Missing in GitHub workflow: %s\n", strings.Join(result.MissingInGH, ", "))
		}

		if len(result.MissingInLocal) > 0 {
			fmt.Fprintf(&sb, "      • Missing in cidx.toml pipeline: %s\n", strings.Join(result.MissingInLocal, ", "))
		}

		if result.OrderMismatch {
			sb.WriteString("      • Phase execution order differs\n")
		}
	}

	return sb.String()
}
