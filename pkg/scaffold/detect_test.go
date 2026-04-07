package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetect_GoProject(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	d := Detect(dir)

	if !d.HasGit {
		t.Error("expected git detected")
	}
	if len(d.Languages) != 1 {
		t.Fatalf("expected 1 language, got %d", len(d.Languages))
	}
	if d.Languages[0].Name != "Go" {
		t.Errorf("expected Go, got %s", d.Languages[0].Name)
	}
}

func TestDetect_PythonProject(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(""), 0644)

	d := Detect(dir)

	if len(d.Languages) != 1 || d.Languages[0].Name != "Python" {
		t.Errorf("expected Python, got %v", d.Languages)
	}
}

func TestDetect_RustProject(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte(""), 0644)

	d := Detect(dir)

	if len(d.Languages) != 1 || d.Languages[0].Name != "Rust" {
		t.Errorf("expected Rust, got %v", d.Languages)
	}
}

func TestDetect_NoProject(t *testing.T) {
	dir := t.TempDir()
	d := Detect(dir)

	if len(d.Languages) != 0 {
		t.Errorf("expected no languages, got %d", len(d.Languages))
	}
	if d.HasGit {
		t.Error("expected no git")
	}
}

func TestDetect_MultiLanguage(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)

	d := Detect(dir)

	if len(d.Languages) != 2 {
		t.Fatalf("expected 2 languages, got %d", len(d.Languages))
	}
}

func TestGenerateTOML_Go(t *testing.T) {
	d := &Detection{
		Languages: []Language{{
			Name: "Go", Marker: "go.mod",
			Security: []string{"trivy", "gitleaks", "gosec"},
			Code:     []string{"golangci-lint", "gofmt", "prettier", "commitizen"},
			Test:     []string{"go-test"},
			Build:    []string{"go-build"},
		}},
		HasGit: true,
		Remote: "github",
	}

	output := GenerateTOML(d)

	checks := []string{
		"Auto-detected: Go project",
		"trivy",
		"golangci-lint",
		"go-test",
		"go-build",
		"[pipelines.ci]",
		"[pipelines.pr]",
	}

	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("expected %q in output", check)
		}
	}
}

func TestGenerateTOML_NoDetection(t *testing.T) {
	d := &Detection{}
	output := GenerateTOML(d)

	if !strings.Contains(output, "No project type detected") {
		t.Error("expected fallback config")
	}
}
