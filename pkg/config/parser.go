package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// knownSections lists top-level TOML sections that are decoded into typed fields.
// Any other section with a "containers" key is treated as a phase;
// remaining sections are container overrides.
var knownSections = map[string]bool{
	"tag_workflow":     true,
	"release_workflow": true,
	"pr":               true,
	"branch":           true,
	"provider":         true,
	"pipelines":        true,
	"actions":          true,
	"required_version": true,
}

// typedConfig is the intermediate struct for decoding known TOML sections.
// Fields are pre-initialized with defaults before decoding, so TOML only
// overwrites explicitly set values (preserving e.g. PRConfig boolean defaults).
type typedConfig struct {
	RequiredVersion string              `toml:"required_version"`
	TagWorkflow     TagConfig           `toml:"tag_workflow"`
	ReleaseWorkflow ReleaseConfig       `toml:"release_workflow"`
	PR              PRConfig            `toml:"pr"`
	Branch          BranchConfig        `toml:"branch"`
	Provider        ProviderConfig      `toml:"provider"`
	Pipelines       map[string]Pipeline `toml:"pipelines"`
	Actions         map[string]Action   `toml:"actions"`
}

// Load loads configuration from a TOML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Pass 1: decode known sections into typed struct (with defaults pre-set)
	typed := typedConfig{
		TagWorkflow:     DefaultTagConfig(),
		ReleaseWorkflow: DefaultReleaseConfig(),
		PR:              DefaultPRConfig(),
	}
	if err := toml.Unmarshal(data, &typed); err != nil {
		return nil, fmt.Errorf("failed to parse TOML config: %w", err)
	}

	// Pass 2: decode into raw map for dynamic sections (phases + container overrides)
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse TOML config: %w", err)
	}

	cfg := &Config{
		RequiredVersion: typed.RequiredVersion,
		Phases:          make(map[string]Phase),
		Pipelines:       typed.Pipelines,
		Actions:         typed.Actions,
		Overrides:       make(map[string]map[string]any),
		Release:         typed.ReleaseWorkflow,
		Tag:             typed.TagWorkflow,
		PR:              typed.PR,
		Branch:          typed.Branch,
		Provider:        typed.Provider,
		Workspace:       os.Getenv("PWD"),
	}

	if cfg.Pipelines == nil {
		cfg.Pipelines = make(map[string]Pipeline)
	}
	if cfg.Actions == nil {
		cfg.Actions = make(map[string]Action)
	}
	if cfg.Workspace == "" {
		cfg.Workspace, _ = os.Getwd()
	}

	// Process nested [containers.<name>] override sections first
	if containersRaw, hasContainers := raw["containers"]; hasContainers {
		if containersMap, ok := containersRaw.(map[string]any); ok {
			for name, value := range containersMap {
				section, ok := value.(map[string]any)
				if !ok {
					continue
				}
				cfg.Overrides[name] = section
			}
		}
	}

	// Process dynamic sections: phases (have "containers" key) vs legacy top-level container overrides
	for name, value := range raw {
		if name == "containers" {
			continue
		}

		if knownSections[name] {
			continue
		}

		section, ok := value.(map[string]any)
		if !ok {
			continue
		}

		if containersRaw, hasContainers := section["containers"]; hasContainers {
			cfg.Phases[name] = Phase{Containers: toStringSlice(containersRaw)}
		} else {
			cfg.Overrides[name] = section
		}
	}

	return cfg, nil
}

// toStringSlice converts an any (typically []any from TOML) to []string
func toStringSlice(v any) []string {
	switch t := v.(type) {
	case []any:
		result := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return t
	default:
		return nil
	}
}

// FindConfig searches for cidx TOML config files in common locations
func FindConfig() (string, error) {
	candidates := []string{
		"cidx.toml",
		".cidx.toml",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no cidx config file found (tried: %s)", strings.Join(candidates, ", "))
}
