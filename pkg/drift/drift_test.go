package drift

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
)

func TestCompareFromData_AllMatch(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code", "test"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
    needs: [bootstrap]
  code:
    name: Code Quality
    needs: [bootstrap]
  test:
    name: Test
    needs: [bootstrap]
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.HasDrift() {
		t.Error("expected no drift")
	}

	for _, p := range result.Phases {
		if p.Status != StatusMatch {
			t.Errorf("phase %q: expected match, got %q", p.Name, p.Status)
		}
	}
}

func TestCompareFromData_MissingPhase(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code", "test", "build"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
  code:
    name: Code
  test:
    name: Test
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.HasDrift() {
		t.Error("expected drift (build missing from CI)")
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "build" && p.Status == StatusMissing {
			found = true
		}
	}
	if !found {
		t.Error("expected 'build' phase with status 'missing from CI'")
	}
}

func TestCompareFromData_ExtraJob(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security", "code"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
  code:
    name: Code
  test:
    name: Test
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "test" && p.Status == StatusExtra {
			found = true
		}
	}
	if !found {
		t.Error("expected 'test' job with status 'extra in CI'")
	}
}

func TestCompareFromData_TriggerMatch(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"pr":   {Phases: []string{"security"}},
			"main": {Phases: []string{"security", "build"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  security:
    name: Security
  build:
    name: Build
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tr := range result.Triggers {
		if tr.Status != StatusMatch {
			t.Errorf("trigger %q: expected match, got %q", tr.Event, tr.Status)
		}
	}
}

func TestCompareFromData_MissingTrigger(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"pr": {Phases: []string{"security"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  security:
    name: Security
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, tr := range result.Triggers {
		if tr.Event == "pull_request" && tr.Status == "missing" {
			found = true
		}
	}
	if !found {
		t.Error("expected pull_request trigger with status 'missing'")
	}
}

func TestResult_DiffCount(t *testing.T) {
	r := &Result{
		Phases: []PhaseDiff{
			{Name: "a", Status: StatusMatch},
			{Name: "b", Status: StatusMissing},
			{Name: "c", Status: StatusExtra},
		},
		Triggers: []TriggerDiff{
			{Event: "push", Status: StatusMatch},
		},
	}

	if got := r.DiffCount(); got != 2 {
		t.Errorf("DiffCount() = %d, want 2", got)
	}
}

func TestFormat(t *testing.T) {
	r := &Result{
		Workflow: ".github/workflows/ci.yml",
		Pipeline: "ci",
		Phases: []PhaseDiff{
			{Name: "security", CIDX: true, CI: true, Status: StatusMatch},
			{Name: "build", CIDX: true, CI: false, Status: StatusMissing},
		},
		Triggers: []TriggerDiff{
			{Event: "push", CIDX: true, CI: true, Status: StatusMatch},
		},
	}

	output := Format(r)
	if !strings.Contains(output, "security") {
		t.Error("expected security in output")
	}
	if !strings.Contains(output, "build") {
		t.Error("expected build in output")
	}
	if !strings.Contains(output, "missing from CI") {
		t.Error("expected 'missing from CI' in output")
	}
	if !strings.Contains(output, "Phases:") {
		t.Error("expected Phases header")
	}
	if !strings.Contains(output, "Triggers:") {
		t.Error("expected Triggers header")
	}
	if !strings.Contains(output, "Comparing pipeline 'ci' with .github/workflows/ci.yml") {
		t.Errorf("expected comparison scope header, got:\n%s", output)
	}
}

func TestCompareFromData_BootstrapIgnored(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci": {Phases: []string{"security"}},
		},
	}

	workflow := `
name: CIDX CI
on:
  push:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
    needs: [bootstrap]
`

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// bootstrap should not appear as "extra in CI"
	for _, p := range result.Phases {
		if p.Name == "bootstrap" {
			t.Error("bootstrap should be excluded from phase comparison")
		}
	}
}

// cidx's own config shape: a release pipeline declaring phases the CI workflow
// is not supposed to run (issue #178).
func splitPipelinesConfig() *config.Config {
	return &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci":      {Phases: []string{"security", "code", "test", "build"}},
			"pr":      {Phases: []string{"security", "code", "test"}},
			"release": {Phases: []string{"security", "code", "test", "build", "docker", "release"}},
		},
	}
}

const ciWorkflowYAML = `
name: CIDX CI
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
jobs:
  bootstrap:
    name: Bootstrap
  security:
    name: Security
  code:
    name: Code
  test:
    name: Test
  build:
    name: Build
`

func TestCompareFromData_OtherPipelinePhasesNotExpectedInCIWorkflow(t *testing.T) {
	result, err := CompareFromData(splitPipelinesConfig(), ".github/workflows/ci.yml", []byte(ciWorkflowYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pipeline != "ci" {
		t.Errorf("expected comparison scoped to pipeline 'ci', got %q", result.Pipeline)
	}
	if result.HasDrift() {
		t.Errorf("expected no drift, got:\n%s", Format(result))
	}
	for _, p := range result.Phases {
		if p.Name == "docker" || p.Name == "release" {
			t.Errorf("phase %q belongs to [pipelines.release] and must not be compared against the CI workflow", p.Name)
		}
	}
}

func TestCompareFromData_MissingCIPhaseStillDetected(t *testing.T) {
	cfg := splitPipelinesConfig()
	// "build" is declared by [pipelines.ci] but has no job in the workflow.
	workflow := strings.Replace(ciWorkflowYAML, "  build:\n    name: Build\n", "", 1)

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "build" && p.Status == StatusMissing {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'build' missing from CI, got:\n%s", Format(result))
	}
}

func TestCompareFromData_ExtraJobStillDetected(t *testing.T) {
	cfg := splitPipelinesConfig()
	// "docker" is declared by [pipelines.release] only — a job for it in the CI
	// workflow is genuinely extra there.
	workflow := ciWorkflowYAML + "  docker:\n    name: Docker\n"

	result, err := CompareFromData(cfg, ".github/workflows/ci.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, p := range result.Phases {
		if p.Name == "docker" && p.Status == StatusExtra {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'docker' extra in CI, got:\n%s", Format(result))
	}
}

func TestCompareFromData_ReleaseWorkflowComparesReleasePipeline(t *testing.T) {
	cfg := &config.Config{
		Pipelines: map[string]config.Pipeline{
			"ci":      {Phases: []string{"security", "code", "test", "build"}},
			"release": {Phases: []string{"build", "docker", "release"}},
		},
	}

	workflow := `
name: CIDX Release
on:
  push:
    tags: ["v*"]
jobs:
  build:
    name: Build
  docker:
    name: Docker
  release:
    name: Release
`

	result, err := CompareFromData(cfg, ".github/workflows/release.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pipeline != "release" {
		t.Errorf("expected comparison scoped to pipeline 'release', got %q", result.Pipeline)
	}
	for _, p := range result.Phases {
		if p.Status != StatusMatch {
			t.Errorf("phase %q: expected match, got %q", p.Name, p.Status)
		}
	}
}

// A workflow whose name matches no pipeline — notably the cidx.yml written by
// `cidx generate github`, which holds one job per phase of every pipeline — is
// compared against all pipelines.
func TestCompareFromData_UnnamedWorkflowComparesAllPipelines(t *testing.T) {
	cfg := splitPipelinesConfig()
	workflow := ciWorkflowYAML + "  docker:\n    name: Docker\n  release:\n    name: Release\n"

	result, err := CompareFromData(cfg, ".github/workflows/cidx.yml", []byte(workflow))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pipeline != "" {
		t.Errorf("expected unscoped comparison, got pipeline %q", result.Pipeline)
	}
	if result.HasDrift() {
		t.Errorf("expected no drift, got:\n%s", Format(result))
	}
	if len(result.Phases) != 6 {
		t.Errorf("expected all 6 phases compared, got %d", len(result.Phases))
	}
}
