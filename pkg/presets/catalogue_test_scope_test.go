package presets

import (
	"regexp"
	"sort"
	"testing"
)

// subtreePattern matches a package selector that names a subtree rather than
// the project: `./pkg/...`, `./cmd/...`, `./internal/commands/...`, `src/...`.
// `./...` is the whole tree and is what a test preset is allowed to say -- the
// leading segment has to start with a word character so that the dot of `./...`
// is not read as a directory name.
var subtreePattern = regexp.MustCompile(`(^|\s)(\./)?[\w-][\w.-]*(/[\w.-]+)*/\.\.\.`)

// TestNoTestPresetRunsOnlyPartOfTheProject is the standing guard behind issue
// #357.
//
// `go-test` shipped as `go test -v ./pkg/... ./cmd/...`, which skips the root
// package and everything under internal/ -- for every project using the preset.
// Both are ordinary places for Go tests: the root package is where a
// single-package tool puts them, internal/ is where Go's own convention puts
// private packages. This repository paid for it twice, and both times the Test
// job stayed green having run nothing: #271 found the godog suite had never run
// in CI, #344 found internal/commands in the same blind spot, 19 files
// including the #274 census and the #335 alias guard.
//
// That is the failure mode worth a standing guard rather than review: a test
// phase that runs a subset is indistinguishable, from the outside, from one
// that runs everything. Both print that the container completed.
//
// The rule is the catalogue's own, from CLAUDE.md: defaults must work without
// overrides. A test preset that needs an override to reach the project's tests
// does not.
func TestNoTestPresetRunsOnlyPartOfTheProject(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}

	examined := 0
	var narrowed []string
	for name, preset := range catalogue {
		if preset.Phase != "test" || preset.Command == "" {
			continue
		}
		examined++
		if subtreePattern.MatchString(preset.Command) {
			narrowed = append(narrowed, name+" -> "+preset.Command)
		}
	}

	if examined == 0 {
		t.Fatal("no test preset found — the guard would pass vacuously")
	}
	if len(narrowed) > 0 {
		sort.Strings(narrowed)
		t.Errorf("these test presets run a chosen subtree instead of the project (#357):\n  %s\n"+
			"A project whose tests live outside that subtree gets a green test job that\n"+
			"compiled none of them. Use the language's whole-project form (`./...` for Go).",
			joinLines(narrowed))
	}
}

// joinLines formats one finding per line, indented under the message.
func joinLines(items []string) string {
	out := ""
	for i, item := range items {
		if i > 0 {
			out += "\n  "
		}
		out += item
	}

	return out
}
