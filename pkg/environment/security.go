package environment

import (
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// removedBehaviorNoPush is not a mode, it is a value CIDX still recognises in
// order to refuse it by name. A config carrying it was written against a mode
// that behaved identically to dry-run, so failing with "unknown local_behavior"
// would leave the reader hunting for a difference that never existed (#353).
const removedBehaviorNoPush = "no-push"

// LocalBehavior defines how a preset behaves in local environment
const (
	BehaviorProduction = "production" // Full execution (dangerous in local)
	BehaviorDraft      = "draft"      // Preview only; creates no release
	BehaviorDryRun     = "dry-run"    // Dry run only
	BehaviorDisabled   = "disabled"   // Completely disabled in local
)

// ExecutionMode determines how a preset should be executed
type ExecutionMode struct {
	Allowed    bool              // Can this preset run in current environment?
	Mode       string            // Execution mode (production, draft, dry-run)
	Reason     string            // Why this mode was chosen
	IsDryRun   bool              // Force dry-run mode
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
		mode.Reason = "Local mode: draft preview only (no release created)"
		// For GitHub releases, force draft mode
		mode.EnvChanges["DRAFT"] = "true"

	case removedBehaviorNoPush:
		// Removed in v3.0.0 (issue #353). It set IsDryRun exactly as dry-run
		// did, so nothing was ever built under it; what it added on top -- a
		// DOCKER_PUSH=false on a container that never starts, a --push stripped
		// from a command that is only printed -- was two ways of doctoring a
		// run that does not happen. The documentation promised it "validates
		// Dockerfile and build process", which is the one thing it could not do.
		return nil, fmt.Errorf("preset '%s' declares local_behavior 'no-push', which was removed in cidx v3.0.0 -- use 'dry-run', which is what it did", preset.Name)

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
	}

	return modified
}
