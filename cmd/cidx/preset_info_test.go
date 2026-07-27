package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/presets"
	"github.com/urfave/cli/v2"
)

// runPresetInfo runs `cidx preset info <name>` and returns what it printed.
func runPresetInfo(t *testing.T, name string) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w

	app := &cli.App{Commands: []*cli.Command{presetCommand()}}
	runErr := app.Run([]string{"cidx", "preset", "info", name})

	os.Stdout = old
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	if runErr != nil {
		t.Fatalf("preset info %s: %v", name, runErr)
	}
	return string(out)
}

// TestPresetInfo_ShowsPullPolicyAndTimeout covers #211: a custom preset can set
// both fields and they change how the container runs, but `preset info` used to
// stay silent — only `preset show`, which re-encodes the whole struct, revealed
// them.
func TestPresetInfo_ShowsPullPolicyAndTimeout(t *testing.T) {
	const name = "info-fields-fixture"
	presets.GlobalRegistry[name] = presets.Preset{
		Name:       name,
		Phase:      "security",
		Image:      "alpine:latest",
		Command:    "scan .",
		Workdir:    "/work",
		PullPolicy: "always",
		Timeout:    "45m",
	}
	t.Cleanup(func() { delete(presets.GlobalRegistry, name) })

	out := runPresetInfo(t, name)

	for _, want := range []string{"Pull policy: always", "Timeout: 45m"} {
		if !strings.Contains(out, want) {
			t.Errorf("preset info output missing %q:\n%s", want, out)
		}
	}
}

// TestPresetInfo_OmitsUnsetFields: a built-in preset sets neither field, and
// silence is the correct rendering — not an empty line.
func TestPresetInfo_OmitsUnsetFields(t *testing.T) {
	out := runPresetInfo(t, "trivy")

	for _, unwanted := range []string{"Pull policy:", "Timeout:"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("preset info should not mention %q when unset:\n%s", unwanted, out)
		}
	}
}
