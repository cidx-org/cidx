package presets

import (
	"strings"
	"testing"
)

// TestMergeWith_BoolOptionEmitsFlagWithoutValue covers #214: a boolean option
// is a switch. Before the fix every option appended its stringified value, so
// `fix = true` produced "--fix true" — an argument golangci-lint reads as a
// target path and rejects.
func TestMergeWith_BoolOptionEmitsFlagWithoutValue(t *testing.T) {
	base := Preset{
		Name:    "demo",
		Command: "tool run",
		Options: map[string]Option{
			"fix":      {Type: "bool", CommandFlag: "--fix"},
			"severity": {Type: "string", CommandFlag: "--severity"},
			"jobs":     {Type: "int", CommandFlag: "--jobs"},
		},
	}

	tests := []struct {
		name      string
		overrides map[string]any
		want      string
	}{
		{
			name:      "native true emits the flag alone",
			overrides: map[string]any{"fix": true},
			want:      "tool run --fix",
		},
		{
			name:      "native false emits nothing",
			overrides: map[string]any{"fix": false},
			want:      "tool run",
		},
		{
			name:      `string "true" emits the flag alone`,
			overrides: map[string]any{"fix": "true"},
			want:      "tool run --fix",
		},
		{
			name:      `string "false" emits nothing`,
			overrides: map[string]any{"fix": "false"},
			want:      "tool run",
		},
		{
			name:      `string "1" is true`,
			overrides: map[string]any{"fix": "1"},
			want:      "tool run --fix",
		},
		{
			name:      `string "0" is false`,
			overrides: map[string]any{"fix": "0"},
			want:      "tool run",
		},
		{
			name:      "surrounding whitespace is tolerated",
			overrides: map[string]any{"fix": " true "},
			want:      "tool run --fix",
		},
		{
			name:      "a value that is not a boolean is refused, preset default kept",
			overrides: map[string]any{"fix": "yes"},
			want:      "tool run",
		},
		{
			name:      "a non-boolean type is refused too",
			overrides: map[string]any{"fix": 1},
			want:      "tool run",
		},
		{
			name:      "string options still carry their value",
			overrides: map[string]any{"severity": "HIGH"},
			want:      "tool run --severity HIGH",
		},
		{
			name:      "int options still carry their value",
			overrides: map[string]any{"jobs": 4},
			want:      "tool run --jobs 4",
		},
		{
			name:      "a bool and a valued option compose",
			overrides: map[string]any{"fix": true, "severity": "HIGH"},
			want:      "tool run --fix --severity HIGH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base.MergeWith(tt.overrides).Command
			// Multi-option cases have no guaranteed order (map iteration), so
			// compare the flag set rather than the raw string.
			if !sameCommandFlags(got, tt.want) {
				t.Errorf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

// sameCommandFlags compares two commands ignoring the order in which options
// were appended — MergeWith iterates a map.
func sameCommandFlags(got, want string) bool {
	gotFields := strings.Fields(got)
	wantFields := strings.Fields(want)
	if len(gotFields) != len(wantFields) {
		return false
	}
	counts := make(map[string]int, len(gotFields))
	for _, f := range gotFields {
		counts[f]++
	}
	for _, f := range wantFields {
		counts[f]--
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

// TestMergeWith_BoolOptionEnvVarKeepsValue documents the other half of the
// rule: an option mapped to an environment variable still receives a value —
// an env var is a value holder, not a switch. No built-in preset combines
// type = "bool" with env_var today, but the code path exists.
func TestMergeWith_BoolOptionEnvVarKeepsValue(t *testing.T) {
	preset := Preset{
		Name:    "demo",
		Command: "tool run",
		Options: map[string]Option{
			"race": {Type: "bool", EnvVar: "RACE", CommandFlag: "--race"},
		},
	}

	merged := preset.MergeWith(map[string]any{"race": false})

	if got := merged.Env["RACE"]; got != "false" {
		t.Errorf("Env[RACE] = %q, want %q", got, "false")
	}
	if merged.Command != "tool run" {
		t.Errorf("command = %q, want the flag omitted for a false boolean", merged.Command)
	}
}

// TestMergeWith_BuiltinBoolOptions is the regression net over the real
// registry: every built-in option declared type = "bool" must emit its flag
// with nothing after it when enabled, and nothing at all when disabled.
func TestMergeWith_BuiltinBoolOptions(t *testing.T) {
	seen := 0

	for name, preset := range GlobalRegistry {
		for optName, opt := range preset.Options {
			if opt.Type != "bool" || opt.CommandFlag == "" {
				continue
			}
			seen++

			t.Run(name+"/"+optName, func(t *testing.T) {
				// Enabling appends exactly " <flag>" — at the end for a plain
				// command, before the closing quote for a `sh -c '...'` one
				// (the placement rule locked in by #200). Nothing follows the
				// flag: "--fix true" is what #214 reported.
				appended := preset.Command + " " + opt.CommandFlag
				injected := appended
				if strings.HasPrefix(preset.Command, "sh -c '") && strings.HasSuffix(preset.Command, "'") {
					injected = preset.Command[:len(preset.Command)-1] + " " + opt.CommandFlag + "'"
				}

				enabled := preset.MergeWith(map[string]any{optName: true})
				if enabled.Command != appended && enabled.Command != injected {
					t.Errorf("enabled command = %q, want %q", enabled.Command, injected)
				}

				disabled := preset.MergeWith(map[string]any{optName: false})
				if disabled.Command != preset.Command {
					t.Errorf("disabled command = %q, want it untouched: %q", disabled.Command, preset.Command)
				}
			})
		}
	}

	if seen == 0 {
		t.Fatal("no built-in boolean option found — the census this test guards is empty")
	}
}

// TestMergeWith_GolangciLintFixOption is the reported case from #214, verbatim.
func TestMergeWith_GolangciLintFixOption(t *testing.T) {
	preset, err := Get("golangci-lint")
	if err != nil {
		t.Fatalf("Get(golangci-lint): %v", err)
	}

	merged := preset.MergeWith(map[string]any{"fix": true})

	if strings.Contains(merged.Command, "--fix true") {
		t.Fatalf("command = %q, want no value after --fix", merged.Command)
	}
	if !strings.HasSuffix(merged.Command, "--fix") {
		t.Errorf("command = %q, want it to end with --fix", merged.Command)
	}
}
