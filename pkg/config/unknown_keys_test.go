package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// loadWith writes a cidx.toml and loads it, returning the error verbatim.
func loadWith(t *testing.T, content string) error {
	t.Helper()

	path := filepath.Join(t.TempDir(), "cidx.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	_, err := Load(path)

	return err
}

// TestLoadRefusesKeysNothingReads covers issue #371.
//
// `stale_dayz = 15` used to leave stale_days at its default of 30 and say
// nothing, and `cidx validate` called the file valid — a typo was
// indistinguishable from the setting working. The check fails rather than
// warns, and on every command rather than only on validate: a warning about a
// config that does not mean what it says is a warning people scroll past, and
// the run that follows is the unattended behaviour the check exists to stop.
func TestLoadRefusesKeysNothingReads(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		refused string // the key the error must name; empty means the file is valid
	}{
		{
			name: "a typo in a known section",
			config: `
[branch]
stale_dayz = 15
`,
			refused: "branch.stale_dayz",
		},
		{
			// The payoff for the removals of v3.0.0. A config carrying a key
			// that was deleted used to get exactly the silence it got before
			// the deletion (#322).
			name: "a key removed in v3.0.0",
			config: `
[branch]
auto_cleanup = true
`,
			refused: "branch.auto_cleanup",
		},
		{
			name: "a typo in a preset option",
			config: `
[security]
containers = ["trivy"]

[containers.trivy]
severty = "HIGH"
`,
			refused: "containers.trivy.severty",
		},
		{
			name: "an option belonging to another preset",
			config: `
[security]
containers = ["trivy"]

[containers.trivy]
deny = true
`,
			refused: "containers.trivy.deny",
		},
		{
			name: "a stray key in a phase section",
			config: `
[security]
containers = ["trivy"]
paralell = true
`,
			refused: "security.paralell",
		},
		{
			name: "a real option of the named preset is accepted",
			config: `
[security]
containers = ["trivy"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
`,
		},
		{
			name: "a custom container declares itself",
			config: `
[format]
containers = ["prettier-write"]

[containers.prettier-write]
phase = "format"
image = "example/prettier:1"
command = "--write ."
workdir = "/work"
volumes = ["${WORKSPACE}:/work"]
`,
		},
		{
			// #352 landed first for this reason: strictness would otherwise
			// refuse this repository's own config on the five it writes.
			name: "a pipeline description is a real field now",
			config: `
[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]
description = "Full CI pipeline for all commits"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := loadWith(t, tt.config)

			if tt.refused == "" {
				if err != nil {
					t.Fatalf("expected the config to load, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected the config to be refused, it loaded")
			}
			if !strings.Contains(err.Error(), tt.refused) {
				t.Errorf("refusal does not name %q:\n%v", tt.refused, err)
			}
		})
	}
}

// TestLoadNamesEveryUnknownKeyAtOnce: a config with three mistakes should not
// need three runs to find them.
func TestLoadNamesEveryUnknownKeyAtOnce(t *testing.T) {
	err := loadWith(t, `
[security]
containers = ["trivy"]
paralell = true

[branch]
stale_dayz = 15

[containers.trivy]
severty = "HIGH"
`)
	if err == nil {
		t.Fatal("expected the config to be refused, it loaded")
	}

	for _, want := range []string{"branch.stale_dayz", "containers.trivy.severty", "security.paralell"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q:\n%v", want, err)
		}
	}
}

// TestTheProjectsOwnConfigIsAccepted: this repository's cidx.toml and the
// complete example the documentation ships both have to pass the rule they
// describe. The example is the more interesting of the two — it exists to name
// every key a user may write, so a key in it that cidx does not read would be
// documentation of something that does not work.
func TestTheProjectsOwnConfigIsAccepted(t *testing.T) {
	for _, path := range []string{
		filepath.Join(projectRoot, "cidx.toml"),
		filepath.Join(projectRoot, "examples", "cidx-complete.toml"),
	} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if _, err := Load(path); err != nil {
				t.Errorf("%s: %v", path, err)
			}
		})
	}
}
