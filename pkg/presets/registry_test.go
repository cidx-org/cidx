package presets

import (
	"testing"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		presetName string
		wantErr   bool
	}{
		{
			name:       "existing preset - trivy",
			presetName: "trivy",
			wantErr:    false,
		},
		{
			name:       "existing preset - gitleaks",
			presetName: "gitleaks",
			wantErr:    false,
		},
		{
			name:       "existing preset - prettier",
			presetName: "prettier",
			wantErr:    false,
		},
		{
			name:       "non-existent preset",
			presetName: "nonexistent",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, err := Get(tt.presetName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Get() expected error for preset %q, got nil", tt.presetName)
				}
				return
			}

			if err != nil {
				t.Errorf("Get() unexpected error: %v", err)
				return
			}

			if preset.Name != tt.presetName {
				t.Errorf("Get() preset.Name = %q, want %q", preset.Name, tt.presetName)
			}

			// Verify required fields are populated
			if preset.Image == "" {
				t.Errorf("Get() preset.Image is empty for %q", tt.presetName)
			}
			if preset.Command == "" {
				t.Errorf("Get() preset.Command is empty for %q", tt.presetName)
			}
			if preset.Workdir == "" {
				t.Errorf("Get() preset.Workdir is empty for %q", tt.presetName)
			}
			if len(preset.Volumes) == 0 {
				t.Errorf("Get() preset.Volumes is empty for %q", tt.presetName)
			}
		})
	}
}

func TestExists(t *testing.T) {
	tests := []struct {
		name       string
		presetName string
		want       bool
	}{
		{
			name:       "existing preset",
			presetName: "trivy",
			want:       true,
		},
		{
			name:       "non-existent preset",
			presetName: "nonexistent",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Exists(tt.presetName); got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	names := List()

	// Should have at least the basic presets we know about
	expectedPresets := []string{"trivy", "gitleaks", "prettier", "gofmt", "go-test", "go-build"}

	for _, expected := range expectedPresets {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing expected preset %q", expected)
		}
	}

	// Verify all returned names actually exist
	for _, name := range names {
		if !Exists(name) {
			t.Errorf("List() returned non-existent preset %q", name)
		}
	}
}

func TestListByPhase(t *testing.T) {
	tests := []struct {
		phase         string
		expectedTools []string
	}{
		{
			phase:         "security",
			expectedTools: []string{"trivy", "gitleaks"},
		},
		{
			phase:         "code",
			expectedTools: []string{"prettier", "gofmt"},
		},
		{
			phase:         "test",
			expectedTools: []string{"go-test"},
		},
		{
			phase:         "build",
			expectedTools: []string{"go-build"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			tools := ListByPhase(tt.phase)

			for _, expected := range tt.expectedTools {
				found := false
				for _, tool := range tools {
					if tool == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ListByPhase(%q) missing expected tool %q", tt.phase, expected)
				}
			}
		})
	}
}

func TestGroupByPhase(t *testing.T) {
	grouped := GroupByPhase()

	// Should have at least these phases
	expectedPhases := []string{"security", "code", "test", "build"}

	for _, phase := range expectedPhases {
		if tools, ok := grouped[phase]; !ok || len(tools) == 0 {
			t.Errorf("GroupByPhase() missing or empty phase %q", phase)
		}
	}

	// Verify all tools in groups actually exist
	for phase, tools := range grouped {
		for _, tool := range tools {
			preset, err := Get(tool)
			if err != nil {
				t.Errorf("GroupByPhase() tool %q in phase %q doesn't exist", tool, phase)
			}
			if preset.Phase != phase {
				t.Errorf("GroupByPhase() tool %q has phase %q but grouped in %q", tool, preset.Phase, phase)
			}
		}
	}
}
