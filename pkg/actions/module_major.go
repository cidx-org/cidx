package actions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// modulePathMajor reads the major suffix a module path declares: 2 for
// `.../cidx/v2`, and 1 for a path with no suffix, which is what Go means by v0
// and v1 sharing the unsuffixed path.
var modulePathMajor = regexp.MustCompile(`/v([0-9]+)$`)

// CheckModuleMajor reports whether go.mod may be tagged as version.
//
// Go resolves a module by its declared path, so `module .../cidx/v2` tagged
// v3.0.0 is not v3 of anything: `go list -m .../cidx/v3` answers "go.mod has
// non-.../v3 module path", and `go install .../cidx/v2/cmd/cidx@latest` — the
// line `cidx generate github` writes into every bootstrapped workflow — quietly
// resolves to the newest v2 instead. The v3.0.0 release shipped exactly that
// (issue #395), and it was the second time: #187 was the same mismatch one
// major earlier, fixed by editing the path once, with nothing left to catch the
// next one.
//
// So the check is here rather than in a checklist. It runs where a major
// becomes permanent — the tag — and it is one comparison.
//
// Only a major bump is constrained. v3.0.1 on a /v3 module is fine, and so is
// any v0/v1 tag on an unsuffixed path, which is the convention Go itself
// defines rather than an omission.
func CheckModuleMajor(workDir, version string) error {
	tagged, err := majorOf(version)
	if err != nil {
		return err
	}

	source, err := os.ReadFile(filepath.Join(workDir, "go.mod"))
	if err != nil {
		// A repository with no go.mod is not a Go module, and this rule has
		// nothing to say about it.
		return nil
	}

	declared, path := declaredMajor(string(source))
	if declared == tagged {
		return nil
	}

	// Go gives v0 and v1 the same unsuffixed path, so both are at home there.
	// Only v2 upward requires a suffix, which is why this rule has anything to
	// say at all.
	if declared == 1 && tagged == 0 {
		return nil
	}

	return fmt.Errorf(
		"go.mod declares module %q, so tagging %s would publish a major nobody can import:\n"+
			"   `go list -m %s/v%d` fails, and `go install %s/cmd/cidx@latest` resolves to the newest v%d\n"+
			"   Change the module path to end in /v%d and update every import, then tag again (#395)",
		path, version, moduleRoot(path), tagged, path, declared, tagged)
}

// majorOf reads the major component of a version, with or without its `v`.
func majorOf(version string) (int, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(version), "v")
	major, _, _ := strings.Cut(trimmed, ".")

	n, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("cannot read a major version from %q: %w", version, err)
	}

	return n, nil
}

// declaredMajor returns the major a go.mod's module path declares, and the path
// itself. An unsuffixed path is major 1, which is how Go reads it.
func declaredMajor(gomod string) (major int, path string) {
	for _, line := range strings.Split(gomod, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "module ")
		if !found {
			continue
		}
		path = strings.TrimSpace(rest)

		if m := modulePathMajor.FindStringSubmatch(path); m != nil {
			n, _ := strconv.Atoi(m[1])
			return n, path
		}
		return 1, path
	}

	return 1, ""
}

// moduleRoot strips a major suffix, so an error can name the path the tag would
// require rather than the one that is there.
func moduleRoot(path string) string {
	return modulePathMajor.ReplaceAllString(path, "")
}
