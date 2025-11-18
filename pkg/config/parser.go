package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// Load loads configuration from a file (auto-detects YAML or TOML)
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse into generic map first
	var raw map[string]interface{}
	ext := filepath.Ext(path)

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse YAML config: %w", err)
		}
	case ".toml":
		if err := toml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse TOML config: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported config format: %s (use .yaml, .yml, or .toml)", ext)
	}

	// Separate phases from overrides
	cfg := &Config{
		Phases:    make(map[string]Phase),
		Overrides: make(map[string]map[string]interface{}),
		Workspace: os.Getenv("PWD"),
	}

	if cfg.Workspace == "" {
		cfg.Workspace, _ = os.Getwd()
	}

	for name, value := range raw {
		section, ok := value.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this section has a "tools" key → it's a phase
		if toolsRaw, hasTools := section["tools"]; hasTools {
			tools := []string{}
			switch t := toolsRaw.(type) {
			case []interface{}:
				for _, tool := range t {
					if toolStr, ok := tool.(string); ok {
						tools = append(tools, toolStr)
					}
				}
			case []string:
				tools = t
			}
			cfg.Phases[name] = Phase{Tools: tools}
		} else {
			// It's a tool override
			cfg.Overrides[name] = section
		}
	}

	return cfg, nil
}

// FindConfig searches for cidx config files in common locations
func FindConfig() (string, error) {
	candidates := []string{
		"cidx.toml",
		"cidx.yaml",
		"cidx.yml",
		".cidx.toml",
		".cidx.yaml",
		".cidx.yml",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no cidx config file found (tried: %s)", strings.Join(candidates, ", "))
}
