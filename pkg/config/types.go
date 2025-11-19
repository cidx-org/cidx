package config

// Config represents the complete CIDX configuration
type Config struct {
	Phases    map[string]Phase                  `toml:",inline"`
	Pipelines map[string]Pipeline               `toml:"pipelines"`
	Overrides map[string]map[string]interface{} `toml:",inline"`
	Workspace string                            // Auto-detected or from env
}

// Phase defines tools for a specific phase
type Phase struct {
	Tools []string `toml:"tools"`
}

// Pipeline defines a sequence of phases to execute
type Pipeline struct {
	Phases []string `toml:"phases"`
}

// ToolConfig represents a fully resolved tool configuration after merging preset + overrides
type ToolConfig struct {
	Name        string
	Phase       string
	Image       string
	Command     string
	Workdir     string
	Volumes     []string
	Env         map[string]string
	ConfigFiles []string
	Privileged  bool // Requires root privileges
}
