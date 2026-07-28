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

// TestSupersedingVariant covers the frozen variant line (#252): the catalogue
// pins a family the repository has stopped publishing, so nothing newer will
// ever appear inside it and "up to date" is the wrong answer.
func TestSupersedingVariant(t *testing.T) {
	// The listing dhi.io/golang actually answers, trimmed to the shapes that
	// matter: no -alpine3.21-dev tag remains anywhere in it.
	golang := []string{
		"1", "1-alpine3.23-dev", "1-alpine3.24-dev",
		"1.25", "1.25-alpine3.23-dev", "1.25-alpine3.24-dev", "1.25-alpine3.24-fips-dev",
		"1.26", "1.26-alpine3.23-dev", "1.26-alpine3.24-dev",
	}

	tests := []struct {
		name      string
		current   string
		available []string
		want      string
	}{
		{
			name:      "a family the repository dropped names its successor",
			current:   "1.23-alpine3.21-dev",
			available: golang,
			want:      "-alpine3.24-dev",
		},
		{
			name:      "a family still published is not frozen, even at its own head",
			current:   "1.26-alpine3.24-dev",
			available: golang,
		},
		{
			name:      "a family still published behind its own head is not frozen either",
			current:   "1.25-alpine3.23-dev",
			available: golang,
		},
		{
			name:      "the successor keeps the rest of the suffix",
			current:   "1.23-alpine3.21-fips-dev",
			available: golang,
			want:      "-alpine3.24-fips-dev",
		},
		{
			name:      "a variant carrying no version has no line to lose",
			current:   "29-cli",
			available: []string{"30-cli-dev", "31-alpine3.24"},
		},
		{
			name:      "a bare version is not a variant line",
			current:   "3.21",
			available: []string{"3.23", "3.24"},
		},
		{
			name:      "a tag with no version at all is compared to nothing",
			current:   "latest",
			available: golang,
		},
		{
			name:      "an older family is no successor",
			current:   "3.13-alpine3.24",
			available: []string{"3.14-alpine3.21", "3.14-alpine3.23"},
		},
		{
			name:      "the newest family wins whatever order the registry listed",
			current:   "3.13-alpine3.21",
			available: []string{"3.14-alpine3.24", "3.13-alpine3.23", "3.14-alpine3.22"},
			want:      "-alpine3.24",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupersedingVariant(tt.current, tt.available); got != tt.want {
				t.Errorf("SupersedingVariant(%q, %v) = %q, want %q", tt.current, tt.available, got, tt.want)
			}
		})
	}
}
