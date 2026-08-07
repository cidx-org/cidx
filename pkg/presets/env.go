package presets

import (
	"fmt"
	"os"
	"strings"
)

// ResolveEnvValue returns the value a preset's env entry takes at run time.
//
// A preset's declared value is a default. `IMAGE_TAG = "latest"`,
// `SCENARIO = "default"`, `TAG = "latest"` read like defaults and are meant to
// be parameterised per run, so a variable the caller exported wins over the
// declaration (issue #384).
//
// It did not, and the consequence was invisible for as long as the catalogue
// has existed: `release.yml` set `IMAGE_TAG` to the release tag in front of
// `cidx run docker`, `nightly.yml` set it to `nightly`, and both published
// `:latest` because the declared value was the only one ever read. What made
// it hard to see is that a reference *inside* a value always worked --
// `IMAGE_NAME = "ghcr.io/${GITHUB_REPOSITORY}"` resolves -- so the mechanism
// looked present while the key itself was never looked up.
//
// Set-but-empty counts as set. Exporting `IMAGE_TAG=""` then yields an
// incomplete reference the registry rejects, which is the loud failure; the
// alternative, silently falling back to `latest`, is the bug this fixes.
func ResolveEnvValue(key, declared string) string {
	if fromEnv, ok := os.LookupEnv(key); ok {
		return fromEnv
	}

	return os.ExpandEnv(declared)
}

// ExpandCommand substitutes the ${KEY} placeholders a preset's command spells
// out, taking each value from ResolveEnvValue.
//
// Only the placeholders a command names are parameterised this way. The
// invariants a container needs to run as the invoking uid -- HOME, GOPATH,
// GOCACHE, the cache directories of #188 -- are declared by presets and
// referenced by no command, so a developer's own HOME never follows them in.
//
// A `sh -c '...'` command keeps whatever references remain: they belong to the
// shell inside the container, and expanding them here would resolve them
// against the host instead.
func ExpandCommand(command string, env map[string]string) string {
	expanded := command
	for key, declared := range env {
		placeholder := fmt.Sprintf("${%s}", key)
		expanded = strings.ReplaceAll(expanded, placeholder, ResolveEnvValue(key, declared))
	}

	if strings.HasPrefix(strings.TrimSpace(command), "sh -c") {
		return expanded
	}

	return os.ExpandEnv(expanded)
}
