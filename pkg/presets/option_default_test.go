package presets

import (
	"bytes"
	"log"
	"os"
	"reflect"
	"strings"
	"testing"
)

// The option schema used to carry a `default` field. It was never applied:
// MergeWith only visits the keys present in the user's overrides, so the 45
// defaults the catalogue shipped went from declaration to nowhere — and two of
// them would have been harmful had they ever run (trivy's `--exit-code 0`
// never fails a scan; bandit's `-ll`/`-i` are counters and reject a value).
// The field is gone (#299); these tests are what stops it coming back.
//
// A re-added `default` in the shipped catalogue is caught by
// TestParsePresetsDataQuietOnKnownKeys, which requires presets.toml to load
// without a single unknown-key warning.

// TestOptionSchemaHasNoDefaultField locks the Go and TOML sides of the schema.
func TestOptionSchemaHasNoDefaultField(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Option{}),
		reflect.TypeOf(OptionTOML{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Name == "Default" {
				t.Errorf("%s declares a Default field: an option default is never applied, so put the value in the preset's command or env instead (#299)", typ.Name())
			}
			for _, tag := range []string{"toml", "yaml"} {
				if name, _, _ := strings.Cut(field.Tag.Get(tag), ","); name == "default" {
					t.Errorf("%s.%s maps to the %s key %q, removed in #299", typ.Name(), field.Name, tag, name)
				}
			}
		}
	}
}

// TestParsePresetsDataWarnsOnResidualOptionDefault is what makes the removal
// safe for someone whose own presets.toml still carries a `default`: the key is
// named as unknown (#203) instead of being accepted and quietly ignored, which
// is what happened while the field existed.
func TestParsePresetsDataWarnsOnResidualOptionDefault(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	data := []byte(`
[presets.my-scanner]
name = "my-scanner"
image = "alpine:3.20"
command = "scan ."

[presets.my-scanner.options.level]
type = "string"
default = "high"
command_flag = "--level"
`)

	registry, err := parsePresetsData(data, "custom.toml")
	if err != nil {
		t.Fatalf("parsePresetsData() error = %v", err)
	}

	got := buf.String()
	for _, want := range []string{"custom.toml", "presets.my-scanner.options.level.default"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning %q does not mention %q", got, want)
		}
	}

	// The rest of the option still loads: only the dead key is dropped.
	if flag := registry["my-scanner"].Options["level"].CommandFlag; flag != "--level" {
		t.Errorf("option level command_flag = %q, want %q", flag, "--level")
	}
}
