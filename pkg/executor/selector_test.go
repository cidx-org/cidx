package executor

import (
	"context"
	"testing"
)

func TestPodmanExecutor_NotImplemented(t *testing.T) {
	p := &PodmanExecutor{}

	err := p.Run(context.TODO(), nil)
	if err == nil {
		t.Fatal("expected error from PodmanExecutor.Run")
	}
	if err.Error() != "podman executor not yet implemented" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPodmanExecutor_Available(t *testing.T) {
	p := &PodmanExecutor{}
	if p.Available() {
		t.Error("PodmanExecutor.Available() should return false")
	}
}

func TestPodmanExecutor_Name(t *testing.T) {
	p := &PodmanExecutor{}
	if p.Name() != "podman" {
		t.Errorf("PodmanExecutor.Name() = %q, want %q", p.Name(), "podman")
	}
}

func TestPodmanExecutor_Close(t *testing.T) {
	p := &PodmanExecutor{}
	if err := p.Close(); err != nil {
		t.Errorf("PodmanExecutor.Close() returned error: %v", err)
	}
}

func TestSelector_BuildUnavailableError_NilDocker(t *testing.T) {
	s := &Selector{
		docker: nil,
		podman: nil,
	}

	err := s.buildUnavailableError()
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if len(msg) == 0 {
		t.Error("expected non-empty error message")
	}
	// Should contain installation help
	if !contains(msg, "Docker") {
		t.Error("error should mention Docker")
	}
}

func TestSelector_PodmanAvailable_NilPodman(t *testing.T) {
	s := &Selector{podman: nil}
	if s.PodmanAvailable() {
		t.Error("PodmanAvailable() should return false when podman is nil")
	}
}

func TestSelector_DockerAvailable_NilDocker(t *testing.T) {
	s := &Selector{docker: nil}
	if s.DockerAvailable() {
		t.Error("DockerAvailable() should return false when docker is nil")
	}
}

func TestSelector_ListAvailableBackends_Empty(t *testing.T) {
	s := &Selector{docker: nil, podman: nil}
	backends := s.ListAvailableBackends()
	if len(backends) != 0 {
		t.Errorf("expected 0 available backends, got %d", len(backends))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
