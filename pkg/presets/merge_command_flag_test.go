package presets

import (
	"strings"
	"testing"
)

// TestAppendCommandFlag covers #200: a command_flag appended after the closing
// quote of a `sh -c '<script>'` command is consumed by sh, never by the wrapped
// tool. Plain commands must keep appending at the end, unchanged.
func TestAppendCommandFlag(t *testing.T) {
	tests := []struct {
		name    string
		command string
		flag    string
		value   string
		want    string
	}{
		{
			name:    "plain command appends at the end",
			command: "trivy fs .",
			flag:    "--severity",
			value:   "HIGH,CRITICAL",
			want:    "trivy fs . --severity HIGH,CRITICAL",
		},
		{
			name:    "shell-wrapped command injects before the closing quote",
			command: "sh -c 'pip install --quiet mypy && mypy .'",
			flag:    "--strict",
			value:   "true",
			want:    "sh -c 'pip install --quiet mypy && mypy . --strict true'",
		},
		{
			name:    "shell-wrapped command with inner double quotes",
			command: `sh -c 'go mod tidy && echo "tidy"'`,
			flag:    "--flag",
			value:   "v",
			want:    `sh -c 'go mod tidy && echo "tidy" --flag v'`,
		},
		{
			name:    "double-quoted shell wrapper injects before the closing quote",
			command: `sh -c "pytest"`,
			flag:    "-v",
			value:   "true",
			want:    `sh -c "pytest -v true"`,
		},
		{
			name:    "two flags compose inside the same quotes",
			command: "sh -c 'pip install --quiet bandit && bandit -r . -ll low'",
			flag:    "-i",
			value:   "low",
			want:    "sh -c 'pip install --quiet bandit && bandit -r . -ll low -i low'",
		},
		{
			// An empty value means "switch, no value" (#214) — the flag lands
			// inside the quotes with nothing appended after it.
			name:    "empty value keeps the flag inside the quotes",
			command: "sh -c 'cargo-audit audit'",
			flag:    "--deny warnings",
			value:   "",
			want:    "sh -c 'cargo-audit audit --deny warnings'",
		},
		{
			name:    "unbalanced quoting falls back to appending at the end",
			command: "sh -c 'echo hi",
			flag:    "--flag",
			value:   "v",
			want:    "sh -c 'echo hi --flag v",
		},
		{
			name:    "unquoted shell script falls back to appending at the end",
			command: "sh -c echo",
			flag:    "--flag",
			value:   "v",
			want:    "sh -c echo --flag v",
		},
		{
			name:    "lone quote is not treated as a quote pair",
			command: "sh -c '",
			flag:    "--flag",
			value:   "v",
			want:    "sh -c ' --flag v",
		},
		{
			name:    "empty command keeps historical behaviour",
			command: "",
			flag:    "--flag",
			value:   "v",
			want:    " --flag v",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendCommandFlag(tt.command, tt.flag, tt.value); got != tt.want {
				t.Errorf("appendCommandFlag(%q, %q, %q) = %q, want %q",
					tt.command, tt.flag, tt.value, got, tt.want)
			}
		})
	}
}

// TestMergeWith_NoOptionLeavesCommandUntouched guards the "no flag, no change"
// case at the merge level: an override that matches no declared option must not
// touch Command at all.
func TestMergeWith_NoOptionLeavesCommandUntouched(t *testing.T) {
	preset := Preset{
		Name:    "demo",
		Command: "sh -c 'echo hello'",
		Options: map[string]Option{
			"strict": {Type: "bool", CommandFlag: "--strict"},
		},
	}

	merged := preset.MergeWith(map[string]any{"unknown": "value"})

	if merged.Command != preset.Command {
		t.Errorf("Command = %q, want unchanged %q", merged.Command, preset.Command)
	}
}

// TestMergeWith_ShellWrappedPresetOptions runs the fix against every built-in
// preset that is both shell-wrapped and option-bearing (#200 census). Setting
// every declared option must leave the command a single balanced `sh -c '...'`
// with all flags inside the quotes.
func TestMergeWith_ShellWrappedPresetOptions(t *testing.T) {
	for name, preset := range GlobalRegistry {
		if !strings.HasPrefix(preset.Command, "sh -c '") || len(preset.Options) == 0 {
			continue
		}

		t.Run(name, func(t *testing.T) {
			// Each option gets a value of its declared type: a boolean option
			// refuses "x" since #214, and an enabled one emits its flag alone.
			overrides := make(map[string]any, len(preset.Options))
			for optName, opt := range preset.Options {
				if opt.Type == "bool" {
					overrides[optName] = true
					continue
				}
				overrides[optName] = "x"
			}

			merged := preset.MergeWith(overrides)

			if !strings.HasSuffix(merged.Command, "'") {
				t.Fatalf("command lost its closing quote: %q", merged.Command)
			}
			for optName, opt := range preset.Options {
				if opt.CommandFlag == "" {
					continue
				}
				want := opt.CommandFlag + " x"
				if opt.Type == "bool" {
					want = opt.CommandFlag
				}
				flagPos := strings.Index(merged.Command, want)
				if flagPos == -1 {
					t.Fatalf("flag for option %q missing from command: %q", optName, merged.Command)
				}
				if flagPos > strings.LastIndex(merged.Command, "'") {
					t.Errorf("flag for option %q lands outside the sh -c quoting: %q", optName, merged.Command)
				}
			}
		})
	}
}

// TestMergeWith_CargoAuditDenyOption is the end-to-end regression from #200:
// the reported preset, the reported override, the exact resolved command.
// `deny` is declared type = "bool", so since #214 it is enabled with a boolean
// and the flag it carries ("--deny warnings") lands with nothing after it.
func TestMergeWith_CargoAuditDenyOption(t *testing.T) {
	preset, err := Get("cargo-audit")
	if err != nil {
		t.Fatalf("Get(cargo-audit): %v", err)
	}

	merged := preset.MergeWith(map[string]any{"deny": true})

	if strings.Contains(merged.Command, "--no-yanked' --deny") {
		t.Fatalf("--deny landed outside the sh -c quoting: %q", merged.Command)
	}
	if !strings.Contains(merged.Command, "/tmp/cargo-audit audit --no-yanked --deny warnings'") {
		t.Errorf("command = %q, want --deny injected before the closing quote", merged.Command)
	}
}
