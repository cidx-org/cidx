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
tools = ["trivy", "gitleaks"]

[code]
tools = ["prettier"]

[trivy]
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

	// Verify security phase tools
	securityPhase, ok := cfg.Phases["security"]
	if !ok {
		t.Fatal("Phases[\"security\"] not found")
	}
	if len(securityPhase.Tools) != 2 {
		t.Errorf("security.Tools length = %d, want 2", len(securityPhase.Tools))
	}
	if securityPhase.Tools[0] != "trivy" {
		t.Errorf("security.Tools[0] = %q, want %q", securityPhase.Tools[0], "trivy")
	}
	if securityPhase.Tools[1] != "gitleaks" {
		t.Errorf("security.Tools[1] = %q, want %q", securityPhase.Tools[1], "gitleaks")
	}

	// Verify code phase tools
	codePhase, ok := cfg.Phases["code"]
	if !ok {
		t.Fatal("Phases[\"code\"] not found")
	}
	if len(codePhase.Tools) != 1 {
		t.Errorf("code.Tools length = %d, want 1", len(codePhase.Tools))
	}
	if codePhase.Tools[0] != "prettier" {
		t.Errorf("code.Tools[0] = %q, want %q", codePhase.Tools[0], "prettier")
	}

	// Verify tool overrides
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

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/nonexistent/config.toml")
	if err == nil {
		t.Error("Load() expected error for non-existent file, got nil")
	}
}

func TestExpandEnvVars(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_WORKSPACE", "/home/user/project")
	os.Setenv("TEST_IMAGE", "myimage:latest")
	defer os.Unsetenv("TEST_WORKSPACE")
	defer os.Unsetenv("TEST_IMAGE")

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
