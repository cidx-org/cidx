package presets

import (
	_ "embed"
	"fmt"
	"os"

	"path/filepath"

	"github.com/BurntSushi/toml"
)

//go:embed presets.toml
var embeddedPresets []byte

// PresetsFile represents the structure of presets.toml
type PresetsFile struct {
	Presets map[string]PresetTOML `toml:"presets"`
}

// PresetTOML represents a preset in TOML format
type PresetTOML struct {
	Name          string                `toml:"name"`
	Phase         string                `toml:"phase"`
	Image         string                `toml:"image"`
	Hardened      bool                  `toml:"hardened"`
	Command       string                `toml:"command"`
	Entrypoint    []string              `toml:"entrypoint"`
	Workdir       string                `toml:"workdir"`
	Volumes       []string              `toml:"volumes"`
	Env           map[string]string     `toml:"env"`
	ConfigFiles   []string              `toml:"config_files"`
	Options       map[string]OptionTOML `toml:"options"`
	RequireCI     bool                  `toml:"require_ci"`
	LocalBehavior string                `toml:"local_behavior"`
	Privileged    bool                  `toml:"privileged"`
}

// OptionTOML represents an option in TOML format
type OptionTOML struct {
	Type        string      `toml:"type"`
	Default     interface{} `toml:"default"`
	Description string      `toml:"description"`
	CommandFlag string      `toml:"command_flag"`
	EnvVar      string      `toml:"env_var"`
}

// loadPresets loads presets from file (dev) or embedded data (production)
// It also loads user presets (~/.config/cidx/presets.toml) and project presets (.cidx/presets.toml)
func loadPresets() (map[string]Preset, error) {
	registry := make(map[string]Preset)

	// 1. Load Base Presets (Embedded or Dev File)
	basePresets, err := loadBasePresets()
	if err != nil {
		return nil, err
	}
	mergePresets(registry, basePresets)

	// 2. Load User Presets (~/.config/cidx/presets.toml)
	if homeDir, err := os.UserHomeDir(); err == nil {
		userPath := filepath.Join(homeDir, ".config", "cidx", "presets.toml")
		if _, err := os.Stat(userPath); err == nil {
			userPresets, err := loadPresetsFromFile(userPath)
			if err == nil {
				mergePresets(registry, userPresets)
			}
		}
	}

	// 3. Load Project Presets (.cidx/presets.toml)
	// We assume current working directory is project root
	if cwd, err := os.Getwd(); err == nil {
		projectPath := filepath.Join(cwd, ".cidx", "presets.toml")
		if _, err := os.Stat(projectPath); err == nil {
			projectPresets, err := loadPresetsFromFile(projectPath)
			if err == nil {
				mergePresets(registry, projectPresets)
			}
		}
	}

	return registry, nil
}

// loadBasePresets loads the core presets
func loadBasePresets() (map[string]Preset, error) {
	var data []byte
	var source string

	// Try loading from file first (development mode)
	paths := []string{
		"pkg/presets/presets.toml",
		"presets.toml",
	}

	for _, path := range paths {
		if fileData, err := os.ReadFile(path); err == nil {
			data = fileData
			source = fmt.Sprintf("file: %s", path)
			break
		}
	}

	// Fallback to embedded presets (production mode)
	if data == nil {
		data = embeddedPresets
		source = "embedded"
	}

	return parsePresetsData(data, source)
}

// loadPresetsFromFile loads presets from a specific file path
func loadPresetsFromFile(path string) (map[string]Preset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parsePresetsData(data, path)
}

// parsePresetsData parses raw TOML data into Presets map
func parsePresetsData(data []byte, source string) (map[string]Preset, error) {
	var presetsFile PresetsFile
	if err := toml.Unmarshal(data, &presetsFile); err != nil {
		return nil, fmt.Errorf("failed to parse presets (%s): %w", source, err)
	}

	registry := make(map[string]Preset)
	for name, tomlPreset := range presetsFile.Presets {
		preset := Preset{
			Name:          tomlPreset.Name,
			Phase:         tomlPreset.Phase,
			Image:         tomlPreset.Image,
			Hardened:      tomlPreset.Hardened,
			Command:       tomlPreset.Command,
			Entrypoint:    tomlPreset.Entrypoint,
			Workdir:       tomlPreset.Workdir,
			Volumes:       tomlPreset.Volumes,
			Env:           tomlPreset.Env,
			ConfigFiles:   tomlPreset.ConfigFiles,
			Options:       make(map[string]Option),
			RequireCI:     tomlPreset.RequireCI,
			LocalBehavior: tomlPreset.LocalBehavior,
			Privileged:    tomlPreset.Privileged,
		}

		// Convert options
		for optName, tomlOpt := range tomlPreset.Options {
			option := Option{
				Type:        tomlOpt.Type,
				Default:     tomlOpt.Default,
				Description: tomlOpt.Description,
				CommandFlag: tomlOpt.CommandFlag,
				EnvVar:      tomlOpt.EnvVar,
			}
			preset.Options[optName] = option
		}

		registry[name] = preset
	}

	return registry, nil
}

// mergePresets merges override presets into registry
func mergePresets(registry map[string]Preset, overrides map[string]Preset) {
	for name, preset := range overrides {
		// Replace/Add preset
		registry[name] = preset
	}
}
