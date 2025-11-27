package config

// Config represents the complete CIDX configuration
type Config struct {
	Phases    map[string]Phase                  `toml:",inline"`
	Pipelines map[string]Pipeline               `toml:"pipelines"`
	Actions   map[string]Action                 `toml:"actions"`
	Overrides map[string]map[string]interface{} `toml:",inline"`
	Workspace string                            // Auto-detected or from env
}

// Phase defines containers for a specific phase
type Phase struct {
	Containers []string `toml:"containers"`
}

// Pipeline defines a sequence of phases to execute
type Pipeline struct {
	Phases []string `toml:"phases"`
}

// ContainerConfig represents a fully resolved container configuration after merging preset + overrides
type ContainerConfig struct {
	Name        string
	Phase       string
	Image       string
	Command     string
	Entrypoint  []string          // Override container entrypoint
	Workdir     string
	Volumes     []string
	Env         map[string]string
	ConfigFiles []string
	Privileged  bool // Requires root privileges
}

// Action represents an automated workflow configuration
type Action struct {
	Description   string            `toml:"description"`
	Image         string            `toml:"image"`
	Command       string            `toml:"command"`
	Entrypoint    []string          `toml:"entrypoint"` // Override container entrypoint
	Workdir       string            `toml:"workdir"`
	Volumes       []string          `toml:"volumes"`
	Env           map[string]string `toml:"env"`
	AutoPush      bool              `toml:"auto_push"`
	PushTags      bool              `toml:"push_tags"`
	WatchWorkflow bool              `toml:"watch_workflow"`
}
