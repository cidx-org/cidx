package presets

import (
	_ "embed"
	"fmt"
	"os"

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
	Command       string                `toml:"command"`
	Workdir       string                `toml:"workdir"`
	Volumes       []string              `toml:"volumes"`
	Env           map[string]string     `toml:"env"`
	ConfigFiles   []string              `toml:"config_files"`
	Options       map[string]OptionTOML `toml:"options"`
	RequireCI     bool                  `toml:"require_ci"`
	LocalBehavior string                `toml:"local_behavior"`
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
func loadPresets() (map[string]Preset, error) {
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

	// Parse TOML
	var presetsFile PresetsFile
	if err := toml.Unmarshal(data, &presetsFile); err != nil {
		return nil, fmt.Errorf("failed to parse presets (%s): %w", source, err)
	}

	// Convert TOML presets to Preset structs
	registry := make(map[string]Preset)
	for name, tomlPreset := range presetsFile.Presets {
		preset := Preset{
			Name:          tomlPreset.Name,
			Phase:         tomlPreset.Phase,
			Image:         tomlPreset.Image,
			Command:       tomlPreset.Command,
			Workdir:       tomlPreset.Workdir,
			Volumes:       tomlPreset.Volumes,
			Env:           tomlPreset.Env,
			ConfigFiles:   tomlPreset.ConfigFiles,
			Options:       make(map[string]Option),
			RequireCI:     tomlPreset.RequireCI,
			LocalBehavior: tomlPreset.LocalBehavior,
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
