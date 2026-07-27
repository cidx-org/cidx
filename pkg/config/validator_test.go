package config

import (
	"strings"
	"testing"
)

// TestValidate_CustomContainerAccepted covers #142: a [containers.NAME] section
// with an `image` field declares a brand-new container, which the validator
// must accept even though it isn't a built-in preset.
func TestValidate_CustomContainerAccepted(t *testing.T) {
	cfg := &Config{
		Phases: map[string]Phase{
			"test": {Containers: []string{"pytest-mycustom"}},
		},
		Overrides: map[string]map[string]any{
			"pytest-mycustom": {
				"image":   "myorg/pytest:custom",
				"command": "pytest tests/",
			},
		},
	}

	result := Validate(cfg)
	if !result.Valid {
		t.Errorf("Validate() Valid = false, want true. Errors: %v", result.Errors)
	}
	if len(result.Errors) != 0 {
		t.Errorf("Validate() errors = %v, want empty", result.Errors)
	}
}

// TestValidate_BuiltinPresetStillAccepted ensures the standard preset-reference
// path still validates (backward compatibility).
func TestValidate_BuiltinPresetStillAccepted(t *testing.T) {
	cfg := &Config{
		Phases: map[string]Phase{
			"security": {Containers: []string{"trivy"}},
		},
	}

	result := Validate(cfg)
	if !result.Valid {
		t.Errorf("Validate() rejected built-in preset 'trivy'. Errors: %v", result.Errors)
	}
}

// TestValidate_UnknownContainerRejected ensures the validator still rejects a
// name that is neither a preset nor a custom declaration. The error message
// must mention both options so the user knows the fix.
func TestValidate_UnknownContainerRejected(t *testing.T) {
	cfg := &Config{
		Phases: map[string]Phase{
			"test": {Containers: []string{"does-not-exist"}},
		},
	}

	result := Validate(cfg)
	if result.Valid {
		t.Fatal("Validate() Valid = true for unknown container, want false")
	}
	if len(result.Errors) == 0 {
		t.Fatal("Validate() Errors empty, want at least one")
	}
	msg := result.Errors[0]
	if !strings.Contains(msg, "does-not-exist") {
		t.Errorf("error message missing container name: %q", msg)
	}
	if !strings.Contains(msg, "image") {
		t.Errorf("error message should mention `image` field as the fix: %q", msg)
	}
}

// TestValidate_OverrideOnlySectionWithoutImageRejected covers the edge case
// where the user added [containers.NAME] for an override but `image` is
// missing AND the name isn't a preset — this is a typo, not a declaration.
func TestValidate_OverrideOnlySectionWithoutImageRejected(t *testing.T) {
	cfg := &Config{
		Phases: map[string]Phase{
			"test": {Containers: []string{"pytest-typo"}},
		},
		Overrides: map[string]map[string]any{
			"pytest-typo": {
				"command": "pytest tests/",
				// Note: no `image` field — this is a misconfiguration.
			},
		},
	}

	result := Validate(cfg)
	if result.Valid {
		t.Fatal("Validate() accepted override-only section with no image and no matching preset, want rejection")
	}
}

// TestCheckVersion covers #212: required_version is a floor, and the "v2.1.4"
// spelling of every tag and doc example must match the "2.1.4" a release binary
// reports.
func TestCheckVersion(t *testing.T) {
	tests := []struct {
		name     string
		required string
		current  string
		wantErr  string // substring, empty means the run must be allowed
	}{
		{name: "no requirement", required: "", current: "2.1.4"},
		{name: "dev build bypasses", required: "9.9.9", current: "dev"},
		{name: "same version", required: "2.1.4", current: "2.1.4"},
		{name: "required carries the v prefix", required: "v2.1.4", current: "2.1.4"},
		{name: "current carries the v prefix", required: "2.1.4", current: "v2.1.4"},
		{name: "both carry the v prefix", required: "v2.1.4", current: "v2.1.4"},
		{name: "newer patch satisfies the floor", required: "v2.1.4", current: "2.1.9"},
		{name: "newer minor satisfies the floor", required: "2.1.4", current: "2.2.0"},
		{name: "newer major satisfies the floor", required: "2.1.4", current: "3.0.0"},
		{name: "older patch is refused", required: "2.1.4", current: "2.1.3", wantErr: "requires 2.1.4 or newer"},
		{name: "older minor is refused", required: "v2.2.0", current: "2.1.9", wantErr: "cidx 2.1.9 is too old"},
		{name: "older major is refused", required: "3.0.0", current: "2.9.9", wantErr: "too old"},
		{name: "malformed requirement is named", required: "latest", current: "2.1.4", wantErr: `invalid required_version "latest"`},
		{name: "incomplete requirement is named", required: "v2.1", current: "2.1.4", wantErr: "invalid required_version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckVersion(&Config{RequiredVersion: tt.required}, tt.current)

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("CheckVersion(%q, %q) = %v, want no error", tt.required, tt.current, err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("CheckVersion(%q, %q) = nil, want error containing %q", tt.required, tt.current, tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("CheckVersion(%q, %q) = %v, want error containing %q", tt.required, tt.current, err, tt.wantErr)
			}
		})
	}
}
