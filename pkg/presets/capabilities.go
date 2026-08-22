package presets

import (
	"regexp"
	"sort"
	"strings"
)

// Mechanical capabilities derived from what a preset declares. These are the
// two the grouped vulnerability decisions fingerprint on — see
// docs/discussions/vulnerability-management-reset.md, "Context derivation".
const (
	CapabilityPublishingCredential = "publishing-credential"
	CapabilityDockerSocket         = "docker-socket"
)

// credentialEnv matches an env key or expansion that hands the container a
// secret. The names come from the catalogue's own declarations: GITHUB_TOKEN,
// GH_TOKEN, TWINE_PASSWORD/PYPI_TOKEN, CARGO_REGISTRY_TOKEN,
// ANSIBLE_GALAXY_TOKEN.
var credentialEnv = regexp.MustCompile(`TOKEN|PASSWORD|SECRET|CREDENTIAL`)

// Capabilities derives the mechanical capability set from what a preset
// declares — env and volumes, the same declarations the executor runs from.
// Nothing is read out of the command string: input origin and repository
// execution are semantic context, established by review rather than guessed
// from shell text.
func Capabilities(p Preset) []string {
	caps := map[string]bool{}
	for key, value := range p.Env {
		if credentialEnv.MatchString(key) || credentialEnv.MatchString(value) {
			caps[CapabilityPublishingCredential] = true
		}
	}
	for _, volume := range p.Volumes {
		if strings.Contains(volume, "/var/run/docker.sock") {
			caps[CapabilityDockerSocket] = true
		}
	}
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AggregateCapabilities is the union over every preset consuming a repository.
// The scanner ignore applies to the image, not to one preset invocation, so a
// benign consumer must never hide the exposure of a more privileged one.
func AggregateCapabilities(consumers []Preset) []string {
	caps := map[string]bool{}
	for _, p := range consumers {
		for _, c := range Capabilities(p) {
			caps[c] = true
		}
	}
	names := make([]string, 0, len(caps))
	for name := range caps {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
