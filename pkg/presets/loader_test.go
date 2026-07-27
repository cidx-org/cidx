package presets

import (
	"bytes"
	"log"
	"os"
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
