package presets

import (
	"os"
	"sort"
	"strings"
	"testing"
)

// TestCommandRepeatsEntrypoint covers the rule of issue #338, on the three
// shapes that actually shipped broken and on the ones that must not be
// mistaken for them.
func TestCommandRepeatsEntrypoint(t *testing.T) {
	tests := []struct {
		name       string
		entrypoint []string
		image      string
		command    string
		want       bool
	}{
		{
			// `unknown command "goreleaser" for "goreleaser release"`, for as
			// long as the preset existed (#336).
			name:       "the tool named again, plainly",
			entrypoint: []string{"goreleaser"},
			image:      "goreleaser/goreleaser:v2.17.0",
			command:    "goreleaser release --clean",
			want:       true,
		},
		{
			// The image spells its entrypoint as a path, the command names the
			// tool: `/bin/shellcheck` and `shellcheck` are the same program.
			name:       "an entrypoint spelled as a path",
			entrypoint: []string{"/bin/shellcheck"},
			image:      "koalaman/shellcheck:stable",
			command:    "shellcheck script.sh",
			want:       true,
		},
		{
			name:       "arguments only, which is the correct shape",
			entrypoint: []string{"prettier"},
			image:      "jauderho/prettier:3.9.4",
			command:    "--check .",
			want:       false,
		},
		{
			// The Ansible images: a wrapper that execs its arguments. Naming
			// the tool is exactly right here, and reading it as a conflict
			// would flag seven working presets.
			name:       "a wrapper entrypoint is not the tool",
			entrypoint: []string{"/opt/builder/bin/entrypoint", "dumb-init"},
			image:      "ghcr.io/ansible/community-ansible-dev-tools:v26.7.1",
			command:    "molecule test -s ${SCENARIO}",
			want:       false,
		},
		{
			// tini's separator is not a program, so it can never be repeated.
			name:       "the -- of a tini wrapper",
			entrypoint: []string{"/sbin/tini", "--", "/entrypoint.sh"},
			image:      "goreleaser/goreleaser:v2.17.0",
			command:    "release --clean",
			want:       false,
		},
		{
			// A subcommand that merely resembles the image name: the docker
			// image's entrypoint is `docker`, and `buildx` is its subcommand.
			name:       "a subcommand of the tool",
			entrypoint: []string{"docker"},
			image:      "dhi.io/docker:29-cli",
			command:    "buildx build --push .",
			want:       false,
		},
		{
			// The case a first version of this rule missed: goreleaser's
			// entrypoint is a wrapper script, so the tool's name appears
			// nowhere the registry can be asked — only in the repository it is
			// published under (#338).
			name:       "a wrapper script that execs the tool the repository is named after",
			entrypoint: []string{"/sbin/tini", "--", "/entrypoint.sh"},
			image:      "goreleaser/goreleaser:v2.17.0",
			command:    "goreleaser release --clean",
			want:       true,
		},
		{
			name:       "an image declaring no entrypoint takes the whole argv",
			entrypoint: nil,
			image:      "golangci/golangci-lint:v2.12.2-alpine",
			command:    "golangci-lint run --timeout 5m",
			want:       false,
		},
		{
			name:       "an empty command",
			entrypoint: []string{"prettier"},
			image:      "jauderho/prettier:3.9.4",
			command:    "",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repeated, conflict := CommandRepeatsEntrypoint(tt.entrypoint, tt.image, tt.command)

			if conflict != tt.want {
				t.Fatalf("conflict = %v, want %v (repeated %q)", conflict, tt.want, repeated)
			}
			if conflict && repeated == "" {
				t.Error("a conflict must name what was repeated")
			}
		})
	}
}

// TestEntrypointConflictIgnoresAnOverriddenEntrypoint: a preset that clears or
// replaces the entrypoint answers for its own command line, and the image's
// entrypoint no longer runs — so it cannot be repeated. commitizen and
// gh-release set [""] precisely so they can wrap the tool in a shell.
func TestEntrypointConflictIgnoresAnOverriddenEntrypoint(t *testing.T) {
	preset := Preset{
		Name:            "commitizen",
		Image:           "commitizen/commitizen:4.16.5",
		ImageEntrypoint: []string{"cz"},
		Entrypoint:      []string{""},
		Command:         "sh -c 'cz check'",
	}

	if _, conflict := EntrypointConflict(preset); conflict {
		t.Error("a preset that clears the entrypoint cannot repeat it")
	}
}

// TestNoCatalogueCommandRepeatsItsEntrypoint is the standing guard, and the
// point of recording the entrypoint at all: the rule is checked offline, in the
// ordinary test suite, at the moment a preset is written.
//
// Before this, the only place these commands ran for real was a release —
// local_behavior short-circuits before the command reaches a container, so the
// presets most protected from doing damage were the least likely to be found
// broken (#338).
func TestNoCatalogueCommandRepeatsItsEntrypoint(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}

	checked := 0
	var conflicts []string
	for name, preset := range catalogue {
		if len(preset.ImageEntrypoint) == 0 {
			continue
		}
		checked++
		if reason, conflict := EntrypointConflict(preset); conflict {
			conflicts = append(conflicts, name+": "+reason)
		}
	}

	if checked == 0 {
		t.Fatal("no preset records an image entrypoint — the guard would pass vacuously")
	}

	if unrecorded := imagesWithNoRecordedEntrypoint(t); len(unrecorded) > 0 {
		// An image nobody has looked at is unguarded, and the count cannot be
		// taken from the decoded presets: `image_entrypoint = []` says "this
		// image declares none" and an absent key says "nobody looked", and Go
		// reads both as an empty slice. Only the file tells them apart.
		t.Logf("%d preset(s) checked; unguarded because no entrypoint was ever recorded: %s",
			checked, strings.Join(unrecorded, ", "))
	}
	if len(conflicts) > 0 {
		sort.Strings(conflicts)
		t.Errorf("these presets hand their tool its own name (#338):\n  %s", strings.Join(conflicts, "\n  "))
	}
}

// imagesWithNoRecordedEntrypoint names the catalogue images whose
// image_entrypoint has never been recorded, read from the file because the
// decoded form cannot distinguish "recorded as none" from "never looked at".
//
// The five dhi.io images are the ones outstanding: reading an entrypoint from
// that registry needs credentials, and the backfill was done without them.
// `cidx security registry login dhi.io` is what closes it.
func imagesWithNoRecordedEntrypoint(t *testing.T) []string {
	t.Helper()

	source, err := os.ReadFile("presets.toml")
	if err != nil {
		t.Fatalf("reading the catalogue: %v", err)
	}

	var missing []string
	seen := make(map[string]bool)
	for _, block := range strings.Split(string(source), "\n[presets.") {
		image := tomlValue(block, "image")
		if image == "" || strings.Contains(block, "\nimage_entrypoint = ") {
			continue
		}
		if name, _, _ := strings.Cut(image, "@"); !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	return missing
}

// tomlValue reads a top-level string assignment out of a preset block.
func tomlValue(block, key string) string {
	for _, line := range strings.Split(block, "\n") {
		if rest, found := strings.CutPrefix(line, key+" = \""); found {
			value, _, _ := strings.Cut(rest, "\"")
			return value
		}
	}

	return ""
}
