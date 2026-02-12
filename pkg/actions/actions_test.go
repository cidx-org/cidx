package actions

import (
	"testing"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0 B"},
		{"bytes", 500, "500 B"},
		{"one KB boundary", 1023, "1023 B"},
		{"one KB", 1024, "1.0 KB"},
		{"KB", 1536, "1.5 KB"},
		{"MB", 1048576, "1.0 MB"},
		{"GB", 1073741824, "1.0 GB"},
		{"large MB", 5242880, "5.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"longer than max", "hello world", 7, "hello.."},
		{"very short max", "hello", 3, "h.."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
