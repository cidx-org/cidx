package config

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// projectRoot is this repository, seen from the directory `go test` runs this
// package in.
const projectRoot = "../.."

// TestTheTestPhaseRunsEveryPackageThatHasTests is the standing guard behind
// issue #344.
//
// The godog suites live in `features/` and, since #317, every CLI test lives
// in `internal/commands`. The `go-test` preset ran
// `go test -v ./pkg/... ./cmd/...` and reached neither, so this repository
// repaired it with a one-line override; #357 widened the catalogue to `./...`
// and the override is gone. Narrowing the resolved command — in the preset now,
// or by putting an override back — would silence the BDD suite and all of the
// CLI tests at once, and the Test job would stay green having run neither: the
// invisible failure of a decorative check, the same one as #272 and #324.
//
// This guard resolves the command the way the runner does, preset plus
// `[containers.NAME]` overrides, so it holds whichever of the two is narrowed.
// `TestNoTestPresetRunsOnlyPartOfTheProject`, in `pkg/presets`, states the rule
// for the catalogue itself and for every project that uses it.
//
// The guard lives here, under ./pkg/..., and not with the repository-wide
// guards of internal/guards, on purpose: the command it checks decides which
// packages are compiled at all, so a guard placed in a package that command can
// stop naming would be skipped by the exact narrowing it exists to catch, and
// CI would stay green twice over. Under ./pkg/... it survives whatever the
// command has been reduced to.
func TestTheTestPhaseRunsEveryPackageThatHasTests(t *testing.T) {
	cfg, err := Load(filepath.Join(projectRoot, "cidx.toml"))
	if err != nil {
		t.Fatalf("failed to load cidx.toml: %v", err)
	}

	tested := packagesHoldingTests(t)
	if len(tested) == 0 {
		t.Fatal("no package with tests found — the guard would examine the wrong tree and pass vacuously")
	}

	examined := 0
	for _, container := range containersReachableFromAPipeline(cfg) {
		command := resolvedCommand(cfg, container)
		if !strings.HasPrefix(command, "go test") {
			continue
		}
		examined++

		patterns := packagePatterns(command)
		for _, pkg := range tested {
			if covers(patterns, pkg) {
				continue
			}
			t.Errorf("[containers.%s] resolves to %q, which does not run %s — the tests there would never run in CI (#344). "+
				"`./...` is what covers every package, features/ and internal/commands included.",
				container, command, spell(pkg))
		}
	}

	if examined == 0 {
		t.Fatal("no `go test` command is reachable from a pipeline: nothing runs the tests at all (#344)")
	}
}

// containersReachableFromAPipeline returns the containers CI can reach: those
// declared by a phase some pipeline lists.
func containersReachableFromAPipeline(cfg *Config) []string {
	var containers []string
	seen := make(map[string]bool)

	for _, pipeline := range cfg.Pipelines {
		for _, phase := range pipeline.Phases {
			for _, container := range cfg.Phases[phase].Containers {
				if !seen[container] {
					seen[container] = true
					containers = append(containers, container)
				}
			}
		}
	}
	return containers
}

// resolvedCommand returns the command a container actually runs: the preset's,
// with the project's [containers.NAME] overrides merged on top — the same two
// steps pipeline.RunTool takes.
func resolvedCommand(cfg *Config, container string) string {
	overrides := cfg.Overrides[container]

	preset, err := presets.Get(container)
	if err != nil {
		// Not a catalogue preset: the [containers.NAME] declaration is the
		// whole definition.
		command, _ := overrides["command"].(string)
		return command
	}
	return preset.MergeWith(overrides).Command
}

// packagesHoldingTests lists the packages of this repository that carry a
// *_test.go file, as paths relative to the repository root ("." for the root
// package).
func packagesHoldingTests(t *testing.T) []string {
	t.Helper()

	var packages []string
	seen := make(map[string]bool)

	err := filepath.WalkDir(projectRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != projectRoot && (strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		pkg, err := filepath.Rel(projectRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg = filepath.ToSlash(pkg)
		if !seen[pkg] {
			seen[pkg] = true
			packages = append(packages, pkg)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk the repository: %v", err)
	}
	return packages
}

// spell names a package the way a command line would.
func spell(pkg string) string {
	if pkg == "." {
		return "the root package"
	}
	return "./" + pkg
}

// packagePatterns extracts the package patterns a `go test` command line
// carries: the arguments spelled the way Go spells them, `./...` and friends.
func packagePatterns(command string) []string {
	var patterns []string
	for _, field := range strings.Fields(command) {
		if strings.HasPrefix(field, "./") {
			patterns = append(patterns, field)
		}
	}
	return patterns
}

// covers reports whether one of the patterns makes `go test` compile pkg, a
// package path relative to the repository root.
func covers(patterns []string, pkg string) bool {
	for _, pattern := range patterns {
		p := strings.TrimPrefix(pattern, "./")

		if trimmed := strings.TrimSuffix(p, "..."); trimmed != p {
			// A wildcard: everything under its prefix, the prefix included.
			// "./..." leaves an empty prefix, which is the whole tree.
			prefix := strings.TrimSuffix(trimmed, "/")
			if prefix == "" || pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
				return true
			}
			continue
		}

		if pkg == strings.TrimSuffix(p, "/") {
			return true
		}
	}
	return false
}
