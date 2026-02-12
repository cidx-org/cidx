package environment

import (
	"testing"

	"github.com/cidx-org/cidx/pkg/presets"
)

func TestValidatePreset_CI(t *testing.T) {
	preset := presets.Preset{Name: "gh-release", RequireCI: true}
	env := &Environment{IsCI: true, Provider: "github"}

	mode, err := ValidatePreset(preset, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mode.Allowed {
		t.Error("expected Allowed=true in CI")
	}
	if mode.Mode != BehaviorProduction {
		t.Errorf("expected mode %q, got %q", BehaviorProduction, mode.Mode)
	}
}

func TestValidatePreset_Local_RequireCI_StrictBlock(t *testing.T) {
	preset := presets.Preset{Name: "deploy", RequireCI: true, LocalBehavior: ""}
	env := &Environment{IsCI: false, Provider: "local"}

	_, err := ValidatePreset(preset, env)
	if err == nil {
		t.Fatal("expected error for RequireCI preset in local")
	}
}

func TestValidatePreset_Local_RequireCI_WithBehavior(t *testing.T) {
	preset := presets.Preset{Name: "gh-release", RequireCI: true, LocalBehavior: "draft"}
	env := &Environment{IsCI: false, Provider: "local"}

	mode, err := ValidatePreset(preset, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode.Mode != BehaviorDraft {
		t.Errorf("expected mode %q, got %q", BehaviorDraft, mode.Mode)
	}
	if !mode.IsDryRun {
		t.Error("expected IsDryRun=true for draft mode")
	}
}

func TestValidatePreset_Local_Behaviors(t *testing.T) {
	tests := []struct {
		name      string
		behavior  string
		wantMode  string
		wantDry   bool
		wantError bool
	}{
		{"production", "production", BehaviorProduction, false, false},
		{"draft", "draft", BehaviorDraft, true, false},
		{"no-push", "no-push", BehaviorNoPush, true, false},
		{"dry-run", "dry-run", BehaviorDryRun, true, false},
		{"disabled", "disabled", "", false, true},
		{"unknown", "bogus", "", false, true},
		{"empty defaults to production", "", BehaviorProduction, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset := presets.Preset{Name: "test", LocalBehavior: tt.behavior}
			env := &Environment{IsCI: false, Provider: "local"}

			mode, err := ValidatePreset(preset, env)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode.Mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode.Mode, tt.wantMode)
			}
			if mode.IsDryRun != tt.wantDry {
				t.Errorf("IsDryRun = %v, want %v", mode.IsDryRun, tt.wantDry)
			}
		})
	}
}

func TestValidatePreset_Draft_EnvChanges(t *testing.T) {
	preset := presets.Preset{Name: "gh-release", LocalBehavior: "draft"}
	env := &Environment{IsCI: false}

	mode, err := ValidatePreset(preset, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode.EnvChanges["DRAFT"] != "true" {
		t.Errorf("expected DRAFT=true in EnvChanges, got %q", mode.EnvChanges["DRAFT"])
	}
}

func TestValidatePreset_NoPush_EnvChanges(t *testing.T) {
	preset := presets.Preset{Name: "docker-buildx", LocalBehavior: "no-push"}
	env := &Environment{IsCI: false}

	mode, err := ValidatePreset(preset, env)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode.EnvChanges["DOCKER_PUSH"] != "false" {
		t.Errorf("expected DOCKER_PUSH=false in EnvChanges, got %q", mode.EnvChanges["DOCKER_PUSH"])
	}
}

func TestApplyExecutionMode_Draft(t *testing.T) {
	preset := presets.Preset{
		Name:    "gh-release",
		Command: "gh release create",
	}
	mode := &ExecutionMode{
		Mode:       BehaviorDraft,
		EnvChanges: map[string]string{"DRAFT": "true"},
	}

	result := ApplyExecutionMode(preset, mode)

	if result.Command != "gh release create --draft" {
		t.Errorf("expected command with --draft, got %q", result.Command)
	}
	if result.Env["DRAFT"] != "true" {
		t.Errorf("expected DRAFT=true in env, got %q", result.Env["DRAFT"])
	}
}

func TestApplyExecutionMode_NoPush(t *testing.T) {
	preset := presets.Preset{
		Name:    "docker-buildx",
		Command: "docker buildx build --push .",
	}
	mode := &ExecutionMode{
		Mode:       BehaviorNoPush,
		EnvChanges: map[string]string{"DOCKER_PUSH": "false"},
	}

	result := ApplyExecutionMode(preset, mode)

	if result.Command != "docker buildx build ." {
		t.Errorf("expected --push removed, got %q", result.Command)
	}
}

func TestApplyExecutionMode_Production(t *testing.T) {
	preset := presets.Preset{
		Name:    "gh-release",
		Command: "gh release create",
	}
	mode := &ExecutionMode{
		Mode:       BehaviorProduction,
		EnvChanges: map[string]string{},
	}

	result := ApplyExecutionMode(preset, mode)

	if result.Command != "gh release create" {
		t.Errorf("expected unchanged command, got %q", result.Command)
	}
}

func TestRemoveFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		flag    string
		want    string
	}{
		{"flag with trailing space", "build --push --tag foo", "--push", "build --tag foo"},
		{"flag at end", "build --push", "--push", "build "},
		{"no flag present", "build --tag foo", "--push", "build --tag foo"},
		{"multiple occurrences", "--push build --push", "--push", "build "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeFlag(tt.command, tt.flag)
			if got != tt.want {
				t.Errorf("removeFlag(%q, %q) = %q, want %q", tt.command, tt.flag, got, tt.want)
			}
		})
	}
}
