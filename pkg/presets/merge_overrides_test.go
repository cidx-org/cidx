package presets

import (
	"reflect"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestMergeWith_VolumesOverride documents the volumes-merge semantics for #143:
// TOML decoding through map[string]any yields []any for arrays, so the previous
// []string type assertion silently dropped overrides. Both shapes must work.
func TestMergeWith_VolumesOverride(t *testing.T) {
	tests := []struct {
		name         string
		presetVols   []string
		overrideVols any
		want         []string
	}{
		{
			name:         "user volumes override preset (TOML-decoded []any)",
			presetVols:   []string{"/preset:/preset"},
			overrideVols: []any{"/user:/user", "/extra:/extra"},
			want:         []string{"/user:/user", "/extra:/extra"},
		},
		{
			name:         "user volumes override preset ([]string in-process form)",
			presetVols:   []string{"/preset:/preset"},
			overrideVols: []string{"/user:/user"},
			want:         []string{"/user:/user"},
		},
		{
			name:         "non-string element rejects whole slice, preset preserved",
			presetVols:   []string{"/preset:/preset"},
			overrideVols: []any{"/user:/user", 42},
			want:         []string{"/preset:/preset"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Preset{Name: "test", Volumes: tt.presetVols}
			merged := p.MergeWith(map[string]any{"volumes": tt.overrideVols})
			if !reflect.DeepEqual(merged.Volumes, tt.want) {
				t.Errorf("Volumes = %v, want %v", merged.Volumes, tt.want)
			}
		})
	}
}

// TestMergeWith_EntrypointOverride covers the same []any vs []string story
// for entrypoint (#143).
func TestMergeWith_EntrypointOverride(t *testing.T) {
	p := &Preset{Name: "test", Entrypoint: []string{"/preset-entry"}}
	merged := p.MergeWith(map[string]any{
		"entrypoint": []any{"/bin/sh", "-c"},
	})
	want := []string{"/bin/sh", "-c"}
	if !reflect.DeepEqual(merged.Entrypoint, want) {
		t.Errorf("Entrypoint = %v, want %v", merged.Entrypoint, want)
	}
}

// TestMergeWith_WorkdirOverride locks down the workdir override path (#143
// covers this too — the original report said it was ignored; in fact strings
// flow through MergeWith correctly, but a regression test prevents future drift).
func TestMergeWith_WorkdirOverride(t *testing.T) {
	p := &Preset{Name: "test", Workdir: "/preset-wd"}
	merged := p.MergeWith(map[string]any{"workdir": "/user-wd"})
	if merged.Workdir != "/user-wd" {
		t.Errorf("Workdir = %q, want %q", merged.Workdir, "/user-wd")
	}
}

// TestMergeWith_TOMLEndToEnd_AllFields reproduces the exact arcker/flouz scenario
// from #143: a real cidx.toml fragment exercised through the TOML decoder, then
// through MergeWith. All documented overridable fields must apply.
func TestMergeWith_TOMLEndToEnd_AllFields(t *testing.T) {
	tomlBlob := `
[containers.pytest]
image = "user/pytest:custom"
command = "pytest --strict tests/"
workdir = "/app"
volumes = ["/host/code:/app", "/host/cache:/cache"]
entrypoint = ["/bin/sh", "-c"]
env = { PYTEST_ARGS = "-vv", DB_URL = "postgres://localhost" }
pull_policy = "always"
timeout = "10m"
`
	var raw map[string]any
	if err := toml.Unmarshal([]byte(tomlBlob), &raw); err != nil {
		t.Fatalf("toml decode: %v", err)
	}
	overrides := raw["containers"].(map[string]any)["pytest"].(map[string]any)

	p := &Preset{
		Name:    "pytest",
		Image:   "preset/pytest:default",
		Command: "pytest tests/",
		Workdir: "/workspace",
		Volumes: []string{"${WORKSPACE}:/workspace"},
		Env:     map[string]string{"PRESET_KEEP": "yes"},
	}
	merged := p.MergeWith(overrides)

	if merged.Image != "user/pytest:custom" {
		t.Errorf("Image not overridden: %q", merged.Image)
	}
	if merged.Command != "pytest --strict tests/" {
		t.Errorf("Command not overridden: %q", merged.Command)
	}
	if merged.Workdir != "/app" {
		t.Errorf("Workdir not overridden: %q", merged.Workdir)
	}
	wantVols := []string{"/host/code:/app", "/host/cache:/cache"}
	if !reflect.DeepEqual(merged.Volumes, wantVols) {
		t.Errorf("Volumes not overridden: %v, want %v", merged.Volumes, wantVols)
	}
	wantEntry := []string{"/bin/sh", "-c"}
	if !reflect.DeepEqual(merged.Entrypoint, wantEntry) {
		t.Errorf("Entrypoint not overridden: %v, want %v", merged.Entrypoint, wantEntry)
	}
	if merged.Env["PYTEST_ARGS"] != "-vv" {
		t.Errorf("env PYTEST_ARGS not overridden: %q", merged.Env["PYTEST_ARGS"])
	}
	if merged.Env["DB_URL"] != "postgres://localhost" {
		t.Errorf("env DB_URL not overridden: %q", merged.Env["DB_URL"])
	}
	if merged.Env["PRESET_KEEP"] != "yes" {
		t.Errorf("preset env key dropped: %q", merged.Env["PRESET_KEEP"])
	}
	if merged.PullPolicy != "always" {
		t.Errorf("PullPolicy not overridden: %q", merged.PullPolicy)
	}
	if merged.Timeout != "10m" {
		t.Errorf("Timeout not overridden: %q", merged.Timeout)
	}
}

// TestPresetFromOverrides_CustomDeclaration covers #142: a [containers.NAME]
// block with an `image` field declares a brand-new container (not a preset
// override). All documented fields propagate.
func TestPresetFromOverrides_CustomDeclaration(t *testing.T) {
	tomlBlob := `
[containers.pytest-mycustom]
phase = "test"
image = "myorg/pytest:custom"
command = "pytest tests/integration"
workdir = "/app"
volumes = ["${WORKSPACE}:/app"]
entrypoint = ["/bin/sh", "-c"]
env = { DB_URL = "postgres://test" }
pull_policy = "if-not-present"
timeout = "15m"
privileged = false
`
	var raw map[string]any
	if err := toml.Unmarshal([]byte(tomlBlob), &raw); err != nil {
		t.Fatalf("toml decode: %v", err)
	}
	overrides := raw["containers"].(map[string]any)["pytest-mycustom"].(map[string]any)

	if !IsCustomDeclaration(overrides) {
		t.Fatal("IsCustomDeclaration returned false on a section with `image`")
	}

	p := PresetFromOverrides("pytest-mycustom", overrides)

	if p.Name != "pytest-mycustom" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Phase != "test" {
		t.Errorf("Phase = %q", p.Phase)
	}
	if p.Image != "myorg/pytest:custom" {
		t.Errorf("Image = %q", p.Image)
	}
	if p.Command != "pytest tests/integration" {
		t.Errorf("Command = %q", p.Command)
	}
	if p.Workdir != "/app" {
		t.Errorf("Workdir = %q", p.Workdir)
	}
	wantVols := []string{"${WORKSPACE}:/app"}
	if !reflect.DeepEqual(p.Volumes, wantVols) {
		t.Errorf("Volumes = %v, want %v", p.Volumes, wantVols)
	}
	wantEntry := []string{"/bin/sh", "-c"}
	if !reflect.DeepEqual(p.Entrypoint, wantEntry) {
		t.Errorf("Entrypoint = %v, want %v", p.Entrypoint, wantEntry)
	}
	if p.Env["DB_URL"] != "postgres://test" {
		t.Errorf("env DB_URL = %q", p.Env["DB_URL"])
	}
	if p.PullPolicy != "if-not-present" {
		t.Errorf("PullPolicy = %q", p.PullPolicy)
	}
	if p.Timeout != "15m" {
		t.Errorf("Timeout = %q", p.Timeout)
	}
}

// TestIsCustomDeclaration_NegativeCases ensures override-only sections (no image)
// are NOT treated as custom declarations — they still mean "override an existing
// preset". Backward compatibility for the historical behavior is critical (#143).
func TestIsCustomDeclaration_NegativeCases(t *testing.T) {
	tests := []struct {
		name      string
		overrides map[string]any
		want      bool
	}{
		{
			name:      "nil overrides",
			overrides: nil,
			want:      false,
		},
		{
			name: "override-only (severity, no image) — preset-override semantics",
			overrides: map[string]any{
				"severity": "HIGH,CRITICAL",
			},
			want: false,
		},
		{
			name: "image field with empty string — not a real declaration",
			overrides: map[string]any{
				"image": "",
			},
			want: false,
		},
		{
			name: "image is a non-string (malformed config) — not a declaration",
			overrides: map[string]any{
				"image": 42,
			},
			want: false,
		},
		{
			name: "image field with real value — custom declaration",
			overrides: map[string]any{
				"image": "myorg/img:latest",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCustomDeclaration(tt.overrides)
			if got != tt.want {
				t.Errorf("IsCustomDeclaration = %v, want %v", got, tt.want)
			}
		})
	}
}
