package presets

import (
	"fmt"
	"strconv"
)

// Preset defines a complete tool configuration with sensible defaults
type Preset struct {
	Name          string            `yaml:"name" toml:"name"`
	Phase         string            `yaml:"phase" toml:"phase"`
	Image         string            `yaml:"image" toml:"image"`
	Hardened      bool              `yaml:"hardened,omitempty" toml:"hardened,omitempty"` // Uses Docker Hardened Image (dhi.io)
	Command       string            `yaml:"command" toml:"command"`
	Entrypoint    []string          `yaml:"entrypoint" toml:"entrypoint"`
	Workdir       string            `yaml:"workdir" toml:"workdir"`
	Volumes       []string          `yaml:"volumes" toml:"volumes"`
	Env           map[string]string `yaml:"env" toml:"env"`
	ConfigFiles   []string          `yaml:"config_files" toml:"config_files"`
	Options       map[string]Option `yaml:"options" toml:"options"`
	RequireCI     bool              `yaml:"require_ci" toml:"require_ci"`                       // Requires CI environment
	LocalBehavior string            `yaml:"local_behavior" toml:"local_behavior"`               // draft, no-push, dry-run, disabled
	Privileged    bool              `yaml:"privileged,omitempty" toml:"privileged,omitempty"`   // Requires root privileges (skip user mapping)
	PullPolicy    string            `yaml:"pull_policy,omitempty" toml:"pull_policy,omitempty"` // always, if-not-present, never (default: env-based)
	Timeout       string            `yaml:"timeout,omitempty" toml:"timeout,omitempty"`         // duration string (e.g., "5m", "45m"), default: 30m
}

// Option defines a configurable parameter for a preset
type Option struct {
	Type        string `yaml:"type" toml:"type"`                 // string, bool, int, array
	Default     any    `yaml:"default" toml:"default"`           // Default value
	Description string `yaml:"description" toml:"description"`   // Help text
	EnvVar      string `yaml:"env_var" toml:"env_var"`           // Maps to environment variable
	CommandFlag string `yaml:"command_flag" toml:"command_flag"` // Maps to command flag
}

// MergeWith merges user overrides into the preset
func (p *Preset) MergeWith(overrides map[string]any) *Preset {
	merged := *p

	if image, ok := overrides["image"].(string); ok {
		merged.Image = image
	}
	if command, ok := overrides["command"].(string); ok {
		merged.Command = command
	}
	// Entrypoint and volumes arrive as []any when decoded from TOML through
	// a map[string]any pass (which is how cidx.toml [containers.X] sections
	// reach this function). The previous []string type assertion silently
	// dropped them — see #143. Accept both shapes via normalizeStringSlice.
	if entrypointRaw, hasEntrypoint := overrides["entrypoint"]; hasEntrypoint {
		if entrypoint, ok := normalizeStringSliceOverride(entrypointRaw); ok {
			merged.Entrypoint = entrypoint
		}
	}
	if workdir, ok := overrides["workdir"].(string); ok {
		merged.Workdir = workdir
	}
	if volumesRaw, hasVolumes := overrides["volumes"]; hasVolumes {
		if volumes, ok := normalizeStringSliceOverride(volumesRaw); ok {
			merged.Volumes = volumes
		}
	}
	// Merge per-key env overrides from cidx.toml.
	// TOML decoding through map[string]any yields map[string]any for nested
	// inline tables (e.g. `env = { FOO = "bar" }`), so we must accept both
	// shapes. Per-key, the user's value overrides the preset's on collision;
	// preset keys not mentioned by the user are preserved (closes #123).
	if envRaw, hasEnv := overrides["env"]; hasEnv {
		userEnv, ok := normalizeEnvOverride(envRaw)
		if ok {
			// Copy preset env first so we don't mutate the original map
			// (Preset is shallow-copied above; the Env map is shared).
			newEnv := make(map[string]string, len(merged.Env)+len(userEnv))
			for k, v := range merged.Env {
				newEnv[k] = v
			}
			for k, v := range userEnv {
				newEnv[k] = v
			}
			merged.Env = newEnv
		}
	}

	if pullPolicy, ok := overrides["pull_policy"].(string); ok {
		merged.PullPolicy = pullPolicy
	}
	if timeout, ok := overrides["timeout"].(string); ok {
		merged.Timeout = timeout
	}

	// Merge options with preset options
	for optName, optValue := range overrides {
		if opt, exists := merged.Options[optName]; exists {
			merged = applyOption(&merged, optName, opt, optValue)
		}
	}

	return &merged
}

// applyOption applies a specific option value to the preset
func applyOption(preset *Preset, name string, opt Option, value any) Preset {
	p := *preset

	// Apply to environment variable if specified
	if opt.EnvVar != "" {
		if p.Env == nil {
			p.Env = make(map[string]string)
		}
		p.Env[opt.EnvVar] = toString(value)
	}

	// Apply to command flag if specified
	if opt.CommandFlag != "" {
		p.Command = p.Command + " " + opt.CommandFlag + " " + toString(value)
	}

	return p
}

// normalizeStringSliceOverride coerces a TOML-decoded array value into []string.
// TOML decoding through map[string]any yields []any for arrays, not []string,
// so we accept both shapes. Non-string elements are skipped silently; if any
// element is unconvertible the whole slice is rejected (returns false) so the
// caller can preserve the preset's default rather than emit a half-built list.
func normalizeStringSliceOverride(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, 0, len(s))
		for _, item := range s {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	default:
		return nil, false
	}
}

// PresetFromOverrides constructs a Preset from a custom container declaration
// in cidx.toml. A declaration is a `[containers.NAME]` section that has an
// `image` field present — that signals a brand-new container, not an override
// of a known preset. The returned Preset is filled from the overrides map;
// fields absent from the map keep their zero value.
//
// This implements the user-facing contract documented in
// examples/cidx-complete.toml (custom containers section) and closes #142.
func PresetFromOverrides(name string, overrides map[string]any) Preset {
	p := Preset{Name: name}

	if phase, ok := overrides["phase"].(string); ok {
		p.Phase = phase
	}
	if image, ok := overrides["image"].(string); ok {
		p.Image = image
	}
	if command, ok := overrides["command"].(string); ok {
		p.Command = command
	}
	if entrypointRaw, ok := overrides["entrypoint"]; ok {
		if entrypoint, ok := normalizeStringSliceOverride(entrypointRaw); ok {
			p.Entrypoint = entrypoint
		}
	}
	if workdir, ok := overrides["workdir"].(string); ok {
		p.Workdir = workdir
	}
	if volumesRaw, ok := overrides["volumes"]; ok {
		if volumes, ok := normalizeStringSliceOverride(volumesRaw); ok {
			p.Volumes = volumes
		}
	}
	if envRaw, ok := overrides["env"]; ok {
		if env, ok := normalizeEnvOverride(envRaw); ok {
			p.Env = env
		}
	}
	if privileged, ok := overrides["privileged"].(bool); ok {
		p.Privileged = privileged
	}
	if pullPolicy, ok := overrides["pull_policy"].(string); ok {
		p.PullPolicy = pullPolicy
	}
	if timeout, ok := overrides["timeout"].(string); ok {
		p.Timeout = timeout
	}

	return p
}

// IsCustomDeclaration reports whether the given override section declares a
// brand-new container (image field present) rather than overriding an
// existing preset. Used by validator and runner to decide between
// preset-override semantics and custom-container semantics.
func IsCustomDeclaration(overrides map[string]any) bool {
	if overrides == nil {
		return false
	}
	image, ok := overrides["image"].(string)
	return ok && image != ""
}

// normalizeEnvOverride coerces an env override value into map[string]string.
// Accepts the in-process map[string]string form and the TOML-decoded
// map[string]any form (where each value is itself converted via toString).
// Returns (nil, false) for any other shape so callers can ignore it safely.
func normalizeEnvOverride(v any) (map[string]string, bool) {
	switch env := v.(type) {
	case map[string]string:
		return env, true
	case map[string]any:
		out := make(map[string]string, len(env))
		for k, val := range env {
			// Only coerce scalar values; reject nested tables / arrays so a
			// malformed user config surfaces upstream rather than producing
			// "<nil>" strings silently.
			switch val.(type) {
			case string, int, int64, bool, float64:
				out[k] = toString(val)
			default:
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}

// toString converts interface{} to string
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}
