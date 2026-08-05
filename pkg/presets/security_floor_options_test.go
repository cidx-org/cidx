package presets

import (
	"strings"
	"testing"
)

// TestSecurityFloorOptionsResolveToUsableFlags covers issue #341.
//
// A string option only works if the flag it names takes a value. bandit's
// severity and confidence pointed at `-ll` and `-i`, which are counters --
// `-l` is LOW, `-ll` MEDIUM, `-lll` HIGH -- so asking for a floor produced
// `bandit -r . -ll high`: the floor was MEDIUM whatever was requested, and
// `high` was left as a positional argument, which bandit reads as a path to
// scan. The option changed what was scanned and ignored what was asked.
//
// It is not a failure anything would have caught. The command is well formed,
// the container starts, bandit runs, and it reports nothing because the
// directory it was pointed at does not exist -- a green security phase that
// scanned nothing, the same silence as #271 and #367.
func TestSecurityFloorOptionsResolveToUsableFlags(t *testing.T) {
	tests := []struct {
		name      string
		preset    string
		overrides map[string]any
		want      []string
		reject    []string
	}{
		{
			name:      "bandit takes a severity level by value",
			preset:    "bandit",
			overrides: map[string]any{"severity": "high"},
			want:      []string{"--severity-level high"},
			reject:    []string{"-ll high", "-ll "},
		},
		{
			name:      "bandit takes a confidence level by value",
			preset:    "bandit",
			overrides: map[string]any{"confidence": "medium"},
			want:      []string{"--confidence-level medium"},
			reject:    []string{"-i medium", "-i "},
		},
		{
			name:      "gosec's flags already took their value",
			preset:    "gosec",
			overrides: map[string]any{"severity": "medium", "confidence": "medium"},
			want:      []string{"-severity medium", "-confidence medium"},
		},
		{
			name:      "cargo-audit's deny is a bare flag",
			preset:    "cargo-audit",
			overrides: map[string]any{"deny": true},
			want:      []string{"--deny warnings"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := Get(tt.preset)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", tt.preset, err)
			}

			command := preset.MergeWith(tt.overrides).Command

			for _, want := range tt.want {
				if !strings.Contains(command, want) {
					t.Errorf("resolved command is missing %q:\n%s", want, command)
				}
			}
			for _, reject := range tt.reject {
				if strings.Contains(command, reject) {
					t.Errorf("resolved command still carries the counter form %q, "+
						"which swallows the level and turns it into a path to scan (#341):\n%s",
						reject, command)
				}
			}
		})
	}
}

// TestSecurityFloorsAreNotDeclaredAsDefaults: the floors stay unset, so the
// catalogue reports what each tool reports (issue #341).
//
// The argument is asymmetry, not strictness. A default that reports too much is
// noisy, visible, and one line to fix by the person it annoys; a default that
// reports too little is invisible -- the phase is green and nobody learns why.
// The measurement made the trade concrete: on this repository, 114 files and
// 30,746 lines, gosec's proposed medium/medium floor dropped none of its 64
// findings, because its LOW band is empty in practice. The floor bought no
// quiet and kept the risk.
func TestSecurityFloorsAreNotDeclaredAsDefaults(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}

	// The commands that must stay free of a floor, and the flags that would
	// put one there.
	floors := map[string][]string{
		"gosec":  {"-severity", "-confidence"},
		"bandit": {"--severity-level", "--confidence-level"},
	}

	examined := 0
	for name, flags := range floors {
		preset, ok := catalogue[name]
		if !ok {
			t.Errorf("preset %q is no longer in the catalogue -- if it went on purpose, "+
				"drop it from this guard", name)
			continue
		}
		examined++

		for _, flag := range flags {
			if strings.Contains(preset.Command, flag) {
				t.Errorf("the %s preset ships a %s floor:\n  %s\n"+
					"A floor is policy about a codebase and belongs to that codebase, set "+
					"through [containers.%s] (#341).", name, flag, preset.Command, name)
			}
		}
	}

	if examined == 0 {
		t.Fatal("no preset examined — the guard would pass vacuously")
	}
}
