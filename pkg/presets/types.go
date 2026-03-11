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
	RequireCI     bool              `yaml:"require_ci" toml:"require_ci"`                     // Requires CI environment
	LocalBehavior string            `yaml:"local_behavior" toml:"local_behavior"`             // draft, no-push, dry-run, disabled
	Privileged    bool              `yaml:"privileged,omitempty" toml:"privileged,omitempty"` // Requires root privileges (skip user mapping)
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
	if entrypoint, ok := overrides["entrypoint"].([]string); ok {
		merged.Entrypoint = entrypoint
	}
	if workdir, ok := overrides["workdir"].(string); ok {
		merged.Workdir = workdir
	}
	if volumes, ok := overrides["volumes"].([]string); ok {
		merged.Volumes = volumes
	}
	if env, ok := overrides["env"].(map[string]string); ok {
		if merged.Env == nil {
			merged.Env = make(map[string]string)
		}
		for k, v := range env {
			merged.Env[k] = v
		}
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
