package presets

import "sort"

// overrideKeys are the keys a `[containers.<name>]` section may carry besides
// the preset's own declared options.
//
// It is the union of what the two readers of such a section consume: MergeWith,
// which layers an override onto a catalogue preset, and PresetFromOverrides,
// which builds a container the catalogue does not have (`phase` and
// `privileged` are only meaningful there, and are accepted on both because a
// section does not announce which reader it is for).
//
// Deliberately absent: `config_files`. Neither reader looks at it, so declaring
// it in cidx.toml has never done anything -- and now says so instead of being
// ignored (issue #371).
//
// TestOverrideKeysMatchTheReaders keeps this list honest against the code that
// actually reads the map, so it cannot drift into promising a key nothing
// consumes or rejecting one that works.
var overrideKeys = map[string]bool{
	"image":       true,
	"command":     true,
	"entrypoint":  true,
	"workdir":     true,
	"volumes":     true,
	"env":         true,
	"phase":       true,
	"privileged":  true,
	"pull_policy": true,
	"timeout":     true,
}

// UnknownOverrideKeys returns the keys of a `[containers.<name>]` section that
// nothing will read, sorted.
//
// A key here is valid if one of the section's two readers consumes it, or if
// the named preset declares it as an option. An unknown preset name yields no
// options, so only the structural keys are accepted -- which is right: a
// section naming no catalogue preset is a custom declaration, and a custom
// container has no options to set.
func UnknownOverrideKeys(presetName string, section map[string]any) []string {
	var options map[string]Option
	if preset, err := Get(presetName); err == nil {
		options = preset.Options
	}

	var unknown []string
	for key := range section {
		if overrideKeys[key] {
			continue
		}
		if _, isOption := options[key]; isOption {
			continue
		}
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)

	return unknown
}
