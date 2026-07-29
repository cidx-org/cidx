package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkflow(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("name: CIDX CI\n"), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
	return path
}

func TestNormalizeWorkflowFile(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare name gets the conventional extension", "ci", "ci.yml"},
		{"a .yml name is left alone", "go-version-check.yml", "go-version-check.yml"},
		{"a .yaml name is left alone", "release.yaml", "release.yaml"},
		{"surrounding spaces are trimmed", "  ci  ", "ci.yml"},
		{"a dotted name is not mistaken for an extension", "container-monitor.v2", "container-monitor.v2.yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeWorkflowFile(tt.in); got != tt.want {
				t.Errorf("NormalizeWorkflowFile(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveWorkflowFile(t *testing.T) {
	t.Run("finds cidx.yml when it is the only candidate", func(t *testing.T) {
		dir := t.TempDir()
		want := writeWorkflow(t, dir, "cidx.yml")

		got, err := ResolveWorkflowFile(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("finds ci.yml when it is the only candidate", func(t *testing.T) {
		dir := t.TempDir()
		want := writeWorkflow(t, dir, "ci.yml")

		got, err := ResolveWorkflowFile(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("prefers cidx.yml when both exist", func(t *testing.T) {
		dir := t.TempDir()
		want := writeWorkflow(t, dir, "cidx.yml")
		writeWorkflow(t, dir, "ci.yml")

		got, err := ResolveWorkflowFile(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("got %q, want %q (cidx.yml must win over ci.yml)", got, want)
		}
	})

	t.Run("errors listing every candidate when none exists", func(t *testing.T) {
		dir := t.TempDir()

		_, err := ResolveWorkflowFile(dir)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
		for _, want := range []string{dir, "cidx.yml", "ci.yml"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q should mention %q", err, want)
			}
		}
	})
}
