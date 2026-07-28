package presets

import "testing"

// TestNewerTagEdges covers what the Gherkin scenarios in
// features/presets/update_detection.feature would only make noisy: the shapes a
// real registry listing throws in around the rule.
func TestNewerTagEdges(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		available []string
		want      string
	}{
		{
			name:    "an empty listing offers nothing",
			current: "0.8.2",
		},
		{
			name:      "a four-component version compares component by component",
			current:   "1.2.3.4",
			available: []string{"1.2.3.5", "1.2.4.0", "1.10.0.0"},
			want:      "1.10.0.0",
		},
		{
			name:      "a suffix that is not a variant separator still has to match",
			current:   "29-cli",
			available: []string{"30", "30-cli-dev", "30-cli"},
			want:      "30-cli",
		},
		{
			name:      "a tag whose version overflows an int is skipped, not crashed on",
			current:   "1.0",
			available: []string{"99999999999999999999.0", "1.1"},
			want:      "1.1",
		},
		{
			name:      "a date-shaped tag stays in its own family",
			current:   "1.0",
			available: []string{"20260728", "1.1"},
			want:      "1.1",
		},
		{
			name:      "an older listing offers nothing",
			current:   "3.24",
			available: []string{"3.23", "3.22"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewerTag(tt.current, tt.available); got != tt.want {
				t.Errorf("NewerTag(%q, %v) = %q, want %q", tt.current, tt.available, got, tt.want)
			}
		})
	}
}
