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

	// Separate phases, pipelines, actions, and overrides
	cfg := &Config{
		Phases:    make(map[string]Phase),
		Pipelines: make(map[string]Pipeline),
		Actions:   make(map[string]Action),
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

		// Check if this is the "actions" section
		if name == "actions" {
			for actionName, actionValue := range section {
				actionMap, ok := actionValue.(map[string]interface{})
				if !ok {
					continue
				}

				action := Action{}

				if desc, ok := actionMap["description"].(string); ok {
					action.Description = desc
				}
				if img, ok := actionMap["image"].(string); ok {
					action.Image = img
				}
				if cmd, ok := actionMap["command"].(string); ok {
					action.Command = cmd
				}
				if wd, ok := actionMap["workdir"].(string); ok {
					action.Workdir = wd
				}
				if autoPush, ok := actionMap["auto_push"].(bool); ok {
					action.AutoPush = autoPush
				}
				if pushTags, ok := actionMap["push_tags"].(bool); ok {
					action.PushTags = pushTags
				}
				if watchWf, ok := actionMap["watch_workflow"].(bool); ok {
					action.WatchWorkflow = watchWf
				}

				// Parse volumes array
				if volsRaw, hasVols := actionMap["volumes"]; hasVols {
					switch v := volsRaw.(type) {
					case []interface{}:
						for _, vol := range v {
							if volStr, ok := vol.(string); ok {
								action.Volumes = append(action.Volumes, volStr)
							}
						}
					case []string:
						action.Volumes = v
					}
				}

				// Parse env map
				if envRaw, hasEnv := actionMap["env"]; hasEnv {
					if envMap, ok := envRaw.(map[string]interface{}); ok {
						action.Env = make(map[string]string)
						for k, v := range envMap {
							if vStr, ok := v.(string); ok {
								action.Env[k] = vStr
							}
						}
					}
				}

				cfg.Actions[actionName] = action
			}
			continue
		}

		// Check if this section has a "containers" key → it's a phase
		if containersRaw, hasContainers := section["containers"]; hasContainers {
			containers := []string{}
			switch t := containersRaw.(type) {
			case []interface{}:
				for _, container := range t {
					if containerStr, ok := container.(string); ok {
						containers = append(containers, containerStr)
					}
				}
			case []string:
				containers = t
			}
			cfg.Phases[name] = Phase{Containers: containers}
		} else {
			// It's a container override
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
