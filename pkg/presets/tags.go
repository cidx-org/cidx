package presets

import (
	"regexp"
	"strconv"
	"strings"
)

// Picking the newest version out of a registry tag listing (issue #245).
//
// Docker Hub and Quay.io hand their tags back ordered by push date, so the
// first one matching the version shape is the newest and nothing else is
// needed. The OCI distribution API — everything reached through
// `GET /v2/<repo>/tags/list` — answers with an unordered list of names and
// nothing else, so the newest version has to be read out of the names.

// tagVersionPattern splits a tag into an optional `v`, a dotted version and
// whatever follows it: `1.23-alpine3.21-dev` is version 1.23 of the
// `-alpine3.21-dev` variant.
//
// A tag that does not start with a version — `latest`, `initial-push`,
// `nightly`, `sha-1b2c3d4` — does not match and takes no part in the choice.
var tagVersionPattern = regexp.MustCompile(`^(v?)([0-9]+(?:\.[0-9]+)*)(.*)$`)

// NewerTag returns the newest tag in available that is a strictly newer version
// of currentTag, or "" when nothing on offer qualifies.
//
// Qualifying means the same shape as the tag the catalogue already pins:
//
//   - Same variant family — same `v` prefix, same suffix after the version.
//     `dhi.io/golang:1.23-alpine3.21-dev` must never be offered a plain `1.24`:
//     that is a different image on a different base, and promoting it would
//     silently change what every Go preset runs on. The same guard keeps
//     `-dev`, `-fips`, `-cli` and `-alpine` variants in their own lane.
//
//   - Same precision. A registry publishing `0.71` and `0.71.2` offers the
//     first to an image pinned `0.68` and the second to one pinned `0.68.1`.
//     The number of components is a choice the catalogue made about how
//     closely it tracks upstream, and an update is not the place to revisit it.
//
// A currentTag carrying no version (`latest`) qualifies nothing: there is no
// way to tell what would be newer, and claiming an update would be a guess.
func NewerTag(currentTag string, available []string) string {
	prefix, current, suffix, ok := splitTagVersion(currentTag)
	if !ok {
		return ""
	}

	newest, newestVersion := "", current
	for _, tag := range available {
		tagPrefix, version, tagSuffix, ok := splitTagVersion(tag)
		if !ok || tagPrefix != prefix || tagSuffix != suffix || len(version) != len(current) {
			continue
		}
		if compareVersions(version, newestVersion) > 0 {
			newest, newestVersion = tag, version
		}
	}
	return newest
}

// SupersedingVariant returns the variant family that replaced the one
// currentTag pins, or "" when nothing has (issue #252).
//
// A variant line does not always die by 404. `dhi.io/golang:1.23-alpine3.21-dev`
// still pulls, but DHI publishes no `-alpine3.21-dev` tag at all any more — it
// moved to `-alpine3.24-dev`. No successor will ever appear inside the pinned
// family, so NewerTag correctly offers nothing, and reporting that as up to date
// leaves the catalogue on a line upstream abandoned: the quieter cousin of the
// images deleted outright in #244.
//
// The line counts as frozen only when the repository lists no tag whatsoever in
// the pinned family. A family still published, even sitting at its own head, is
// alive and still receives fixes — saying otherwise would fire every week on
// every image that is merely current.
//
// The successor is found by reading the version the suffix itself carries:
// `-alpine3.21-dev` is version 3.21 of the `-alpine…-dev` line, so
// `-alpine3.24-dev` supersedes it while `-alpine3.24-fips-dev` is a different
// line altogether.
func SupersedingVariant(currentTag string, available []string) string {
	_, _, suffix, ok := splitTagVersion(currentTag)
	if !ok {
		return ""
	}
	prefix, current, rest, ok := splitSuffixVersion(suffix)
	if !ok {
		return ""
	}

	newest, newestVersion := "", current
	for _, tag := range available {
		_, _, tagSuffix, ok := splitTagVersion(tag)
		if !ok {
			continue
		}
		if tagSuffix == suffix {
			return "" // the pinned family is still published, so it is not frozen
		}

		tagPrefix, version, tagRest, ok := splitSuffixVersion(tagSuffix)
		if !ok || tagPrefix != prefix || tagRest != rest || len(version) != len(current) {
			continue
		}
		if compareVersions(version, newestVersion) > 0 {
			newest, newestVersion = tagSuffix, version
		}
	}
	return newest
}

// suffixVersionPattern splits a variant suffix around the version it carries:
// `-alpine3.21-dev` is version 3.21 of the `-alpine…-dev` line. The prefix is
// non-greedy so the first version in the suffix is the one that names the line.
var suffixVersionPattern = regexp.MustCompile(`^(.*?)([0-9]+(?:\.[0-9]+)*)(.*)$`)

// splitSuffixVersion reads a variant suffix as (prefix, version, rest). ok is
// false for a suffix carrying no version — `-dev`, `-cli`, or none at all —
// which names no line that could be superseded.
func splitSuffixVersion(suffix string) (prefix string, version []int, rest string, ok bool) {
	match := suffixVersionPattern.FindStringSubmatch(suffix)
	if match == nil {
		return "", nil, "", false
	}

	for _, part := range strings.Split(match[2], ".") {
		number, err := strconv.Atoi(part)
		if err != nil {
			return "", nil, "", false
		}
		version = append(version, number)
	}
	return match[1], version, match[3], true
}

// splitTagVersion reads a tag as (prefix, version, variant suffix). ok is false
// for a tag that carries no leading version at all.
func splitTagVersion(tag string) (prefix string, version []int, suffix string, ok bool) {
	match := tagVersionPattern.FindStringSubmatch(tag)
	if match == nil {
		return "", nil, "", false
	}

	for _, part := range strings.Split(match[2], ".") {
		number, err := strconv.Atoi(part)
		if err != nil {
			return "", nil, "", false
		}
		version = append(version, number)
	}
	return match[1], version, match[3], true
}

// compareVersions orders two versions component by component, as numbers —
// `1.24` is above `1.9`, which no lexical ordering would say. NewerTag only
// compares versions of the same precision, so a shared prefix means equal.
func compareVersions(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
