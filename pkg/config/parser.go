package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// Load loads configuration from a TOML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse TOML into generic map
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse TOML config: %w", err)
	}

	// Separate phases, pipelines, and overrides
	cfg := &Config{
		Phases:    make(map[string]Phase),
		Pipelines: make(map[string]Pipeline),
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

		// Check if this is the "pipelines" section
		if name == "pipelines" {
			for pipelineName, pipelineValue := range section {
				pipelineMap, ok := pipelineValue.(map[string]interface{})
				if !ok {
					continue
				}

				// Parse phases array
				if phasesRaw, hasPhases := pipelineMap["phases"]; hasPhases {
					phases := []string{}
					switch p := phasesRaw.(type) {
					case []interface{}:
						for _, phase := range p {
							if phaseStr, ok := phase.(string); ok {
								phases = append(phases, phaseStr)
							}
						}
					case []string:
						phases = p
					}
					cfg.Pipelines[pipelineName] = Pipeline{Phases: phases}
				}
			}
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
