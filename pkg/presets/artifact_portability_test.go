package presets

import (
	"strings"
	"testing"
)

// A build preset is the one kind whose output outlives its container. Every
// other phase reads the workspace and reports; a build writes an artifact that
// someone then runs somewhere else — another preset, a Docker image, a laptop,
// a production host.
//
// That crossing is where libc bites. `go build` with cgo on links against
// whatever libc the image ships; the go-build preset builds on Alpine, so it
// produced a musl-linked binary that would not start on Debian, Ubuntu, or any
// glibc host. The kernel reports the missing loader as ENOENT and the shell
// prints "not found" — naming the binary you just built, which exists (#195).
//
// It went unnoticed because nothing crosses that boundary inside this
// repository: `release.yml` cross-compiles natively with CGO_ENABLED=0 (#281)
// and every other `go build` here runs the binary on the machine that built it,
// where the libc necessarily matches. Only a user running `cidx run build` got
// the broken one.
//
// So each build preset states what its artifact needs to run elsewhere, and a
// new one cannot be added without saying which case it is. That is the point:
// the decision is cheap when the preset is written and expensive years later.

// artifactPortability classifies what a build preset leaves behind.
type artifactPortability int

const (
	// noNativeBinary: archives, wheels, collections. No loader, no libc, no
	// question to answer.
	noNativeBinary artifactPortability = iota

	// selfContained: an executable carrying everything it needs. Runs on any
	// kernel of the right architecture, including scratch images.
	selfContained

	// libcBound: an executable that resolves a libc at startup, so it runs only
	// where a compatible one exists. Legitimate, but it has to be stated where
	// the user reads it — the preset description — because nothing in the
	// failure message will.
	libcBound
)

// buildArtifacts is the answer for every build preset in the catalogue.
//
// A build preset missing from this map fails the test rather than defaulting:
// a default here would be a silent answer to the one question that costs an
// afternoon to debug.
var buildArtifacts = map[string]artifactPortability{
	"go-build":             selfContained,
	"cargo-build":          libcBound,
	"python-build":         noNativeBinary,
	"ansible-galaxy-build": noNativeBinary,
}

// TestEveryBuildPresetSaysWhatItsArtifactNeeds fails on a build preset that has
// not answered the question.
func TestEveryBuildPresetSaysWhatItsArtifactNeeds(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("failed to load the catalogue: %v", err)
	}

	seen := 0
	for name, preset := range catalogue {
		if preset.Phase != "build" {
			continue
		}
		seen++

		portability, classified := buildArtifacts[name]
		if !classified {
			t.Errorf("build preset %q does not say what its artifact needs to run outside this container.\n"+
				"Add it to buildArtifacts: noNativeBinary if it produces an archive, selfContained if the\n"+
				"executable carries its libc, libcBound if it does not -- and say so in its description (#419).", name)
			continue
		}

		switch portability {
		case selfContained:
			assertCarriesItsOwnLibc(t, name, preset)
		case libcBound:
			assertTheConstraintIsWrittenDown(t, name, preset)
		}
	}

	if seen == 0 {
		t.Fatal("no build preset found in the catalogue -- this guard would pass by watching nothing")
	}
}

// assertCarriesItsOwnLibc checks the flag that actually decides it, for the
// toolchains where a flag decides it.
func assertCarriesItsOwnLibc(t *testing.T, name string, preset Preset) {
	t.Helper()

	// Fail-closed on a toolchain this does not know. Returning quietly would let
	// a preset claim self-contained and be checked by nothing -- the shape of
	// every guard that reports green because it stopped looking.
	if !strings.Contains(preset.Command, "go build") {
		t.Errorf("%q claims to produce a self-contained artifact, but this guard knows no mechanism for\n"+
			"its toolchain (%q). Teach it one: for Go it is CGO_ENABLED=0, for Rust a musl --target.\n"+
			"Until then the claim is unchecked, which is worse than an unclassified preset.",
			name, preset.Command)
		return
	}

	if preset.Env["CGO_ENABLED"] != "0" {
		t.Errorf("%q builds with cgo enabled, so `go build` links against the libc of %s and the\n"+
			"artifact only starts on images shipping that same libc -- musl here, which fails on every\n"+
			"glibc host with \"not found\" naming a binary that exists.\n"+
			"Set CGO_ENABLED = \"0\" in [presets.%s.env], as release.yml has since #281.",
			name, preset.Image, name)
	}
}

// assertTheConstraintIsWrittenDown checks that a libc-bound artifact says so
// where someone hits the problem, since the runtime error will not.
func assertTheConstraintIsWrittenDown(t *testing.T, name string, preset Preset) {
	t.Helper()

	description := strings.ToLower(preset.Description)
	for _, said := range []string{"glibc", "musl", "libc", "static"} {
		if strings.Contains(description, said) {
			return
		}
	}

	t.Errorf("%q produces an executable tied to the libc of its image, and its description does not\n"+
		"say so. The failure it causes is a \"not found\" on a file that exists, which nobody debugs\n"+
		"quickly -- the description is the only place a user reads before hitting it (#195).", name)
}
