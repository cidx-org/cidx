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
