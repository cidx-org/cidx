package validator

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
)

// phasesFromStep parses a one-job workflow whose only step runs script, and
// returns the phases extracted from it.
func phasesFromStep(t *testing.T, script string) []string {
	t.Helper()

	var block strings.Builder
	for _, line := range strings.Split(script, "\n") {
		block.WriteString("          " + line + "\n")
	}
	content := "name: CI\njobs:\n  only:\n    steps:\n      - run: |\n" + block.String()

	workflowPath := filepath.Join(t.TempDir(), "ci.yml")
	if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	workflow, err := ParseWorkflow(testApp(), workflowPath)
	if err != nil {
		t.Fatalf("ParseWorkflow: %v", err)
	}
	return workflow.Phases
}

// TestParseWorkflow_ExtractsPhasesFromEveryInvocationForm is the regression for
// the second half of #233: the phases were extracted by matching the substring
// "cidx run ", which `./bin/cidx --verbose run test` — the form ci.yml has used
// since #271 — does not contain. `check workflow` then reported the test phase
// as missing from a workflow that runs it, while `check drift` saw it. Reading
// the command line with the invocation parser of #239 covers every form the
// workflows really use, and keeps its refusal to judge what it cannot read.
func TestParseWorkflow_ExtractsPhasesFromEveryInvocationForm(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   []string
	}{
		{"plain invocation", "cidx run ci", []string{"ci"}},
		{"built binary", "./bin/cidx run security", []string{"security"}},
		{"relative binary", "bin/cidx run code", []string{"code"}},
		{"global flag before the subcommand", "./bin/cidx --verbose run test", []string{"test"}},
		{"valued global flag before", "cidx --config ci/pipeline.toml run ci", []string{"ci"}},
		{"valued global flag in --flag=value form", "cidx --config=ci/pipeline.toml run build", []string{"build"}},
		{"short valued global flag", "cidx -c other.toml run ci", []string{"ci"}},
		{"flag after the subcommand", "cidx run --dry-run security", []string{"security"}},
		{"valued flag after the subcommand", "cidx run -j 4 security", []string{"security"}},
		{"go run", "go run ./cmd/cidx run ci", []string{"ci"}},
		{"line continuation", "./bin/cidx run \\\n  security", []string{"security"}},
		{"several invocations in one step", "./bin/cidx run security\n./bin/cidx run code", []string{"security", "code"}},
		{"the same phase twice", "./bin/cidx run test\n./bin/cidx run test", []string{"test"}},
		{"multi-line script", "chmod +x bin/cidx\n./bin/cidx --verbose run test", []string{"test"}},
		{"after a shell keyword", "if ./bin/cidx run security; then", []string{"security"}},

		// Forms the parser deliberately refuses to judge: no phase is better
		// than a wrong one.
		{"invoked through a variable", "$CIDX run ci", nil},
		{"heredoc body", "cat <<'EOF' > doc\ncidx run ci\nEOF", nil},
		{"commented out", "# cidx run ci", nil},
		{"documented in an echo", `echo "Run it with: cidx run ci"`, nil},
		{"phase through a variable", "./bin/cidx run $PHASE", nil},
		{"unknown flag hides the subcommand", "./bin/cidx --unknown value run ci", nil},
		{"building the binary", "go build -o bin/cidx ./cmd/cidx", nil},
		{"another subcommand", "./bin/cidx check drift", nil},
		{"a run that is not the run command", "./bin/cidx workflow run ci.yml", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phasesFromStep(t, tt.script)
			if len(got) == 0 && len(tt.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("phases of %q = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

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
    needs: [security]
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
	workflow, err := ParseWorkflow(testApp(), workflowPath)
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

// TestParseWorkflow_PhaseOrderIsDeterministic covers the first half of #233:
// `check workflow` reported "Phase execution order differs" intermittently on
// cidx's own repo, because the order of jobs that no dependency separates came
// out of a Go map. Same file, same answer — every time.
func TestParseWorkflow_PhaseOrderIsDeterministic(t *testing.T) {
	// The shape of the repo's own ci.yml: three jobs run in parallel after the
	// bootstrap, so only the declaration order can order them.
	workflowPath := filepath.Join(t.TempDir(), "ci.yml")
	workflowContent := `name: CI
jobs:
  bootstrap:
    steps:
      - run: go build -o bin/cidx ./cmd/cidx
  security:
    needs: [bootstrap]
    steps:
      - run: ./bin/cidx run security
  code:
    needs: [bootstrap]
    steps:
      - run: ./bin/cidx run code
  test:
    needs: [bootstrap]
    steps:
      - run: ./bin/cidx run test
  build:
    needs: [security, code, test]
    steps:
      - run: ./bin/cidx run build
`
	if err := os.WriteFile(workflowPath, []byte(workflowContent), 0644); err != nil {
		t.Fatalf("Failed to create test workflow file: %v", err)
	}

	want := []string{"security", "code", "test", "build"}
	for i := 0; i < 30; i++ {
		workflow, err := ParseWorkflow(testApp(), workflowPath)
		if err != nil {
			t.Fatalf("Failed to parse workflow: %v", err)
		}
		if !equalOrder(workflow.Phases, want) {
			t.Fatalf("run %d extracted %v, want %v", i, workflow.Phases, want)
		}
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
	result, err := ValidateWorkflow(testApp(), cfg, "ci", workflowPath)
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
	result, err := ValidateWorkflow(testApp(), cfg, "ci", workflowPath)
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

	jobPhases := map[string][]string{
		"security": {"security"},
		"code":     {"code"},
		"test":     {"test"},
		"build":    {"build"},
	}

	phases := topologicalSort(jobs, jobPhases, []string{"setup", "security", "code", "test", "build"})

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

// stageWorkflows writes the given workflow files into a fresh .github/workflows
// directory and returns it.
func stageWorkflows(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), ".github", "workflows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The two workflows this repository actually has: a CI workflow that mirrors
// [pipelines.ci] through cidx, and a release workflow that publishes natively
// and delegates a single phase.
const (
	mirrorCIWorkflow = `name: CI
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
      - run: ./bin/cidx --verbose run test
  build:
    needs: [test]
    steps:
      - run: ./bin/cidx run build
`

	nativeReleaseWorkflow = `name: Release
jobs:
  build:
    steps:
      - run: GOOS=linux go build -o dist/cidx ./cmd/cidx
  docker:
    needs: [build]
    steps:
      - run: ./bin/cidx run docker
  release:
    needs: [build, docker]
    steps:
      - uses: softprops/action-gh-release@v3
`
)

func releaseRepoConfig(releaseWorkflow string) *config.Config {
	return &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci":      {Phases: []string{"security", "code", "test", "build"}},
			"pr":      {Phases: []string{"security", "code", "test"}},
			"release": {Phases: []string{"security", "code", "test", "build", "docker", "release"}, Workflow: releaseWorkflow},
		},
	}
}

func pipelinesValidated(results []*ValidationResult) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.Pipeline)
	}
	return names
}

// TestValidateAllWorkflows_SkipsAPipelineNoWorkflowImplements is the second half
// of #233: `check workflow` exited 1 on this repository because release.yml
// shares a name with [pipelines.release]. It never was that pipeline's mirror —
// it publishes the release natively and delegates only `docker` to cidx, while
// the pipeline is the end-to-end rehearsal `cidx run release` walks locally.
// The pairing is now declared, so the two are no longer compared.
func TestValidateAllWorkflows_SkipsAPipelineNoWorkflowImplements(t *testing.T) {
	dir := stageWorkflows(t, map[string]string{
		"ci.yml":      mirrorCIWorkflow,
		"release.yml": nativeReleaseWorkflow,
	})

	results, err := ValidateAllWorkflows(testApp(), releaseRepoConfig(config.NoWorkflow), dir)
	if err != nil {
		t.Fatalf("ValidateAllWorkflows: %v", err)
	}

	if got := pipelinesValidated(results); !reflect.DeepEqual(got, []string{"ci"}) {
		t.Fatalf("validated pipelines = %v, want [ci]", got)
	}
	if !results[0].Success {
		t.Errorf("expected ci to be in sync, got:\n%s", FormatResult(results[0]))
	}
}

// TestValidateAllWorkflows_ComparesByConventionWhenNothingIsDeclared is the
// guard on the fix above: the exception has to be written down to apply. A
// project whose release.yml really does run the release pipeline keeps the
// filename convention, and keeps failing when the two disagree.
func TestValidateAllWorkflows_ComparesByConventionWhenNothingIsDeclared(t *testing.T) {
	dir := stageWorkflows(t, map[string]string{
		"ci.yml":      mirrorCIWorkflow,
		"release.yml": nativeReleaseWorkflow,
	})

	results, err := ValidateAllWorkflows(testApp(), releaseRepoConfig(""), dir)
	if err != nil {
		t.Fatalf("ValidateAllWorkflows: %v", err)
	}

	if got := pipelinesValidated(results); !reflect.DeepEqual(got, []string{"ci", "release"}) {
		t.Fatalf("validated pipelines = %v, want [ci release]", got)
	}
	if results[1].Success {
		t.Error("expected the undeclared release pipeline to be compared with release.yml and fail")
	}
}

// TestValidateAllWorkflows_StillReportsRealDriftOnCI is the reason the fix is
// an explicit declaration rather than a softer verdict on the "declared but not
// executed" direction: on ci.yml that direction is exactly the drift the check
// exists to catch. Both ways of drifting a mirror workflow must keep failing.
func TestValidateAllWorkflows_StillReportsRealDriftOnCI(t *testing.T) {
	cfg := releaseRepoConfig(config.NoWorkflow)

	t.Run("a phase the workflow stopped running", func(t *testing.T) {
		// The test job survives, its `cidx run test` call does not.
		drifted := strings.Replace(mirrorCIWorkflow, "./bin/cidx --verbose run test", "echo 'tests disabled'", 1)
		dir := stageWorkflows(t, map[string]string{"ci.yml": drifted, "release.yml": nativeReleaseWorkflow})

		results, err := ValidateAllWorkflows(testApp(), cfg, dir)
		if err != nil {
			t.Fatalf("ValidateAllWorkflows: %v", err)
		}
		if len(results) != 1 || results[0].Success {
			t.Fatalf("expected ci to be reported out of sync, got %d result(s)", len(results))
		}
		if !reflect.DeepEqual(results[0].MissingInGH, []string{"test"}) {
			t.Errorf("missing in workflow = %v, want [test]", results[0].MissingInGH)
		}
	})

	t.Run("a phase added to the pipeline only", func(t *testing.T) {
		extended := &config.Config{Pipelines: map[string]config.Pipeline{
			"ci":      {Phases: []string{"security", "code", "test", "build", "docker"}},
			"release": {Phases: []string{"docker"}, Workflow: config.NoWorkflow},
		}}
		dir := stageWorkflows(t, map[string]string{"ci.yml": mirrorCIWorkflow, "release.yml": nativeReleaseWorkflow})

		results, err := ValidateAllWorkflows(testApp(), extended, dir)
		if err != nil {
			t.Fatalf("ValidateAllWorkflows: %v", err)
		}
		if len(results) != 1 || results[0].Success {
			t.Fatalf("expected ci to be reported out of sync, got %d result(s)", len(results))
		}
		if !reflect.DeepEqual(results[0].MissingInGH, []string{"docker"}) {
			t.Errorf("missing in workflow = %v, want [docker]", results[0].MissingInGH)
		}
	})

	t.Run("a phase the workflow runs without declaring it", func(t *testing.T) {
		trimmed := &config.Config{Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code", "test"}},
		}}
		dir := stageWorkflows(t, map[string]string{"ci.yml": mirrorCIWorkflow})

		results, err := ValidateAllWorkflows(testApp(), trimmed, dir)
		if err != nil {
			t.Fatalf("ValidateAllWorkflows: %v", err)
		}
		if len(results) != 1 || results[0].Success {
			t.Fatalf("expected ci to be reported out of sync, got %d result(s)", len(results))
		}
		if !reflect.DeepEqual(results[0].MissingInLocal, []string{"build"}) {
			t.Errorf("missing in cidx.toml = %v, want [build]", results[0].MissingInLocal)
		}
	})
}

// TestValidateAllWorkflows_IsDeterministic guards what the first half of #233
// fixed one level up: the pipelines are now walked from a Go map, so they have
// to be sorted before they are reported.
func TestValidateAllWorkflows_IsDeterministic(t *testing.T) {
	dir := stageWorkflows(t, map[string]string{
		"ci.yml":      mirrorCIWorkflow,
		"release.yml": nativeReleaseWorkflow,
	})
	cfg := releaseRepoConfig("")

	for i := 0; i < 20; i++ {
		results, err := ValidateAllWorkflows(testApp(), cfg, dir)
		if err != nil {
			t.Fatalf("ValidateAllWorkflows: %v", err)
		}
		if got := pipelinesValidated(results); !reflect.DeepEqual(got, []string{"ci", "release"}) {
			t.Fatalf("run %d: validated pipelines = %v, want [ci release]", i, got)
		}
	}
}
