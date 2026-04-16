package scaffold

import (
	"fmt"
	"regexp"
	"strings"
)

// containerLineRe matches a TOML containers array line, capturing the content inside brackets.
var containerLineRe = regexp.MustCompile(`^(\s*containers\s*=\s*\[)(.*)(\]\s*)$`)

// UpdateTOML applies additive changes from a DiffResult to raw TOML content.
// It preserves all existing content, comments, and formatting.
// Only containers arrays in matching phases are extended, and new phases are appended.
func UpdateTOML(raw string, diff *DiffResult) string {
	if !diff.HasChanges() {
		return raw
	}

	lines := strings.Split(raw, "\n")

	// Build a map of phase -> containers to add
	toAdd := make(map[string][]string)
	for _, c := range diff.Changes {
		if len(c.Added) > 0 {
			toAdd[c.Phase] = c.Added
		}
	}

	// Process existing phases: find the containers line and extend it
	currentPhase := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track current TOML section
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			// Extract section name (handles [security], [containers.trivy], [pipelines.ci], etc.)
			name := strings.Trim(trimmed, "[] ")
			// Only track top-level phase sections (no dots)
			if !strings.Contains(name, ".") {
				currentPhase = name
			} else {
				currentPhase = ""
			}
			continue
		}

		// Check if this is a containers line in a phase we need to update
		additions, ok := toAdd[currentPhase]
		if !ok || len(additions) == 0 {
			continue
		}

		match := containerLineRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		// Extend the containers list
		existingContent := strings.TrimSpace(match[2])
		newItems := quotedList(additions)

		var updated string
		if existingContent == "" {
			updated = fmt.Sprintf("%s%s%s", match[1], newItems, match[3])
		} else {
			updated = fmt.Sprintf("%s%s, %s%s", match[1], existingContent, newItems, match[3])
		}

		lines[i] = updated
		delete(toAdd, currentPhase) // mark as done
	}

	result := strings.Join(lines, "\n")

	// Append entirely new phases (from NewPhases) and any phases we couldn't find in the file
	var newSections []string

	for _, c := range diff.NewPhases {
		newSections = append(newSections, fmt.Sprintf("[%s]\ncontainers = [%s]\n",
			c.Phase, quotedList(c.Added)))
	}

	// Also handle phases that were in Changes but not found in the file
	for phase, containers := range toAdd {
		newSections = append(newSections, fmt.Sprintf("[%s]\ncontainers = [%s]\n",
			phase, quotedList(containers)))
	}

	if len(newSections) > 0 {
		// Insert before [pipelines.*] if it exists, otherwise append at end
		pipelineIdx := strings.Index(result, "\n[pipelines.")
		if pipelineIdx >= 0 {
			result = result[:pipelineIdx] + "\n\n" + strings.Join(newSections, "\n") + result[pipelineIdx:]
		} else {
			if !strings.HasSuffix(result, "\n") {
				result += "\n"
			}
			result += "\n" + strings.Join(newSections, "\n")
		}
	}

	return result
}
