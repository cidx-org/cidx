package config

// Config represents the complete CIDX configuration
type Config struct {
	Phases    map[string]Phase                  `yaml:",inline" toml:",inline"`
	Overrides map[string]map[string]interface{} `yaml:",inline" toml:",inline"`
	Workspace string                            // Auto-detected or from env
}

// Phase defines tools for a specific phase
type Phase struct {
	Tools []string `yaml:"tools" toml:"tools"`
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
}
