package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/arcker/cidx/pkg/config"
)

func TestParseWorkflow(t *testing.T) {
	// Create a temporary workflow file
	tmpDir := t.TempDir()
	workflowPath := filepath.Join(tmpDir, "ci.yml")

	workflowContent := `name: CI Pipeline
jobs:
  setup:
    name: Setup
    steps:
      - name: Build
        run: go build
  security:
    name: Security
    needs: [setup]
    steps:
      - name: Run security
        run: ./bin/cidx run security
  code:
    name: Code Quality
    needs: [setup]
    steps:
      - name: Run code checks
        run: ./bin/cidx run code
  test:
    name: Test
    needs: [security, code]
    steps:
      - name: Run tests
        run: ./bin/cidx run test
`

	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to create test workflow file: %v", err)
	}

	// Parse the workflow
	workflow, err := ParseWorkflow(workflowPath)
	if err != nil {
		t.Fatalf("Failed to parse workflow: %v", err)
	}

	// Verify workflow name
	if workflow.Name != "ci" {
		t.Errorf("Expected workflow name 'ci', got '%s'", workflow.Name)
	}

	// Verify phases are extracted in topological order
	expectedPhases := []string{"security", "code", "test"}
	if len(workflow.Phases) != len(expectedPhases) {
		t.Errorf("Expected %d phases, got %d", len(expectedPhases), len(workflow.Phases))
	}

	for i, phase := range expectedPhases {
		if i >= len(workflow.Phases) || workflow.Phases[i] != phase {
			t.Errorf("Expected phase %d to be '%s', got '%s'", i, phase, workflow.Phases[i])
		}
	}

	// Verify jobs are parsed correctly
	if len(workflow.Jobs) != 4 {
		t.Errorf("Expected 4 jobs, got %d", len(workflow.Jobs))
	}

	// Verify dependencies
	securityJob := workflow.Jobs["security"]
	if len(securityJob.Needs) != 1 || securityJob.Needs[0] != "setup" {
		t.Errorf("Expected security job to depend on setup, got %v", securityJob.Needs)
	}

	testJob := workflow.Jobs["test"]
	if len(testJob.Needs) != 2 {
		t.Errorf("Expected test job to have 2 dependencies, got %d", len(testJob.Needs))
	}
}

func TestValidateWorkflow(t *testing.T) {
	// Create temporary files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "cidx.toml")
	workflowPath := filepath.Join(tmpDir, "ci.yml")

	// Create config file
	configContent := `[security]
containers = ["trivy"]

[code]
containers = ["prettier"]

[test]
containers = ["go-test"]

[pipelines.ci]
phases = ["security", "code", "test"]
description = "CI pipeline"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Create workflow file that matches the config
	workflowContent := `name: CI
jobs:
  security:
    steps:
      - run: ./bin/cidx run security
  code:
    needs: [security]
    steps:
      - run: ./bin/cidx run code
  test:
    needs: [code]
    steps:
      - run: ./bin/cidx run test
`
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to create test workflow file: %v", err)
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate workflow
	result, err := ValidateWorkflow(cfg, "ci", workflowPath)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Verify validation passed
	if !result.Success {
		t.Errorf("Expected validation to pass, but it failed")
		t.Logf("Missing in GitHub: %v", result.MissingInGH)
		t.Logf("Missing in local: %v", result.MissingInLocal)
		t.Logf("Order mismatch: %v", result.OrderMismatch)
	}

	// Verify no missing phases
	if len(result.MissingInGH) > 0 {
		t.Errorf("Expected no missing phases in GitHub, got %v", result.MissingInGH)
	}
	if len(result.MissingInLocal) > 0 {
		t.Errorf("Expected no missing phases in local, got %v", result.MissingInLocal)
	}

	// Verify no order mismatch
	if result.OrderMismatch {
		t.Errorf("Expected no order mismatch")
		t.Logf("Local order: %v", result.LocalOrder)
		t.Logf("GitHub order: %v", result.GitHubOrder)
	}
}

func TestValidateWorkflowMissingPhase(t *testing.T) {
	// Create temporary files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "cidx.toml")
	workflowPath := filepath.Join(tmpDir, "ci.yml")

	// Create config with security, code, test
	configContent := `[security]
containers = ["trivy"]

[code]
containers = ["prettier"]

[test]
containers = ["go-test"]

[pipelines.ci]
phases = ["security", "code", "test"]
description = "CI pipeline"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Create workflow that only has security and code (missing test)
	workflowContent := `name: CI
jobs:
  security:
    steps:
      - run: ./bin/cidx run security
  code:
    needs: [security]
    steps:
      - run: ./bin/cidx run code
`
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to create test workflow file: %v", err)
	}

	// Load config
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Validate workflow
	result, err := ValidateWorkflow(cfg, "ci", workflowPath)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Verify validation failed
	if result.Success {
		t.Errorf("Expected validation to fail, but it passed")
	}

	// Verify test phase is missing in GitHub
	if len(result.MissingInGH) != 1 || result.MissingInGH[0] != "test" {
		t.Errorf("Expected 'test' to be missing in GitHub, got %v", result.MissingInGH)
	}
}

func TestTopologicalSort(t *testing.T) {
	// Create a simple dependency graph:
	// setup -> security, code -> test -> build
	jobs := map[string]Job{
		"setup": {
			Name:  "Setup",
			Needs: []string{},
		},
		"security": {
			Name:  "Security",
			Needs: []string{"setup"},
		},
		"code": {
			Name:  "Code",
			Needs: []string{"setup"},
		},
		"test": {
			Name:  "Test",
			Needs: []string{"security", "code"},
		},
		"build": {
			Name:  "Build",
			Needs: []string{"test"},
		},
	}

	jobPhases := map[string]string{
		"security": "security",
		"code":     "code",
		"test":     "test",
		"build":    "build",
	}

	phases := topologicalSort(jobs, jobPhases)

	// Expected order: security, code (parallel after setup), test, build
	// Note: security and code can be in any order since they're parallel
	expectedPhases := []string{"security", "code", "test", "build"}

	if len(phases) != len(expectedPhases) {
		t.Errorf("Expected %d phases, got %d", len(expectedPhases), len(phases))
	}

	// Check that test comes after both security and code
	testIdx := -1
	securityIdx := -1
	codeIdx := -1
	buildIdx := -1

	for i, phase := range phases {
		switch phase {
		case "test":
			testIdx = i
		case "security":
			securityIdx = i
		case "code":
			codeIdx = i
		case "build":
			buildIdx = i
		}
	}

	if testIdx < securityIdx || testIdx < codeIdx {
		t.Errorf("Expected test to come after security and code")
	}

	if buildIdx < testIdx {
		t.Errorf("Expected build to come after test")
	}
}

func TestDifference(t *testing.T) {
	a := []string{"a", "b", "c", "d"}
	b := []string{"b", "d"}

	diff := difference(a, b)

	expected := []string{"a", "c"}
	if len(diff) != len(expected) {
		t.Errorf("Expected %d elements in difference, got %d", len(expected), len(diff))
	}

	for i, val := range expected {
		if diff[i] != val {
			t.Errorf("Expected element %d to be '%s', got '%s'", i, val, diff[i])
		}
	}
}

func TestEqualOrder(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "Same order",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "Different order",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "c", "b"},
			expected: false,
		},
		{
			name:     "Different length",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "Empty slices",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := equalOrder(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("equalOrder(%v, %v) = %v, expected %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}
