package executor

import (
	"testing"
)

func TestNewSelector(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error creating selector: %v", err)
	}

	if selector == nil {
		t.Error("Expected non-nil selector")
	}
}

func TestSelector_ListAvailableBackends(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	backends := selector.ListAvailableBackends()
	// Should have at least one backend in a test environment
	// (usually Docker in CI)
	t.Logf("Available backends: %v", backends)
}

func TestSelector_SelectDocker(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !selector.DockerAvailable() {
		t.Skip("Docker not available, skipping")
	}

	executor, err := selector.Select("test-tool", BackendDocker)
	if err != nil {
		t.Fatalf("Unexpected error selecting docker: %v", err)
	}

	if executor.Name() != "docker" {
		t.Errorf("Expected docker executor, got %s", executor.Name())
	}
}

func TestSelector_SelectPodman(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if !selector.PodmanAvailable() {
		t.Skip("Podman not available, skipping")
	}

	executor, err := selector.Select("test-tool", BackendPodman)
	if err != nil {
		t.Fatalf("Unexpected error selecting podman: %v", err)
	}

	if executor.Name() != "podman" {
		t.Errorf("Expected podman executor, got %s", executor.Name())
	}
}

func TestSelector_SelectAuto(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// At least one backend should be available in test environment
	if !selector.DockerAvailable() && !selector.PodmanAvailable() {
		t.Skip("No container runtime available")
	}

	executor, err := selector.Select("test-tool", BackendAuto)
	if err != nil {
		t.Fatalf("Unexpected error with auto selection: %v", err)
	}

	name := executor.Name()
	if name != "docker" && name != "podman" {
		t.Errorf("Expected docker or podman, got %s", name)
	}
}

func TestSelector_Close(t *testing.T) {
	selector, err := NewSelector(false, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	err = selector.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}

func TestBackendType_Values(t *testing.T) {
	tests := []struct {
		backend BackendType
		value   string
	}{
		{BackendAuto, "auto"},
		{BackendDocker, "docker"},
		{BackendPodman, "podman"},
		{BackendKubernetes, "kubernetes"},
	}

	for _, tt := range tests {
		if string(tt.backend) != tt.value {
			t.Errorf("Expected %s, got %s", tt.value, tt.backend)
		}
	}
}
