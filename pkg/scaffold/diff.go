package scaffold

import (
	"fmt"
	"strings"
)

// PhaseChange describes additions to a single phase.
type PhaseChange struct {
	Phase    string
	Added    []string // containers found by detection but missing from config
	Existing []string // containers already in config for this phase
}

// DiffResult holds the comparison between fresh detection and existing config.
type DiffResult struct {
	Changes   []PhaseChange
	NewPhases []PhaseChange // entirely new phases (not in existing config)
}

// HasChanges returns true if the diff contains any additions.
func (d *DiffResult) HasChanges() bool {
	for _, c := range d.Changes {
		if len(c.Added) > 0 {
			return true
		}
	}
	return len(d.NewPhases) > 0
}

// TotalAdded returns the total number of new containers across all phases.
func (d *DiffResult) TotalAdded() int {
	count := 0
	for _, c := range d.Changes {
		count += len(c.Added)
	}
	for _, c := range d.NewPhases {
		count += len(c.Added)
	}
	return count
}

// Compare computes the additive diff between a fresh detection and existing config phases.
// existingPhases maps phase name to its current container list.
func Compare(detection *Detection, existingPhases map[string][]string) *DiffResult {
	result := &DiffResult{}

	if len(detection.Languages) == 0 {
		return result
	}

	// Build detected containers per phase from all languages
	detected := make(map[string][]string)
	seen := make(map[string]map[string]bool)

	for _, lang := range detection.Languages {
		for phase, containers := range map[string][]string{
			"security": lang.Security,
			"code":     lang.Code,
			"test":     lang.Test,
			"build":    lang.Build,
		} {
			if len(containers) == 0 {
				continue
			}
			if seen[phase] == nil {
				seen[phase] = make(map[string]bool)
			}
			for _, c := range containers {
				if !seen[phase][c] {
					seen[phase][c] = true
					detected[phase] = append(detected[phase], c)
				}
			}
		}
	}

	// Compare detected vs existing
	for _, phase := range []string{"security", "code", "test", "build"} {
		detectedContainers := detected[phase]
		if len(detectedContainers) == 0 {
			continue
		}

		existingContainers, phaseExists := existingPhases[phase]

		if !phaseExists {
			// Entirely new phase
			result.NewPhases = append(result.NewPhases, PhaseChange{
				Phase: phase,
				Added: detectedContainers,
			})
			continue
		}

		// Find containers missing from existing
		existingSet := make(map[string]bool)
		for _, c := range existingContainers {
			existingSet[c] = true
		}

		var added []string
		for _, c := range detectedContainers {
			if !existingSet[c] {
				added = append(added, c)
			}
		}

		result.Changes = append(result.Changes, PhaseChange{
			Phase:    phase,
			Added:    added,
			Existing: existingContainers,
		})
	}

	return result
}

// FormatDiff returns a human-readable representation of the diff.
func FormatDiff(diff *DiffResult) string {
	if !diff.HasChanges() {
		return "  Config is up to date — no new tools detected.\n"
	}

	var b strings.Builder

	for _, c := range diff.Changes {
		if len(c.Added) == 0 {
			continue
		}
		fmt.Fprintf(&b, "  [%s]\n", c.Phase)
		for _, name := range c.Added {
			fmt.Fprintf(&b, "    + %s\n", name)
		}
	}

	for _, c := range diff.NewPhases {
		fmt.Fprintf(&b, "  [%s] (new phase)\n", c.Phase)
		for _, name := range c.Added {
			fmt.Fprintf(&b, "    + %s\n", name)
		}
	}

	fmt.Fprintf(&b, "\n  %d container(s) to add.\n", diff.TotalAdded())
	return b.String()
}
