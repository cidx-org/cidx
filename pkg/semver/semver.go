// Package semver provides the minimal major.minor.patch handling the release
// flow and the required_version check need.
//
// Every entry point accepts both spellings a version reaches cidx with: the
// tag form ("v2.1.4", what every tag and doc example uses) and the bare form
// ("2.1.4", what release binaries carry in ldflags). Comparing them as strings
// made a config pinned to "v2.1.4" always mismatch a binary reporting "2.1.4"
// (issue #212), so the prefix is dropped before anything numeric happens.
package semver

import (
	"fmt"
	"strings"
	"unicode"
)

// Trim drops any tag prefix ("v", "release-") so the numeric part parses
// whatever prefix the project configured.
func Trim(version string) string {
	return strings.TrimLeftFunc(version, func(r rune) bool { return !unicode.IsDigit(r) })
}

// Parse splits a version into its numeric parts, ignoring any tag prefix.
func Parse(version string) (major, minor, patch int, ok bool) {
	n, _ := fmt.Sscanf(Trim(version), "%d.%d.%d", &major, &minor, &patch)
	return major, minor, patch, n == 3
}

// IsValid reports whether the version carries a major.minor.patch.
func IsValid(version string) bool {
	_, _, _, ok := Parse(version)
	return ok
}

// Compare returns -1, 0 or 1 depending on how a compares to b.
func Compare(a, b string) int {
	aMajor, aMinor, aPatch, _ := Parse(a)
	bMajor, bMinor, bPatch, _ := Parse(b)

	for _, pair := range [][2]int{{aMajor, bMajor}, {aMinor, bMinor}, {aPatch, bPatch}} {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}
	return 0
}
