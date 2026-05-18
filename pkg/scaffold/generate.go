package scaffold

import (
	"fmt"
	"strings"
)

// GenerateTOML produces a cidx.toml from detection results.
//
// For single-language detections the generated config mirrors that language's
// recommended containers. For multi-language detections (fullstack monorepos:
// Python backend + Node frontend, etc.) containers are aggregated across all
// detected languages with order-preserving dedup, so a project gets the
// linters, scanners, and test runners of every stack it actually uses.
func GenerateTOML(d *Detection) string {
	if len(d.Languages) == 0 {
		return defaultConfig()
	}

	primary := d.Languages[0]

	// Aggregate per-phase containers across all detected languages.
	security := mergeContainers(d.Languages, func(l Language) []string { return l.Security })
	code := mergeContainers(d.Languages, func(l Language) []string { return l.Code })
	test := mergeContainers(d.Languages, func(l Language) []string { return l.Test })
	build := mergeContainers(d.Languages, func(l Language) []string { return l.Build })

	var b strings.Builder

	b.WriteString("# CIDX Configuration\n")
	fmt.Fprintf(&b, "# Auto-detected: %s project", primary.Name)
	if len(d.Languages) > 1 {
		names := make([]string, len(d.Languages))
		for i, l := range d.Languages {
			names[i] = l.Name
		}
		fmt.Fprintf(&b, " (also: %s)", strings.Join(names[1:], ", "))
	}
	b.WriteString("\n#\n")
	b.WriteString("# Edit to match your needs. Run 'cidx preset list' for all available tools.\n")
	b.WriteString("# Run 'cidx validate' to check, 'cidx run ci' to execute.\n\n")

	if len(security) > 0 {
		b.WriteString("[security]\n")
		fmt.Fprintf(&b, "containers = [%s]\n\n", quotedList(security))
	}
	if len(code) > 0 {
		b.WriteString("[code]\n")
		fmt.Fprintf(&b, "containers = [%s]\n\n", quotedList(code))
	}
	if len(test) > 0 {
		b.WriteString("[test]\n")
		fmt.Fprintf(&b, "containers = [%s]\n\n", quotedList(test))
	}
	if len(build) > 0 {
		b.WriteString("[build]\n")
		fmt.Fprintf(&b, "containers = [%s]\n\n", quotedList(build))
	}

	// Pipelines reference whichever phases ended up populated.
	phases := collectPhases(security, code, test, build)

	b.WriteString("[pipelines.ci]\n")
	fmt.Fprintf(&b, "phases = [%s]\n\n", quotedList(phases))

	b.WriteString("[pipelines.pr]\n")
	prPhases := filterPhases(phases, "build") // PR skips build
	fmt.Fprintf(&b, "phases = [%s]\n", quotedList(prPhases))

	return b.String()
}

// mergeContainers walks the language list in order and accumulates the
// per-phase container slice (returned by sel), preserving first-seen order and
// dropping duplicates. The first language's containers therefore lead the
// resulting list — keeping single-language output identical to the pre-merge
// behavior.
func mergeContainers(langs []Language, sel func(Language) []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, l := range langs {
		for _, c := range sel(l) {
			if seen[c] {
				continue
			}
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

func collectPhases(security, code, test, build []string) []string {
	var phases []string
	if len(security) > 0 {
		phases = append(phases, "security")
	}
	if len(code) > 0 {
		phases = append(phases, "code")
	}
	if len(test) > 0 {
		phases = append(phases, "test")
	}
	if len(build) > 0 {
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
