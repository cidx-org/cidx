package presets

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

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
	Description   string                `toml:"description"`
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
	PullPolicy    string                `toml:"pull_policy"`
	Timeout       string                `toml:"timeout"`
}

// OptionTOML represents an option in TOML format.
// No `default` key: see the Option doc comment (#299). A residual one in a
// user preset file is reported by warnUndecodedKeys rather than ignored.
type OptionTOML struct {
	Type        string `toml:"type"`
	Description string `toml:"description"`
	CommandFlag string `toml:"command_flag"`
	EnvVar      string `toml:"env_var"`
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
		mergeOptionalPresetFile(registry, filepath.Join(homeDir, ".config", "cidx", "presets.toml"))
	}

	// 3. Load Project Presets (.cidx/presets.toml)
	// We assume current working directory is project root
	if cwd, err := os.Getwd(); err == nil {
		mergeOptionalPresetFile(registry, filepath.Join(cwd, ".cidx", "presets.toml"))
	}

	return registry, nil
}

// mergeOptionalPresetFile merges a preset file into registry if it exists.
//
// A file that fails to parse used to be dropped whole and in silence: every
// custom preset in it vanished, and the user only met "container X is not a
// built-in preset" much later, pointing at the wrong thing (#210). Say so.
//
// Warning rather than hard error, including for the project file: loadPresets
// runs from this package's init(), whose only failure path is log.Fatalf. A
// returned error would abort every cidx invocation before the CLI starts —
// including `cidx doctor` and `--help`, the commands that would tell the user
// what is broken. A named warning is observable and leaves them in control.
func mergeOptionalPresetFile(registry map[string]Preset, path string) {
	if _, err := os.Stat(path); err != nil {
		return
	}
	filePresets, err := loadPresetsFromFile(path)
	if err != nil {
		// Both failure modes already name the file: os.ReadFile returns a
		// *PathError, parsePresetsData embeds its source.
		log.Printf("warning: ignoring preset file: %v", err)
		return
	}
	mergePresets(registry, filePresets)
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
	md, err := toml.Decode(string(data), &presetsFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse presets (%s): %w", source, err)
	}

	// A key that PresetTOML does not know about is dropped by the decoder
	// without a word — that is how pull_policy and timeout went missing (#203),
	// and it is how any typo ("comand", "workdirr") goes missing today. Warn
	// instead of failing: an unknown key costs one field, whereas an error here
	// costs the whole file (mergeOptionalPresetFile drops a file it cannot parse).
	warnUndecodedKeys(md.Undecoded(), source)

	registry := make(map[string]Preset)
	for name, tomlPreset := range presetsFile.Presets {
		preset := Preset{
			Name:          tomlPreset.Name,
			Phase:         tomlPreset.Phase,
			Image:         tomlPreset.Image,
			Description:   tomlPreset.Description,
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
			PullPolicy:    tomlPreset.PullPolicy,
			Timeout:       tomlPreset.Timeout,
		}

		// Convert options
		for optName, tomlOpt := range tomlPreset.Options {
			option := Option{
				Type:        tomlOpt.Type,
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

// warnUndecodedKeys reports TOML keys that no field of PresetTOML claims, so a
// misspelled or unsupported key surfaces instead of vanishing. Keys are sorted
// for a stable message.
func warnUndecodedKeys(keys []toml.Key, source string) {
	if len(keys) == 0 {
		return
	}
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, k.String())
	}
	sort.Strings(names)
	log.Printf("warning: ignoring unknown preset key(s) in %s: %s", source, strings.Join(names, ", "))
}

// mergePresets merges override presets into registry
func mergePresets(registry map[string]Preset, overrides map[string]Preset) {
	for name, preset := range overrides {
		// Replace/Add preset
		registry[name] = preset
	}
}
