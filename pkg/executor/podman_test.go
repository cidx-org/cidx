package executor

import (
	"os/exec"
	"testing"
)

func TestNewPodmanExecutor_WhenNotInstalled(t *testing.T) {
	// Skip if podman is actually installed
	if _, err := exec.LookPath("podman"); err == nil {
		t.Skip("Podman is installed, skipping 'not installed' test")
	}

	_, err := NewPodmanExecutor(false, false)
	if err == nil {
		t.Error("Expected error when podman is not installed")
	}
}

func TestNewPodmanExecutor_WhenInstalled(t *testing.T) {
	// Skip if podman is not installed
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("Podman not installed, skipping")
	}

	executor, err := NewPodmanExecutor(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if executor == nil {
		t.Error("Expected non-nil executor")
	}

	if executor.Name() != "podman" {
		t.Errorf("Expected name 'podman', got '%s'", executor.Name())
	}
}

func TestPodmanExecutor_Available(t *testing.T) {
	// Skip if podman is not installed
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("Podman not installed, skipping")
	}

	executor, err := NewPodmanExecutor(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// This will depend on whether podman is actually running
	// Just verify it doesn't panic
	_ = executor.Available()
}

func TestPodmanExecutor_Close(t *testing.T) {
	// Skip if podman is not installed
	if _, err := exec.LookPath("podman"); err != nil {
		t.Skip("Podman not installed, skipping")
	}

	executor, err := NewPodmanExecutor(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	err = executor.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

func TestPodmanExecutor_Name(t *testing.T) {
	// This test works even without podman installed
	// by directly instantiating the struct
	executor := &PodmanExecutor{}
	if executor.Name() != "podman" {
		t.Errorf("Expected name 'podman', got '%s'", executor.Name())
	}
}
