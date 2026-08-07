package presets

import (
	"os"
	"strings"
	"testing"
)

// TestExpandCommand covers the substitution the executor performs on a
// preset's command. Moved here with the function in #384: it belongs beside
// the presets whose placeholders it resolves.
func TestExpandCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		env     map[string]string
		want    string
	}{
		{
			"simple substitution",
			"trivy fs --severity ${SEVERITY} .",
			map[string]string{"SEVERITY": "HIGH"},
			"trivy fs --severity HIGH .",
		},
		{
			"no substitution",
			"trivy scan .",
			map[string]string{"UNUSED": "val"},
			"trivy scan .",
		},
		{
			"multiple substitutions",
			"${TOOL} ${ACTION}",
			map[string]string{"TOOL": "trivy", "ACTION": "scan"},
			"trivy scan",
		},
		{
			"shell command preserves structure",
			"sh -c 'echo ${MSG}'",
			map[string]string{"MSG": "hello"},
			"sh -c 'echo hello'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandCommand(tt.command, tt.env)
			if got != tt.want {
				t.Errorf("ExpandCommand(%q, %v) = %q, want %q", tt.command, tt.env, got, tt.want)
			}
		})
	}
}

// TestResolveEnvValue is issue #384: a preset's declared value is a default,
// and what the caller exported wins.
//
// `release.yml` set IMAGE_TAG to the release tag in front of `cidx run docker`
// and `nightly.yml` set it to `nightly`; both published `:latest`, because the
// declaration was the only value ever read. The registry carried exactly one
// tag on every version ever pushed.
func TestResolveEnvValue(t *testing.T) {
	t.Run("an exported value wins over the declaration", func(t *testing.T) {
		t.Setenv("IMAGE_TAG", "v3.0.0")

		if got := ResolveEnvValue("IMAGE_TAG", "latest"); got != "v3.0.0" {
			t.Errorf("ResolveEnvValue = %q, want the exported v3.0.0", got)
		}
	})

	t.Run("the declaration stands when nothing is exported", func(t *testing.T) {
		_ = os.Unsetenv("IMAGE_TAG")

		if got := ResolveEnvValue("IMAGE_TAG", "latest"); got != "latest" {
			t.Errorf("ResolveEnvValue = %q, want the declared latest", got)
		}
	})

	t.Run("a reference inside the declaration still resolves", func(t *testing.T) {
		_ = os.Unsetenv("IMAGE_NAME")
		t.Setenv("GITHUB_REPOSITORY", "cidx-org/cidx")

		got := ResolveEnvValue("IMAGE_NAME", "ghcr.io/${GITHUB_REPOSITORY}")
		if got != "ghcr.io/cidx-org/cidx" {
			t.Errorf("ResolveEnvValue = %q, want the expanded reference", got)
		}
	})

	t.Run("set but empty counts as set", func(t *testing.T) {
		t.Setenv("IMAGE_TAG", "")

		if got := ResolveEnvValue("IMAGE_TAG", "latest"); got != "" {
			t.Errorf("ResolveEnvValue = %q, want the exported empty value: falling back to "+
				"the default is how a mistyped tag published :latest in silence (#384)", got)
		}
	})
}

// TestExpandCommandTakesTheExportedValue is the whole chain on the command
// that broke: kaniko's destination, parameterised the way release.yml does it.
func TestExpandCommandTakesTheExportedValue(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", "cidx-org/cidx")
	t.Setenv("IMAGE_TAG", "v3.0.0")

	preset, err := Get("kaniko")
	if err != nil {
		t.Fatalf("Get(kaniko) error = %v", err)
	}

	got := ExpandCommand(preset.Command, preset.Env)
	if !strings.Contains(got, "--destination=ghcr.io/cidx-org/cidx:v3.0.0") {
		t.Errorf("resolved command = %q, want the exported tag in the destination", got)
	}
	if strings.Contains(got, ":latest") {
		t.Errorf("resolved command = %q, still carries the preset default", got)
	}
}
