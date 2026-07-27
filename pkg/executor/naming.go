package executor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
)

// Container name scoping (issue #197).
//
// Containers used to be named `cidx_<tool>`, which is global to the Docker
// host. Two repositories running the same preset fought over one container:
// each run saw a foreign `cidx.config_hash`, destroyed the other project's
// container (losing its build cache) and recreated its own.
//
// Names are now scoped to the workspace:
//
//	cidx_<project>_<tool>
//	  e.g. cidx_cidx-3f5a9c21_golangci-lint
//
// `<project>` is the sanitized workspace basename (readable — you can tell at a
// glance which repo a container belongs to) plus a short hash of the absolute
// workspace path (unique — two checkouts of the same repo, or two different
// repos sharing a basename, never collide).
const (
	// containerNamePrefix is the fixed prefix of every cidx container name.
	// It also guarantees the name starts with an alphanumeric character, as
	// Docker requires ([a-zA-Z0-9][a-zA-Z0-9_.-]*).
	containerNamePrefix = "cidx"

	// maxProjectBaseLen caps the readable part of the project scope so a
	// deeply named repository cannot blow up the container name.
	maxProjectBaseLen = 24

	// projectHashLen is the number of hex characters of the workspace path
	// hash appended to the readable basename.
	projectHashLen = 8
)

// ContainerName returns the Docker container name for a tool running in the
// given workspace: `cidx_<project>_<tool>`.
//
// The result always satisfies Docker's container name grammar
// ([a-zA-Z0-9][a-zA-Z0-9_.-]*): the `cidx` prefix covers the first character,
// and every dynamic part is sanitized to [a-zA-Z0-9_.-].
func ContainerName(workspace, tool string) string {
	name := sanitizeNamePart(tool)
	if name == "" {
		name = "tool"
	}
	return fmt.Sprintf("%s_%s_%s", containerNamePrefix, ProjectScope(workspace), name)
}

// ProjectScope returns the per-project component of a container name:
// a sanitized, truncated workspace basename followed by a short hash of the
// absolute workspace path.
//
// The hash is computed over the cleaned absolute path, so it is stable for a
// given directory across runs and differs for any two distinct directories —
// including two repositories that share a basename.
func ProjectScope(workspace string) string {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		// filepath.Abs only fails when the working directory is unavailable;
		// fall back to the raw value so naming stays deterministic.
		abs = workspace
	}
	abs = filepath.Clean(abs)

	base := sanitizeNamePart(filepath.Base(abs))
	if len(base) > maxProjectBaseLen {
		base = strings.Trim(base[:maxProjectBaseLen], "-._")
	}
	if base == "" {
		// Root directories ("/") and paths made entirely of invalid
		// characters leave nothing readable behind — the hash still
		// disambiguates them.
		base = "workspace"
	}

	sum := sha256.Sum256([]byte(abs))
	return base + "-" + hex.EncodeToString(sum[:])[:projectHashLen]
}

// sanitizeNamePart keeps only characters Docker accepts inside a container
// name, collapsing every run of rejected characters into a single "-" and
// trimming separators from both ends.
func sanitizeNamePart(s string) string {
	var b strings.Builder
	lastWasDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == '-':
			if r == '-' && lastWasDash {
				continue
			}
			b.WriteRune(r)
			lastWasDash = r == '-'
		case !lastWasDash:
			b.WriteRune('-')
			lastWasDash = true
		}
	}
	return strings.Trim(b.String(), "-._")
}
