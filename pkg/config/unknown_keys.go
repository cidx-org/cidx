package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// unknownKeys returns every key of a cidx.toml that nothing will read, sorted
// and in dotted form.
//
// This is the check `cidx.toml` never had (issue #371). `pkg/presets` grew it
// for custom preset files in #203, after the same class of bug hid `pull_policy`
// and `timeout`; the file every project has went without, so `stale_dayz = 15`
// kept its default of 30 and `cidx validate` called the file valid. A typo was
// indistinguishable from the setting working.
//
// It cannot be a single Undecoded() call, because the loader decodes in two
// passes: the typed struct claims the known sections, and the raw map carries
// the ones whose names the user chooses -- phases and container overrides. To
// Undecoded() those look identical to a mistake, so each is checked against
// what its own reader accepts:
//
//   - known sections ([branch], [pr], [pipelines.*], ...): the struct decides,
//     and MetaData.Undecoded() names exactly what it refused.
//   - a phase section: `containers` and nothing else.
//   - a [containers.<name>] section: the structural override keys plus the
//     options the named preset declares, which pkg/presets answers so the
//     list lives next to the code that reads it.
func unknownKeys(md toml.MetaData, raw map[string]any) []string {
	var unknown []string

	// Known sections: the typed struct is the authority.
	for _, key := range md.Undecoded() {
		if parts := key; len(parts) > 0 && knownSections[parts[0]] {
			unknown = append(unknown, key.String())
		}
	}

	for name, value := range raw {
		if knownSections[name] {
			continue
		}
		section, ok := value.(map[string]any)
		if !ok {
			// A bare top-level key that is not a known one -- neither a phase
			// nor a container override can be a scalar.
			unknown = append(unknown, name)
			continue
		}

		if name == "containers" {
			for container, raw := range section {
				overrides, ok := raw.(map[string]any)
				if !ok {
					unknown = append(unknown, "containers."+container)
					continue
				}
				for _, key := range presets.UnknownOverrideKeys(container, overrides) {
					unknown = append(unknown, "containers."+container+"."+key)
				}
			}
			continue
		}

		// A section carrying `containers` is a phase; anything else is a
		// top-level container override in the legacy spelling.
		if _, isPhase := section["containers"]; isPhase {
			for key := range section {
				if key != "containers" {
					unknown = append(unknown, name+"."+key)
				}
			}
			continue
		}
		for _, key := range presets.UnknownOverrideKeys(name, section) {
			unknown = append(unknown, name+"."+key)
		}
	}

	sort.Strings(unknown)

	return unknown
}

// errUnknownKeys reports the keys as a single error naming all of them.
//
// It fails rather than warns, and it fails for `cidx run` as much as for `cidx
// validate`. A warning on a config that does not mean what it says is a warning
// people scroll past, and the run that follows is the unattended behaviour the
// check exists to prevent.
func errUnknownKeys(path string, keys []string) error {
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, "  "+key)
	}

	return fmt.Errorf("%s declares %d key(s) that cidx does not read:\n%s\n\n"+
		"A key cidx ignores is a setting that is not in effect, so it is refused rather\n"+
		"than dropped. Check the spelling, or remove it.\n"+
		"Run `cidx preset info <name>` to see the options a container accepts",
		path, len(keys), strings.Join(lines, "\n"))
}
