package presets

import (
	"testing"

	"github.com/BurntSushi/toml"
)

// TestMergeWith_EnvOverride documents the env-merge semantics for #123:
// per-key, the user's [containers.X].env entry overrides the preset's
// matching key; preset keys not mentioned by the user are preserved.
func TestMergeWith_EnvOverride(t *testing.T) {
	tests := []struct {
		name      string
		presetEnv map[string]string
		// overrideEnv is stored as `any` so we can exercise both the
		// in-process map[string]string form and the TOML-decoded
		// map[string]any form.
		overrideEnv any
		want        map[string]string
	}{
		{
			name:        "user value overrides preset on key collision (map[string]string)",
			presetEnv:   map[string]string{"RUSTUP_HOME": "/tmp/rustup", "FOO": "preset-foo"},
			overrideEnv: map[string]string{"RUSTUP_HOME": "/usr/local/rustup"},
			want:        map[string]string{"RUSTUP_HOME": "/usr/local/rustup", "FOO": "preset-foo"},
		},
		{
			name:        "user value overrides preset on key collision (map[string]any, TOML-decoded form)",
			presetEnv:   map[string]string{"RUSTUP_HOME": "/tmp/rustup", "FOO": "preset-foo"},
			overrideEnv: map[string]any{"RUSTUP_HOME": "/usr/local/rustup"},
			want:        map[string]string{"RUSTUP_HOME": "/usr/local/rustup", "FOO": "preset-foo"},
		},
		{
			name:        "user keys not in preset are added",
			presetEnv:   map[string]string{"FOO": "preset-foo"},
			overrideEnv: map[string]any{"BAR": "user-bar"},
			want:        map[string]string{"FOO": "preset-foo", "BAR": "user-bar"},
		},
		{
			name:        "preset keys not mentioned by user are preserved",
			presetEnv:   map[string]string{"FOO": "preset-foo", "BAZ": "preset-baz"},
			overrideEnv: map[string]any{"FOO": "user-foo"},
			want:        map[string]string{"FOO": "user-foo", "BAZ": "preset-baz"},
		},
		{
			name:        "preset env nil, user adds keys",
			presetEnv:   nil,
			overrideEnv: map[string]any{"FOO": "user-foo"},
			want:        map[string]string{"FOO": "user-foo"},
		},
		{
			name:        "non-string scalar values are coerced (int)",
			presetEnv:   map[string]string{"COUNT": "1"},
			overrideEnv: map[string]any{"COUNT": int64(42)},
			want:        map[string]string{"COUNT": "42"},
		},
		{
			name:        "malformed nested override is ignored, preset env preserved",
			presetEnv:   map[string]string{"FOO": "preset-foo"},
			overrideEnv: map[string]any{"NESTED": map[string]any{"x": "y"}},
			want:        map[string]string{"FOO": "preset-foo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset := &Preset{
				Name: "test",
				Env:  tt.presetEnv,
			}
			overrides := map[string]any{"env": tt.overrideEnv}

			merged := preset.MergeWith(overrides)

			if len(merged.Env) != len(tt.want) {
				t.Fatalf("env length = %d, want %d (got=%v want=%v)",
					len(merged.Env), len(tt.want), merged.Env, tt.want)
			}
			for k, want := range tt.want {
				got, ok := merged.Env[k]
				if !ok {
					t.Errorf("env[%q] missing (got=%v)", k, merged.Env)
					continue
				}
				if got != want {
					t.Errorf("env[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

// TestMergeWith_EnvOverride_DoesNotMutatePreset guards against the shallow-copy
// hazard: Preset.MergeWith does `merged := *p` which shares the Env map. The
// fix must not write into the original preset's map.
func TestMergeWith_EnvOverride_DoesNotMutatePreset(t *testing.T) {
	original := map[string]string{"FOO": "preset-foo"}
	preset := &Preset{Name: "test", Env: original}

	_ = preset.MergeWith(map[string]any{
		"env": map[string]any{"FOO": "user-foo", "BAR": "user-bar"},
	})

	if got := original["FOO"]; got != "preset-foo" {
		t.Errorf("preset env was mutated: FOO=%q (want preset-foo)", got)
	}
	if _, leaked := original["BAR"]; leaked {
		t.Errorf("preset env was mutated: BAR leaked from override")
	}
}

// TestMergeWith_EnvOverride_TOMLEndToEnd reproduces the exact scenario from
// issue #123: a user cidx.toml with [containers.X].env entries should override
// the preset's matching keys after going through the real TOML decode path.
func TestMergeWith_EnvOverride_TOMLEndToEnd(t *testing.T) {
	tomlData := `
[containers.clippy]
env = { RUSTUP_HOME = "/usr/local/rustup", CARGO_HOME = "/usr/local/cargo" }
`
	var raw map[string]any
	if err := toml.Unmarshal([]byte(tomlData), &raw); err != nil {
		t.Fatalf("toml.Unmarshal: %v", err)
	}
	containers := raw["containers"].(map[string]any)
	clippyOverride := containers["clippy"].(map[string]any)

	preset := &Preset{
		Name: "clippy",
		Env: map[string]string{
			"RUSTUP_HOME": "/tmp/rustup", // would silently win pre-fix
			"OTHER":       "preset-only",
		},
	}

	merged := preset.MergeWith(clippyOverride)

	if got := merged.Env["RUSTUP_HOME"]; got != "/usr/local/rustup" {
		t.Errorf("RUSTUP_HOME = %q, want /usr/local/rustup (issue #123 regression)", got)
	}
	if got := merged.Env["CARGO_HOME"]; got != "/usr/local/cargo" {
		t.Errorf("CARGO_HOME = %q, want /usr/local/cargo", got)
	}
	if got := merged.Env["OTHER"]; got != "preset-only" {
		t.Errorf("OTHER = %q, want preset-only (preset key should survive)", got)
	}
}
