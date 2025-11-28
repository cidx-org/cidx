package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cidx-org/cidx/pkg/config"
	"gopkg.in/yaml.v3"
)

// WorkflowDefinition represents a GitHub Actions workflow
type WorkflowDefinition struct {
	Name   string            // Workflow name (e.g., "ci", "release")
	File   string            // Workflow file path
	Jobs   map[string]Job    // Jobs defined in the workflow
	Phases []string          // Extracted phases from "cidx run <phase>" commands
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
	Name string                    `yaml:"name"`
	Jobs map[string]WorkflowJobYAML `yaml:"jobs"`
}

// WorkflowJobYAML represents a job in the workflow YAML
type WorkflowJobYAML struct {
	Name  string              `yaml:"name"`
	Needs interface{}         `yaml:"needs"` // Can be string or []string
	Steps []WorkflowStepYAML `yaml:"steps"`
}

// WorkflowStepYAML represents a step in a job
type WorkflowStepYAML struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// ParseWorkflow parses a GitHub Actions workflow file and extracts phase information
func ParseWorkflow(workflowPath string) (*WorkflowDefinition, error) {
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
	jobPhases := make(map[string]string) // jobID -> phase

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
		var jobPhase string
		for _, stepYAML := range jobYAML.Steps {
			steps = append(steps, Step(stepYAML))

			// Extract phase from "cidx run <phase>" commands
			if strings.Contains(stepYAML.Run, "cidx run ") {
				parts := strings.Fields(stepYAML.Run)
				for i, part := range parts {
					if part == "run" && i+1 < len(parts) {
						jobPhase = parts[i+1]
						jobPhases[jobID] = jobPhase
					}
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
	phases := topologicalSort(jobs, jobPhases)

	return &WorkflowDefinition{
		Name:   workflowName,
		File:   workflowPath,
		Jobs:   jobs,
		Phases: phases,
	}, nil
}

// ValidateWorkflow compares a pipeline definition with a GitHub Actions workflow
func ValidateWorkflow(cfg *config.Config, pipelineName string, workflowPath string) (*ValidationResult, error) {
	// Get pipeline from config
	pipeline, exists := cfg.Pipelines[pipelineName]
	if !exists {
		return nil, fmt.Errorf("pipeline '%s' not found in configuration", pipelineName)
	}

	// Parse GitHub workflow
	workflow, err := ParseWorkflow(workflowPath)
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

// ValidateAllWorkflows validates all pipelines against their corresponding workflows
func ValidateAllWorkflows(cfg *config.Config, workflowDir string) ([]*ValidationResult, error) {
	results := []*ValidationResult{}

	// Map pipeline names to workflow files
	workflowMap := map[string]string{
		"ci":      filepath.Join(workflowDir, "ci.yml"),
		"release": filepath.Join(workflowDir, "release.yml"),
	}

	for pipelineName, workflowPath := range workflowMap {
		// Check if workflow file exists
		if _, err := os.Stat(workflowPath); os.IsNotExist(err) {
			continue // Skip if workflow doesn't exist
		}

		result, err := ValidateWorkflow(cfg, pipelineName, workflowPath)
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

// topologicalSort performs a topological sort of jobs based on dependencies
// and returns the phases in execution order
func topologicalSort(jobs map[string]Job, jobPhases map[string]string) []string {
	// Build adjacency list and in-degree map
	inDegree := make(map[string]int)
	graph := make(map[string][]string)

	// Initialize all jobs with in-degree 0
	for jobID := range jobs {
		inDegree[jobID] = 0
		graph[jobID] = []string{}
	}

	// Build graph and calculate in-degrees
	for jobID, job := range jobs {
		for _, dep := range job.Needs {
			graph[dep] = append(graph[dep], jobID)
			inDegree[jobID]++
		}
	}

	// Find all jobs with in-degree 0 (no dependencies)
	queue := []string{}
	for jobID, degree := range inDegree {
		if degree == 0 {
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
		if phase, exists := jobPhases[jobID]; exists && phase != "" {
			if !seenPhases[phase] {
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
		sb.WriteString(fmt.Sprintf("✅ Pipeline '%s' ↔ Workflow %s\n", result.Pipeline, filepath.Base(result.WorkflowFile)))
		sb.WriteString(fmt.Sprintf("   Both execute phases: [%s]\n", strings.Join(result.LocalOrder, ", ")))
		sb.WriteString("   Status: In sync ✓\n")
	} else {
		sb.WriteString(fmt.Sprintf("⚠️  Pipeline '%s' ↔ Workflow %s\n", result.Pipeline, filepath.Base(result.WorkflowFile)))
		sb.WriteString("   Status: Out of sync ✗\n\n")

		// Show what's in each
		sb.WriteString(fmt.Sprintf("   📄 cidx.toml [pipelines.%s]:\n", result.Pipeline))
		sb.WriteString(fmt.Sprintf("      phases = [%s]\n\n", strings.Join(result.LocalOrder, ", ")))

		sb.WriteString(fmt.Sprintf("   🔧 GitHub Actions [%s]:\n", filepath.Base(result.WorkflowFile)))
		sb.WriteString(fmt.Sprintf("      executes = [%s]\n\n", strings.Join(result.GitHubOrder, ", ")))

		// Show differences
		sb.WriteString("   Differences:\n")

		if len(result.MissingInGH) > 0 {
			sb.WriteString(fmt.Sprintf("      • Missing in GitHub workflow: %s\n", strings.Join(result.MissingInGH, ", ")))
		}

		if len(result.MissingInLocal) > 0 {
			sb.WriteString(fmt.Sprintf("      • Missing in cidx.toml pipeline: %s\n", strings.Join(result.MissingInLocal, ", ")))
		}

		if result.OrderMismatch {
			sb.WriteString("      • Phase execution order differs\n")
		}
	}

	return sb.String()
}
