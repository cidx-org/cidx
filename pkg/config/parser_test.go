package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_TOML(t *testing.T) {
	// Create a temporary TOML config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.toml")

	configContent := `
[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["prettier"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load the config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify security phase containers
	securityPhase, ok := cfg.Phases["security"]
	if !ok {
		t.Fatal("Phases[\"security\"] not found")
	}
	if len(securityPhase.Containers) != 2 {
		t.Errorf("security.Containers length = %d, want 2", len(securityPhase.Containers))
	}
	if securityPhase.Containers[0] != "trivy" {
		t.Errorf("security.Containers[0] = %q, want %q", securityPhase.Containers[0], "trivy")
	}
	if securityPhase.Containers[1] != "gitleaks" {
		t.Errorf("security.Containers[1] = %q, want %q", securityPhase.Containers[1], "gitleaks")
	}

	// Verify code phase containers
	codePhase, ok := cfg.Phases["code"]
	if !ok {
		t.Fatal("Phases[\"code\"] not found")
	}
	if len(codePhase.Containers) != 1 {
		t.Errorf("code.Containers length = %d, want 1", len(codePhase.Containers))
	}
	if codePhase.Containers[0] != "prettier" {
		t.Errorf("code.Containers[0] = %q, want %q", codePhase.Containers[0], "prettier")
	}

	// Verify container overrides
	trivyConfig, ok := cfg.Overrides["trivy"]
	if !ok {
		t.Fatal("Overrides[\"trivy\"] not found")
	}

	if severity, ok := trivyConfig["severity"].(string); !ok || severity != "HIGH,CRITICAL" {
		t.Errorf("trivy.severity = %v, want %q", trivyConfig["severity"], "HIGH,CRITICAL")
	}

	if exitCode, ok := trivyConfig["exit_code"].(int64); !ok || exitCode != 1 {
		t.Errorf("trivy.exit_code = %v, want 1", trivyConfig["exit_code"])
	}
}

func TestLoad_LegacyTopLevelOverride(t *testing.T) {
	// Create a temporary TOML config file using the legacy top-level override syntax.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config-legacy.toml")

	configContent := `
[security]
containers = ["trivy"]

[trivy]
severity = "HIGH,CRITICAL"
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	trivyConfig, ok := cfg.Overrides["trivy"]
	if !ok {
		t.Fatal("Overrides[\"trivy\"] not found")
	}

	if severity, ok := trivyConfig["severity"].(string); !ok || severity != "HIGH,CRITICAL" {
		t.Errorf("trivy.severity = %v, want %q", trivyConfig["severity"], "HIGH,CRITICAL")
	}
}

// TestPipelineWorkflowFile covers the pairing `check workflow` reads: the
// filename convention by default, an explicit file when the workflow is named
// otherwise, and nothing at all when the pipeline declares that no workflow
// implements it (issue #233).
func TestPipelineWorkflowFile(t *testing.T) {
	tests := []struct {
		name     string
		pipeline string
		declared string
		want     string
	}{
		{"convention when unset", "ci", "", "ci.yml"},
		{"convention follows the pipeline name", "release", "", "release.yml"},
		{"an explicitly named workflow", "ci", "main.yml", "main.yml"},
		{"no workflow implements it", "release", NoWorkflow, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Pipeline{Phases: []string{"security"}, Workflow: tt.declared}
			if got := p.WorkflowFile(tt.pipeline); got != tt.want {
				t.Errorf("WorkflowFile(%q) with workflow = %q = %q, want %q", tt.pipeline, tt.declared, got, tt.want)
			}
		})
	}
}

// TestLoad_PipelineWorkflow checks the key survives the real loader — the
// pipeline section is decoded through a typed struct, so an unlisted key would
// be dropped in silence.
func TestLoad_PipelineWorkflow(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "cidx.toml")
	configContent := `
[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]

[pipelines.release]
phases = ["security"]
workflow = "none"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.Pipelines["ci"].WorkflowFile("ci"); got != "ci.yml" {
		t.Errorf("ci pipeline workflow = %q, want %q", got, "ci.yml")
	}
	if got := cfg.Pipelines["release"].WorkflowFile("release"); got != "" {
		t.Errorf("release pipeline workflow = %q, want none", got)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.toml")
	if err == nil {
		t.Error("Load() expected error for non-existent file, got nil")
	}
}

func TestExpandEnvVars(t *testing.T) {
	// Set test environment variables
	_ = os.Setenv("TEST_WORKSPACE", "/home/user/project")
	_ = os.Setenv("TEST_IMAGE", "myimage:latest")
	defer func() { _ = os.Unsetenv("TEST_WORKSPACE") }()
	defer func() { _ = os.Unsetenv("TEST_IMAGE") }()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple variable",
			input: "${TEST_WORKSPACE}/src",
			want:  "/home/user/project/src",
		},
		{
			name:  "multiple variables",
			input: "${TEST_WORKSPACE}:${TEST_IMAGE}",
			want:  "/home/user/project:myimage:latest",
		},
		{
			name:  "no variables",
			input: "/static/path",
			want:  "/static/path",
		},
		{
			name:  "undefined variable",
			input: "${UNDEFINED_VAR}",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := os.ExpandEnv(tt.input)
			if got != tt.want {
				t.Errorf("ExpandEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestLoad_PipelineDescription covers issue #352: `description` was written on
// all five pipelines of this repository's own cidx.toml and dropped by the
// decoder without a word, because config.Pipeline had no field for it.
//
// The BDD suite carried the consequence for a while: a scenario claimed a
// pipeline "indicates its purpose", the step behind it asserted nothing, and
// the claim survived (#349). A key nobody reads and a step that checks nothing
// fail the same way — silently.
func TestLoad_PipelineDescription(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "cidx.toml")
	configContent := `
[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]
description = "Full CI pipeline for all commits"

[pipelines.quick]
phases = ["security"]
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got := cfg.Pipelines["ci"].Description; got != "Full CI pipeline for all commits" {
		t.Errorf("description = %q, want the one the file states", got)
	}
	if got := cfg.Pipelines["quick"].Description; got != "" {
		t.Errorf("a pipeline that states no description should have none, got %q", got)
	}
}
