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
