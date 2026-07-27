package executor

import (
	"strings"
	"testing"
)

// stubPodmanOnPath replaces the Podman CLI detection for the duration of a test.
func stubPodmanOnPath(t *testing.T, installed bool) {
	t.Helper()
	orig := podmanOnPath
	t.Cleanup(func() { podmanOnPath = orig })
	podmanOnPath = func() bool { return installed }
}

func TestSelectPodman_NoSocket_HonestError(t *testing.T) {
	s := &Selector{} // podman nil: the API socket was not found
	_, err := s.Select("trivy", BackendPodman)
	if err == nil {
		t.Fatal("expected error when podman executor is unavailable")
	}
	if strings.Contains(err.Error(), "not yet implemented") {
		t.Errorf("error %q still claims Podman is unimplemented", err.Error())
	}
	if !strings.Contains(err.Error(), "socket") {
		t.Errorf("error %q should explain the missing Podman API socket", err.Error())
	}
	if !strings.Contains(err.Error(), PodmanSocketHint()) {
		t.Errorf("error %q should include the socket remedy %q", err.Error(), PodmanSocketHint())
	}
}

func TestBuildUnavailableError_PodmanInstalledWithoutSocket(t *testing.T) {
	stubPodmanOnPath(t, true)

	s := &Selector{} // no docker, no podman executor
	err := s.buildUnavailableError()
	msg := err.Error()

	if !strings.Contains(msg, "Docker is not installed or not accessible") {
		t.Errorf("message %q should describe Docker state", msg)
	}
	if !strings.Contains(msg, "Podman is installed, but cidx cannot use it") {
		t.Errorf("message %q should describe the unusable Podman", msg)
	}
	if !strings.Contains(msg, PodmanSocketHint()) {
		t.Errorf("message %q should include the socket remedy %q", msg, PodmanSocketHint())
	}
}

func TestBuildUnavailableError_NoPodman(t *testing.T) {
	stubPodmanOnPath(t, false)

	s := &Selector{}
	err := s.buildUnavailableError()
	msg := err.Error()

	if strings.Contains(msg, "podman") || strings.Contains(msg, "Podman") {
		t.Errorf("message %q should not suggest Podman when it is not installed", msg)
	}
	if !strings.Contains(msg, "systemctl start docker") {
		t.Errorf("message %q should keep the Docker remedies", msg)
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
