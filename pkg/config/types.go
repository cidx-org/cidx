package config

// Config represents the complete CIDX configuration
type Config struct {
	Phases    map[string]Phase                  `toml:",inline"`
	Pipelines map[string]Pipeline               `toml:"pipelines"`
	Actions   map[string]Action                 `toml:"actions"`
	Branch    BranchConfig                      `toml:"branch"`
	Release   ReleaseConfig                     `toml:"release"`
	Tag       TagConfig                         `toml:"tag"`
	Overrides map[string]map[string]interface{} `toml:",inline"`
	Workspace string                            // Auto-detected or from env
}

// BranchConfig defines branch management settings
type BranchConfig struct {
	StaleDays     int      `toml:"stale_days"`      // Days before a branch is considered stale (default: 30)
	NamingPattern string   `toml:"naming_pattern"`  // Regex pattern for valid branch names
	AutoCleanup   bool     `toml:"auto_cleanup"`    // Cleanup merged branches after PR merge
	Protected     []string `toml:"protected"`       // Branches that should never be deleted
}

// ReleaseConfig defines release workflow settings
type ReleaseConfig struct {
	// MainBranch is the branch where releases are created (default: "main")
	MainBranch string `toml:"main_branch"`

	// AllowReleaseFromAnyBranch allows creating releases from any branch (default: false)
	// When false, releases can only be created from MainBranch
	AllowReleaseFromAnyBranch bool `toml:"allow_release_from_any_branch"`

	// RequirePrepare requires running 'release prepare' before 'release create' (default: false)
	// When true, releases cannot be created without prepared notes
	RequirePrepare bool `toml:"require_prepare"`

	// AutoCleanup automatically removes .cidx/release-* files after successful release (default: true)
	AutoCleanup bool `toml:"auto_cleanup"`

	// Editor is the command to open release notes for editing (default: $EDITOR or "vim")
	Editor string `toml:"editor"`

	// NotesTemplate is a custom template for release notes (optional)
	NotesTemplate string `toml:"notes_template"`
}

// DefaultReleaseConfig returns a ReleaseConfig with sensible defaults
func DefaultReleaseConfig() ReleaseConfig {
	return ReleaseConfig{
		MainBranch:                "main",
		AllowReleaseFromAnyBranch: false,
		RequirePrepare:            false,
		AutoCleanup:               true,
		Editor:                    "", // Will fall back to $EDITOR or "vim"
		NotesTemplate:             "",
	}
}

// GetMainBranch returns the main branch name with fallback to "main"
func (r *ReleaseConfig) GetMainBranch() string {
	if r.MainBranch == "" {
		return "main"
	}
	return r.MainBranch
}

// GetEditor returns the editor command with fallback to $EDITOR or "vim"
func (r *ReleaseConfig) GetEditor() string {
	if r.Editor != "" {
		return r.Editor
	}
	// Check $EDITOR env var at runtime (in the action)
	return ""
}

// TagConfig defines tag workflow settings
type TagConfig struct {
	// Prefix for version tags (default: "v")
	// Examples: "v" → v1.2.3, "" → 1.2.3, "release-" → release-1.2.3
	Prefix string `toml:"prefix"`

	// Pattern is a regex pattern for valid tag names (optional)
	// Default allows semver with optional prefix: ^v?\d+\.\d+\.\d+.*$
	Pattern string `toml:"pattern"`

	// UseCommitizen uses commitizen to determine next version (default: true)
	// When true: runs "cz bump --dry-run" to get suggested version
	// When false: increments patch version from last tag
	UseCommitizen bool `toml:"use_commitizen"`

	// AutoPush automatically pushes tags after creation (default: true)
	AutoPush bool `toml:"auto_push"`

	// SignTags signs tags with GPG (default: false)
	SignTags bool `toml:"sign_tags"`

	// RequireAnnotated requires annotated tags with message (default: true)
	// When true: creates annotated tags with -a -m flags
	// When false: allows lightweight tags
	RequireAnnotated bool `toml:"require_annotated"`

	// ProtectedTags is a list of tag patterns that cannot be deleted
	// Supports glob patterns: ["v1.*", "release-*"]
	ProtectedTags []string `toml:"protected_tags"`

	// LinkedToRelease indicates if tags trigger release workflow (default: true)
	// When true: pushing a tag triggers the release pipeline
	LinkedToRelease bool `toml:"linked_to_release"`
}

// DefaultTagConfig returns a TagConfig with sensible defaults
func DefaultTagConfig() TagConfig {
	return TagConfig{
		Prefix:           "v",
		Pattern:          "", // Default pattern applied in validation
		UseCommitizen:    true,
		AutoPush:         true,
		SignTags:         false,
		RequireAnnotated: true,
		ProtectedTags:    []string{},
		LinkedToRelease:  true,
	}
}

// GetPrefix returns the tag prefix with default "v"
func (t *TagConfig) GetPrefix() string {
	// Empty string is valid (no prefix), only use default if not explicitly set
	return t.Prefix
}

// FormatTag formats a version with the configured prefix
func (t *TagConfig) FormatTag(version string) string {
	return t.Prefix + version
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
