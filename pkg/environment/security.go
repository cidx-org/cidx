package environment

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/presets"
)

// LocalBehavior defines how a preset behaves in local environment
const (
	BehaviorProduction = "production" // Full execution (dangerous in local)
	BehaviorDraft      = "draft"      // Create drafts only (GitHub releases)
	BehaviorNoPush     = "no-push"    // Build without push (Docker)
	BehaviorDryRun     = "dry-run"    // Dry run only
	BehaviorDisabled   = "disabled"   // Completely disabled in local
)

// ExecutionMode determines how a preset should be executed
type ExecutionMode struct {
	Allowed    bool   // Can this preset run in current environment?
	Mode       string // Execution mode (production, draft, no-push, dry-run)
	Reason     string // Why this mode was chosen
	IsDryRun   bool   // Force dry-run mode
	EnvChanges map[string]string // Environment variable overrides
}

// ValidatePreset checks if a preset can run in the current environment
// and returns the appropriate execution mode
func ValidatePreset(preset presets.Preset, env *Environment) (*ExecutionMode, error) {
	mode := &ExecutionMode{
		Allowed:    true,
		Mode:       BehaviorProduction,
		Reason:     "",
		IsDryRun:   false,
		EnvChanges: make(map[string]string),
	}

	// If in CI, always allow production mode
	if env.IsCI {
		mode.Mode = BehaviorProduction
		mode.Reason = fmt.Sprintf("Running in CI (%s)", env.Provider)
		return mode, nil
	}

	// Running locally
	// Check if preset requires CI
	if preset.RequireCI && preset.LocalBehavior == "" {
		// Strict mode: completely disallow
		return nil, fmt.Errorf("preset '%s' requires CI environment (detected: local)", preset.Name)
	}

	// Apply local behavior
	localBehavior := preset.LocalBehavior
	if localBehavior == "" {
		// No local behavior specified, default to production (backward compat)
		localBehavior = BehaviorProduction
	}

	switch localBehavior {
	case BehaviorDisabled:
		return nil, fmt.Errorf("preset '%s' is disabled in local environment", preset.Name)

	case BehaviorDryRun:
		mode.Mode = BehaviorDryRun
		mode.IsDryRun = true
		mode.Reason = "Local mode: dry-run only"

	case BehaviorDraft:
		mode.Mode = BehaviorDraft
		mode.IsDryRun = true // Force dry-run in local mode
		mode.Reason = "Local mode: draft creation only"
		// For GitHub releases, force draft mode
		mode.EnvChanges["DRAFT"] = "true"

	case BehaviorNoPush:
		mode.Mode = BehaviorNoPush
		mode.IsDryRun = true // Force dry-run in local mode
		mode.Reason = "Local mode: build without push"
		// For Docker, remove push flags
		mode.EnvChanges["DOCKER_PUSH"] = "false"

	case BehaviorProduction:
		mode.Mode = BehaviorProduction
		mode.Reason = "Local mode: production (use with caution!)"

	default:
		return nil, fmt.Errorf("unknown local_behavior '%s' for preset '%s'", localBehavior, preset.Name)
	}

	return mode, nil
}

// ApplyExecutionMode applies the execution mode to a preset
func ApplyExecutionMode(preset presets.Preset, mode *ExecutionMode) presets.Preset {
	modified := preset

	// Apply environment variable changes
	if modified.Env == nil {
		modified.Env = make(map[string]string)
	}
	for key, value := range mode.EnvChanges {
		modified.Env[key] = value
	}

	// Modify command based on mode
	switch mode.Mode {
	case BehaviorDraft:
		// Add --draft flag for GitHub CLI
		if preset.Name == "gh-release" {
			modified.Command = modified.Command + " --draft"
		}

	case BehaviorNoPush:
		// Remove --push flag for Docker
		if preset.Name == "docker-buildx" {
			// Remove --push from command
			modified.Command = removeFlag(modified.Command, "--push")
		}
	}

	return modified
}

// removeFlag removes a flag from a command string
func removeFlag(command, flag string) string {
	// Simple implementation - can be improved
	result := command
	// Remove flag and its value if present
	for _, variant := range []string{flag + " ", flag} {
		result = replaceAll(result, variant, "")
	}
	return result
}

// replaceAll is a helper to replace all occurrences
func replaceAll(s, old, new string) string {
	result := s
	for {
		before := result
		result = replace(result, old, new)
		if result == before {
			break
		}
	}
	return result
}

// replace replaces first occurrence
func replace(s, old, new string) string {
	if old == "" {
		return s
	}
	idx := indexOf(s, old)
	if idx == -1 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

// indexOf finds index of substring
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
