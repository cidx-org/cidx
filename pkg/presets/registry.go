package presets

import (
	"fmt"
	"log"
)

// GlobalRegistry contains all built-in presets
// Loaded from presets.yaml (dev) or embedded data (production)
var GlobalRegistry map[string]Preset

func init() {
	var err error
	GlobalRegistry, err = loadPresets()
	if err != nil {
		log.Fatalf("Failed to load presets: %v", err)
	}
}

// Get retrieves a preset by name
func Get(name string) (Preset, error) {
	preset, exists := GlobalRegistry[name]
	if !exists {
		return Preset{}, fmt.Errorf("preset '%s' not found", name)
	}
	return preset, nil
}

// Catalogue returns the built-in preset catalogue alone: what presets.toml
// ships, without the user's (~/.config/cidx) or the project's (.cidx/) own
// presets merged on top.
//
// The image supply-chain policy governs these and only these. A preset declared
// in a project's .cidx/presets.toml is the project's business (guardrail 1), and
// letting one reach `cidx preset scan-targets` made the monitor count it as a
// candidate and run a promotion `sed` for an image presets.toml has never heard
// of (#248).
//
// It reads the file rather than the registry init() built, which is why it can
// fail: GlobalRegistry has already swallowed the merge by then.
func Catalogue() (map[string]Preset, error) {
	return loadBasePresets()
}

// Exists checks if a preset exists
func Exists(name string) bool {
	_, exists := GlobalRegistry[name]
	return exists
}

// List returns all preset names
func List() []string {
	names := make([]string, 0, len(GlobalRegistry))
	for name := range GlobalRegistry {
		names = append(names, name)
	}
	return names
}

// ListByPhase returns presets filtered by phase
func ListByPhase(phase string) []string {
	names := make([]string, 0)
	for name, preset := range GlobalRegistry {
		if preset.Phase == phase {
			names = append(names, name)
		}
	}
	return names
}

// GroupByPhase returns presets grouped by phase
func GroupByPhase() map[string][]string {
	grouped := make(map[string][]string)
	for name, preset := range GlobalRegistry {
		grouped[preset.Phase] = append(grouped[preset.Phase], name)
	}
	return grouped
}
