package presets

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// overrideRead matches a key read out of a `[containers.<name>]` section by the
// code that consumes one: `overrides["image"]`.
var overrideRead = regexp.MustCompile(`overrides\["([a-z_]+)"\]`)

// TestOverrideKeysMatchTheReaders keeps overrideKeys honest against the two
// functions that actually read an override section.
//
// The list exists so cidx.toml can reject a key nothing will read (issue
// #371), which only works while it says the same thing as the readers. A list
// restating what code does elsewhere is exactly what drifts -- it is how
// bandit kept offering a flag that had stopped working (#341) and how the BDD
// steps kept offering a local_behavior that had been removed (#353). Here the
// list is checked against the source rather than trusted.
func TestOverrideKeysMatchTheReaders(t *testing.T) {
	src, err := os.ReadFile("types.go")
	if err != nil {
		t.Fatalf("reading types.go: %v", err)
	}

	read := make(map[string]bool)
	for _, match := range overrideRead.FindAllStringSubmatch(string(src), -1) {
		read[match[1]] = true
	}
	if len(read) == 0 {
		t.Fatal("no override read found in types.go — the guard would pass vacuously")
	}

	for key := range read {
		if !overrideKeys[key] {
			t.Errorf("types.go reads overrides[%q] and overrideKeys does not list it: "+
				"cidx.toml would reject a key that works (#371)", key)
		}
	}
	for key := range overrideKeys {
		if !read[key] {
			t.Errorf("overrideKeys lists %q and nothing in types.go reads it: "+
				"cidx.toml would accept a key that does nothing, which is the bug (#371)", key)
		}
	}
}

// TestUnknownOverrideKeys covers what a section may carry.
func TestUnknownOverrideKeys(t *testing.T) {
	tests := []struct {
		name    string
		preset  string
		section map[string]any
		want    []string
	}{
		{
			name:    "a structural override of a catalogue preset",
			preset:  "trivy",
			section: map[string]any{"image": "x", "command": "y", "timeout": "5m"},
		},
		{
			name:    "an option the preset declares",
			preset:  "trivy",
			section: map[string]any{"severity": "HIGH,CRITICAL", "exit_code": 1},
		},
		{
			name:    "a typo in an option name",
			preset:  "trivy",
			section: map[string]any{"severty": "HIGH"},
			want:    []string{"severty"},
		},
		{
			name:    "an option of another preset",
			preset:  "trivy",
			section: map[string]any{"deny": true},
			want:    []string{"deny"},
		},
		{
			name:    "a custom container declares itself with structural keys only",
			preset:  "prettier-write",
			section: map[string]any{"phase": "format", "image": "x", "command": "--write ."},
		},
		{
			name:    "config_files is read by nothing here",
			preset:  "trivy",
			section: map[string]any{"config_files": []any{".trivyignore"}},
			want:    []string{"config_files"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnknownOverrideKeys(tt.preset, tt.section)

			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Fatalf("unknown = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("unknown = %v, want %v", got, tt.want)
					break
				}
			}
		})
	}
}
