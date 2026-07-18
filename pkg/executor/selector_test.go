package executor

import (
	"testing"
)

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

func TestPodmanExecutor_NilInner(t *testing.T) {
	p := &PodmanExecutor{} // no inner executor
	if p.Available() {
		t.Error("PodmanExecutor without inner should not be available")
	}
}

func TestNewPodmanExecutor_NoSocket(t *testing.T) {
	// With no Podman installed, NewPodmanExecutor should fail gracefully
	_, err := NewPodmanExecutor(true, false, false)
	// Either succeeds (Podman socket found) or fails (not found)
	// We just verify it doesn't panic
	_ = err
}

func TestFindPodmanSocket_Candidates(t *testing.T) {
	candidates := podmanSocketCandidates()
	if len(candidates) == 0 {
		t.Error("expected at least one socket candidate path")
	}
}
