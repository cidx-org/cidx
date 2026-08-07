package environment

import (
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
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
		{"dry-run", "dry-run", BehaviorDryRun, true, false},
		{"disabled", "disabled", "", false, true},
		{"unknown", "bogus", "", false, true},
		// Removed in v3.0.0, and refused by name rather than as an unknown
		// value: it behaved identically to dry-run, so "unknown
		// local_behavior" would send the reader looking for a difference
		// that never existed (issue #353).
		{"the removed no-push", "no-push", "", false, true},
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

// TestValidatePreset_NoPushNamesItsReplacement: a config still carrying the
// removed value must be told what to write instead, not merely that the value
// is not recognised (issue #353).
func TestValidatePreset_NoPushNamesItsReplacement(t *testing.T) {
	preset := presets.Preset{Name: "docker-buildx", LocalBehavior: "no-push"}

	_, err := ValidatePreset(preset, &Environment{IsCI: false, Provider: "local"})
	if err == nil {
		t.Fatal("expected the removed behaviour to be refused")
	}
	for _, want := range []string{"removed in cidx v3.0.0", "dry-run", "docker-buildx"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal = %q, want it to mention %q", err, want)
		}
	}
}

// TestApplyExecutionMode_LeavesTheCommandAlone: nothing rewrites a docker
// command any more. no-push stripped --push from docker-buildx, which doctored
// a command that a dry-run never runs; a local dry-run now prints what CI will
// really execute (issue #353).
func TestApplyExecutionMode_LeavesTheCommandAlone(t *testing.T) {
	preset := presets.Preset{
		Name:    "docker-buildx",
		Command: "docker buildx build --push .",
	}
	mode := &ExecutionMode{Mode: BehaviorDryRun, IsDryRun: true}

	result := ApplyExecutionMode(preset, mode)

	if result.Command != "docker buildx build --push ." {
		t.Errorf("expected the command untouched, got %q", result.Command)
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
