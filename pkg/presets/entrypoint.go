package presets

import (
	"fmt"
	"path"
	"strings"
)

// CommandRepeatsEntrypoint reports a command that hands its tool its own name.
//
// An image declaring ENTRYPOINT ["prettier"] runs `prettier <command>`, so the
// preset's command carries arguments only. Repeat the name and the tool gets it
// as its first argument: `unknown command "goreleaser" for "goreleaser
// release"`, `docker: unknown command: docker sh`, `Unknown argument:
// commitlint`. Three presets shipped exactly that, each for as long as it had
// existed (#278, #336).
//
// Nobody noticed because local_behavior short-circuits before the command
// reaches a container: draft and dry-run stop a local run from publishing, and
// they also stop it from ever exercising the command line. The presets most
// protected from doing damage were the least likely to be found broken, and the
// only place they ran for real was a release (issue #338).
//
// The comparison is against the base name of each entrypoint element, because
// an image spells its entrypoint as a path — `/kaniko/executor`,
// `/bin/shellcheck` — while a command names the tool. "--" is skipped: it
// belongs to wrappers like tini and is a separator, not a program.
//
// The entrypoint alone is not enough, which cost a first attempt at this rule.
// goreleaser declares `/sbin/tini -- /entrypoint.sh`: the tool it execs is
// named inside that script, nowhere the registry can be asked. Reverting the
// #336 fix and re-running the guard proved it — the guard stayed green on the
// bug it was written for. So the image repository answers where the entrypoint
// cannot: `goreleaser/goreleaser` runs goreleaser however its wrapper spells
// it. Both are consulted, and a preset is flagged when its command opens with
// either.
//
// A wrapper that execs its arguments is not a conflict and must not be read as
// one. The Ansible images declare `/opt/builder/bin/entrypoint dumb-init`, and
// their presets legitimately name `molecule`, `yamllint`, `ansible-playbook` —
// none of which is the entrypoint.
func CommandRepeatsEntrypoint(imageEntrypoint []string, image, command string) (repeated string, conflict bool) {
	repeated, _, conflict = commandConflict(imageEntrypoint, image, command)
	return repeated, conflict
}

// commandConflict also says which of the two answers matched, so the report can
// be accurate: naming the repository's tool as "the entrypoint" would be a
// small lie in exactly the case that is hardest to understand.
func commandConflict(imageEntrypoint []string, image, command string) (repeated, because string, conflict bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 || len(imageEntrypoint) == 0 {
		return "", "", false
	}

	first := fields[0]
	for _, element := range imageEntrypoint {
		if element == "--" {
			continue
		}
		if path.Base(element) == first {
			return first, fmt.Sprintf("the image's entrypoint (%s)", strings.Join(imageEntrypoint, " ")), true
		}
	}

	// An image that declares an entrypoint runs a tool; when the entrypoint is
	// a wrapper script the name of that tool is only legible in the repository
	// it is published under.
	if first == imageTool(image) {
		return first, fmt.Sprintf("the tool %s runs behind its entrypoint (%s)",
			imageTool(image), strings.Join(imageEntrypoint, " ")), true
	}

	return "", "", false
}

// imageTool is the tool an image repository is named after: the last path
// element, without the tag or digest. `goreleaser/goreleaser` -> "goreleaser",
// `ghcr.io/securego/gosec` -> "gosec".
func imageTool(image string) string {
	ref := image
	if at := strings.Index(ref, "@"); at >= 0 {
		ref = ref[:at]
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref = ref[:colon]
	}

	return path.Base(ref)
}

// EntrypointConflict phrases the conflict the way the catalogue guard reports
// it, naming both sides so the fix is readable without opening the image.
func EntrypointConflict(preset Preset) (string, bool) {
	// A preset that clears or replaces the entrypoint answers for its own
	// command line: `commitizen` and `gh-release` set [""] precisely so they
	// can run a shell, and `shellcheck` sets ["sh", "-c"]. The image's own
	// entrypoint no longer runs, so it cannot be repeated.
	if len(preset.Entrypoint) > 0 {
		return "", false
	}

	repeated, because, conflict := commandConflict(preset.ImageEntrypoint, preset.Image, preset.Command)
	if !conflict {
		return "", false
	}

	return fmt.Sprintf(
		"%q starts with %q, which is already %s: the tool would receive its own name as its first argument. "+
			"Pass arguments only, or clear the entrypoint with `entrypoint = [\"\"]` if the preset needs a shell",
		preset.Command, repeated, because), true
}
