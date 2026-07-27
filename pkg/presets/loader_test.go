package presets

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParsePresetsDataDecodesPullPolicyAndTimeout covers #203: both fields carry
// TOML tags on Preset and are consumed at execution time (pipeline.Runner copies
// them into ContainerConfig, which the Docker executor reads for the pull policy
// and the run deadline), but PresetTOML did not decode them — a custom preset
// setting either was silently truncated.
func TestParsePresetsDataDecodesPullPolicyAndTimeout(t *testing.T) {
	data := []byte(`
[presets.my-scanner]
name = "my-scanner"
phase = "security"
image = "alpine:3.20"
command = "echo scanning"
workdir = "/work"
volumes = ["${WORKSPACE}:/work"]
pull_policy = "always"
timeout = "45m"
`)

	registry, err := parsePresetsData(data, "test")
	if err != nil {
		t.Fatalf("parsePresetsData() error = %v", err)
	}

	preset, ok := registry["my-scanner"]
	if !ok {
		t.Fatalf("preset my-scanner not loaded, got %v", registry)
	}
	if preset.PullPolicy != "always" {
		t.Errorf("PullPolicy = %q, want %q", preset.PullPolicy, "always")
	}
	if preset.Timeout != "45m" {
		t.Errorf("Timeout = %q, want %q", preset.Timeout, "45m")
	}
}

// TestPullPolicyAndTimeoutSurviveMergeWith checks the loaded values reach the
// merged preset the runner converts into a ContainerConfig, both when the user
// leaves them alone and when a [containers.X] override restates them.
func TestPullPolicyAndTimeoutSurviveMergeWith(t *testing.T) {
	data := []byte(`
[presets.my-scanner]
name = "my-scanner"
image = "alpine:3.20"
pull_policy = "never"
timeout = "45m"
`)

	registry, err := parsePresetsData(data, "test")
	if err != nil {
		t.Fatalf("parsePresetsData() error = %v", err)
	}
	preset := registry["my-scanner"]

	// No overrides: the preset's own values must be preserved.
	merged := preset.MergeWith(nil)
	if merged.PullPolicy != "never" || merged.Timeout != "45m" {
		t.Errorf("MergeWith(nil) = (%q, %q), want (%q, %q)",
			merged.PullPolicy, merged.Timeout, "never", "45m")
	}

	// [containers.X] overrides still win over the preset value.
	merged = preset.MergeWith(map[string]any{"pull_policy": "always", "timeout": "5m"})
	if merged.PullPolicy != "always" || merged.Timeout != "5m" {
		t.Errorf("MergeWith(overrides) = (%q, %q), want (%q, %q)",
			merged.PullPolicy, merged.Timeout, "always", "5m")
	}
}

// TestParsePresetsDataWarnsOnUnknownKeys is the general form of #203: a key
// PresetTOML does not know about must not disappear without a word.
func TestParsePresetsDataWarnsOnUnknownKeys(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	data := []byte(`
[presets.typo-tool]
name = "typo-tool"
image = "alpine:3.20"
comand = "echo oops"
workdirr = "/work"
`)

	if _, err := parsePresetsData(data, "custom.toml"); err != nil {
		t.Fatalf("parsePresetsData() error = %v", err)
	}

	got := buf.String()
	for _, want := range []string{"custom.toml", "presets.typo-tool.comand", "presets.typo-tool.workdirr"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q does not mention %q", got, want)
		}
	}
}

// TestParsePresetsDataQuietOnKnownKeys guards against the warning firing on
// valid input — the built-in presets.toml must load without noise.
func TestParsePresetsDataQuietOnKnownKeys(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	if _, err := parsePresetsData(embeddedPresets, "embedded"); err != nil {
		t.Fatalf("parsePresetsData(embedded) error = %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("built-in presets produced warnings: %s", buf.String())
	}
}

// TestMergeOptionalPresetFileWarnsOnBrokenFile covers #210: a presets.toml that
// does not parse was dropped whole and in silence, so every custom preset in it
// disappeared and the user only saw "container X is not a built-in preset"
// later. The warning must name the file and carry the TOML error.
func TestMergeOptionalPresetFileWarnsOnBrokenFile(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	path := filepath.Join(t.TempDir(), "presets.toml")
	// Unterminated string: the file cannot be parsed at all.
	broken := []byte("[presets.my-scanner]\nname = \"my-scanner\"\nimage = \"alpine:3.20\ncommand = \"echo scanning\"\n")
	if err := os.WriteFile(path, broken, 0644); err != nil {
		t.Fatal(err)
	}

	registry := map[string]Preset{"trivy": {Name: "trivy"}}
	mergeOptionalPresetFile(registry, path)

	got := buf.String()
	if !strings.Contains(got, path) {
		t.Errorf("warning %q does not name the offending file %q", got, path)
	}
	if !strings.Contains(got, "toml") {
		t.Errorf("warning %q does not carry the TOML parse error", got)
	}

	// The broken file is still skipped -- built-ins must survive it.
	if _, ok := registry["trivy"]; !ok {
		t.Error("a broken preset file must not take the built-in presets with it")
	}
	if _, ok := registry["my-scanner"]; ok {
		t.Error("presets from an unparseable file must not be merged")
	}
}

// TestMergeOptionalPresetFileLoadsValidFileQuietly guards the other side: a
// good file merges, and does so without noise.
func TestMergeOptionalPresetFileLoadsValidFileQuietly(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	path := filepath.Join(t.TempDir(), "presets.toml")
	valid := []byte("[presets.my-scanner]\nname = \"my-scanner\"\nphase = \"security\"\nimage = \"alpine:3.20\"\n")
	if err := os.WriteFile(path, valid, 0644); err != nil {
		t.Fatal(err)
	}

	registry := map[string]Preset{}
	mergeOptionalPresetFile(registry, path)

	if _, ok := registry["my-scanner"]; !ok {
		t.Fatalf("valid preset file was not merged, got %v", registry)
	}
	if buf.Len() != 0 {
		t.Errorf("valid preset file produced output: %s", buf.String())
	}
}

// TestMergeOptionalPresetFileSilentWhenAbsent keeps the common case quiet:
// almost no project ships a .cidx/presets.toml.
func TestMergeOptionalPresetFileSilentWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	registry := map[string]Preset{}
	mergeOptionalPresetFile(registry, filepath.Join(t.TempDir(), "does-not-exist.toml"))

	if buf.Len() != 0 {
		t.Errorf("missing preset file produced output: %s", buf.String())
	}
}
