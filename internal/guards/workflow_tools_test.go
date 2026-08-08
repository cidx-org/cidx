package guards

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The tools a workflow runs are as much a supply chain as the images they
// inspect, and for a long time only one of the two was governed.
//
// `container-monitor.yml` and `security-audit.yml` ran `aquasec/trivy:latest`
// and `anchore/grype:latest` — ten invocations, none pinned — with the
// runner's docker config mounted and DOCKER_CONFIG pointed at it, so each
// container could read the DHI credentials. That alone is the substitution rule
// 1 of the supply-chain policy exists to refuse (#242), applied to the
// catalogue but not to the tools enforcing it.
//
// The sharper half is what those containers *are*. Their output is the
// promotion gate: a scanner that reported "no findings" would clear every
// candidate in the catalogue, and the differential verdict, the cooldown and
// the acceptances file would all agree with it. The whole policy rested on two
// images pinned by nothing.
//
// So the same rule now governs both: what a workflow runs is pinned by digest,
// and what it installs names a version.

// imageWithTag matches `registry/org/name:tag` carrying no digest. The `/` is
// required: it is what tells a container reference from the `key: value`,
// `$HOME/.docker:/mnt:ro` and `https://…` shapes a shell line is full of.
var imageWithTag = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*(/[a-z0-9._-]+)+:[A-Za-z0-9._-]+$`)

// pipInstall matches a pip install of a named package.
var pipInstall = regexp.MustCompile(`pip3?\s+install\s+([A-Za-z0-9._-]+)`)

// TestWorkflowToolsArePinned fails on a workflow that runs a mutable tool.
//
// The image is looked for across the whole `docker run` invocation rather than
// at the end of its last line: `docker run … aquasec/trivy:latest image \` puts
// the reference mid-line, followed by a subcommand, and an end-of-line rule
// misses five of the ten invocations this guard was written for — including
// every Trivy one. A guard that sees half the bug is worse than none, because
// it reports green.
func TestWorkflowToolsArePinned(t *testing.T) {
	for _, path := range workflowFiles(t) {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		name := filepath.Base(path)

		inDockerRun := false
		for i, line := range strings.Split(string(source), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue // a comment may discuss `:latest`; this file does
			}

			if strings.Contains(line, "docker run") {
				inDockerRun = true
			}
			if inDockerRun {
				for _, token := range strings.Fields(line) {
					token = strings.Trim(token, `"'\`)
					// `"$TRIVY_IMAGE"` is what a pinned invocation looks like
					// once the reference moves to a workflow env: no colon
					// survives the expansion, so it never reaches the match.
					if imageWithTag.MatchString(token) && !strings.Contains(token, "@sha256:") {
						t.Errorf("%s:%d runs %s, an image nothing pins — a tool that decides a promotion "+
							"must be as immutable as the catalogue it decides about (rule 1, #242)",
							name, i+1, token)
					}
				}
				// A `\` continues the command onto the next line.
				inDockerRun = strings.HasSuffix(trimmed, `\`)
			}

			if m := pipInstall.FindStringSubmatch(line); m != nil && !strings.Contains(line, "==") {
				t.Errorf("%s:%d installs %s with no version — whatever PyPI serves that morning "+
					"gets write access to presets.toml and to the pull request",
					name, i+1, m[1])
			}
		}
	}
}

// TestPinnedWorkflowImagesCarryATagAsWellAsADigest keeps the pins readable. A
// digest alone is immutable and says nothing about what it is; the tag is what
// makes a bump reviewable, which is the same reason the catalogue is written
// `image:tag@sha256:...` rather than `image@sha256:...`.
func TestPinnedWorkflowImagesCarryATagAsWellAsADigest(t *testing.T) {
	pinned := regexp.MustCompile(`([A-Za-z0-9._/-]+)(:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}`)

	for _, path := range workflowFiles(t) {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		name := filepath.Base(path)

		for i, line := range strings.Split(string(source), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "uses:") {
				continue // GitHub actions are pinned `owner/repo@sha`, a different form
			}
			for _, m := range pinned.FindAllStringSubmatch(line, -1) {
				if m[2] == "" {
					t.Errorf("%s:%d pins %s by digest alone — add the tag it was resolved from, "+
						"so the next bump is reviewable rather than a hex diff", name, i+1, m[1])
				}
			}
		}
	}
}

// workflowFiles lists the repository's GitHub workflows.
func workflowFiles(t *testing.T) []string {
	t.Helper()

	dir := filepath.Join(projectRoot, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dir, err)
	}

	var paths []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".yml") || strings.HasSuffix(entry.Name(), ".yaml") {
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	if len(paths) == 0 {
		t.Fatalf("no workflow found under %s — this guard would pass by watching nothing", dir)
	}
	return paths
}
