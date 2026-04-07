package scaffold

import (
	"fmt"
	"strings"
)

// GenerateTOML produces a cidx.toml from detection results.
func GenerateTOML(d *Detection) string {
	if len(d.Languages) == 0 {
		return defaultConfig()
	}

	lang := d.Languages[0] // primary language

	var b strings.Builder

	b.WriteString("# CIDX Configuration\n")
	fmt.Fprintf(&b,"# Auto-detected: %s project", lang.Name)
	if len(d.Languages) > 1 {
		names := make([]string, len(d.Languages))
		for i, l := range d.Languages {
			names[i] = l.Name
		}
		fmt.Fprintf(&b," (also: %s)", strings.Join(names[1:], ", "))
	}
	b.WriteString("\n#\n")
	b.WriteString("# Edit to match your needs. Run 'cidx preset list' for all available tools.\n")
	b.WriteString("# Run 'cidx validate' to check, 'cidx run ci' to execute.\n\n")

	// Security phase
	if len(lang.Security) > 0 {
		b.WriteString("[security]\n")
		fmt.Fprintf(&b,"containers = [%s]\n\n", quotedList(lang.Security))
	}

	// Code phase
	if len(lang.Code) > 0 {
		b.WriteString("[code]\n")
		fmt.Fprintf(&b,"containers = [%s]\n\n", quotedList(lang.Code))
	}

	// Test phase
	if len(lang.Test) > 0 {
		b.WriteString("[test]\n")
		fmt.Fprintf(&b,"containers = [%s]\n\n", quotedList(lang.Test))
	}

	// Build phase
	if len(lang.Build) > 0 {
		b.WriteString("[build]\n")
		fmt.Fprintf(&b,"containers = [%s]\n\n", quotedList(lang.Build))
	}

	// Pipelines
	b.WriteString("[pipelines.ci]\n")
	phases := collectPhasesFromLang(lang)
	fmt.Fprintf(&b,"phases = [%s]\n\n", quotedList(phases))

	b.WriteString("[pipelines.pr]\n")
	prPhases := filterPhases(phases, "build") // PR skips build
	fmt.Fprintf(&b,"phases = [%s]\n", quotedList(prPhases))

	return b.String()
}

func collectPhasesFromLang(lang Language) []string {
	var phases []string
	if len(lang.Security) > 0 {
		phases = append(phases, "security")
	}
	if len(lang.Code) > 0 {
		phases = append(phases, "code")
	}
	if len(lang.Test) > 0 {
		phases = append(phases, "test")
	}
	if len(lang.Build) > 0 {
		phases = append(phases, "build")
	}
	return phases
}

func filterPhases(phases []string, exclude string) []string {
	var filtered []string
	for _, p := range phases {
		if p != exclude {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func quotedList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	return strings.Join(quoted, ", ")
}

func defaultConfig() string {
	return `# CIDX Configuration
#
# No project type detected. Edit to match your needs.
# Run 'cidx preset list' for all available tools.

[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["prettier"]

[pipelines.ci]
phases = ["security", "code"]
`
}
