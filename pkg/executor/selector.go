package executor

import (
	"context"
	"errors"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/sirupsen/logrus"
)

// Selector manages executor selection based on availability and user preference
type Selector struct {
	docker  *DockerExecutor
	podman  *PodmanExecutor // Future: Podman support
	logger  *logrus.Logger
	dryRun  bool
	verbose bool
}

// NewSelector creates a new executor selector
func NewSelector(dryRun, verbose bool) (*Selector, error) {
	logger := logrus.New()
	if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	// Try to create Docker executor (may fail if Docker not installed)
	docker, dockerErr := NewDockerExecutor(dryRun, verbose)
	if dockerErr != nil {
		logger.Debugf("Docker executor unavailable: %v", dockerErr)
	}

	// TODO: Try to create Podman executor
	// podman, podmanErr := NewPodmanExecutor(dryRun, verbose)

	return &Selector{
		docker:  docker,
		podman:  nil, // Future: Podman support
		logger:  logger,
		dryRun:  dryRun,
		verbose: verbose,
	}, nil
}

// Select chooses the appropriate executor for a tool based on backend preference
func (s *Selector) Select(toolName string, backend BackendType) (Executor, error) {
	switch backend {
	case BackendDocker:
		return s.selectDocker()

	case BackendPodman:
		return s.selectPodman()

	default: // BackendAuto
		return s.selectAuto()
	}
}

// selectDocker forces Docker backend
func (s *Selector) selectDocker() (Executor, error) {
	if s.docker == nil {
		return nil, errors.New("docker client could not be initialized, is Docker installed")
	}

	if !s.docker.Available() {
		return nil, errors.New("docker daemon is not running, start Docker and try again")
	}

	return s.docker, nil
}

// selectPodman forces Podman backend
func (s *Selector) selectPodman() (Executor, error) {
	if s.podman == nil {
		return nil, errors.New("podman executor not yet implemented")
	}

	if !s.podman.Available() {
		return nil, errors.New("podman is not running, start Podman and try again")
	}

	return s.podman, nil
}

// selectAuto automatically selects the best available executor
func (s *Selector) selectAuto() (Executor, error) {
	// 1. Prefer Docker if available
	if s.docker != nil && s.docker.Available() {
		s.logger.Debugf("Auto-selected: Docker (daemon available)")
		return s.docker, nil
	}

	// 2. Try Podman if Docker not available
	if s.podman != nil && s.podman.Available() {
		s.logger.Debugf("Auto-selected: Podman (Docker unavailable)")
		return s.podman, nil
	}

	// 3. No executor available - provide helpful error
	return nil, s.buildUnavailableError()
}

// buildUnavailableError creates a helpful error message when no executor is available
func (s *Selector) buildUnavailableError() error {
	var msg string

	if s.docker == nil {
		msg = "Docker is not installed or not accessible."
	} else if !s.docker.Available() {
		msg = "Docker daemon is not running."
	}

	msg += "\n\nStart a container runtime:\n"
	msg += "  sudo systemctl start docker  # Docker on Linux\n"
	msg += "  open -a Docker               # Docker on macOS\n"
	msg += "  podman machine start         # Podman"

	return errors.New(msg)
}

// GetDocker returns the Docker executor (may be nil)
func (s *Selector) GetDocker() *DockerExecutor {
	return s.docker
}

// DockerAvailable checks if Docker is available
func (s *Selector) DockerAvailable() bool {
	return s.docker != nil && s.docker.Available()
}

// PodmanAvailable checks if Podman is available
func (s *Selector) PodmanAvailable() bool {
	return s.podman != nil && s.podman.Available()
}

// Close releases resources held by all executors
func (s *Selector) Close() error {
	if s.docker != nil {
		if err := s.docker.Close(); err != nil {
			return err
		}
	}
	if s.podman != nil {
		if err := s.podman.Close(); err != nil {
			return err
		}
	}
	return nil
}

// ListAvailableBackends returns which backends are currently available
func (s *Selector) ListAvailableBackends() []BackendType {
	var available []BackendType

	if s.docker != nil && s.docker.Available() {
		available = append(available, BackendDocker)
	}

	if s.podman != nil && s.podman.Available() {
		available = append(available, BackendPodman)
	}

	return available
}

// PodmanExecutor is a placeholder for future Podman support
// TODO: Implement PodmanExecutor with same interface as DockerExecutor
type PodmanExecutor struct{}

func (e *PodmanExecutor) Run(ctx context.Context, config *config.ContainerConfig) error {
	return errors.New("podman executor not yet implemented")
}
func (e *PodmanExecutor) Available() bool { return false }
func (e *PodmanExecutor) Name() string    { return "podman" }
func (e *PodmanExecutor) Close() error    { return nil }

// Ensure PodmanExecutor implements Executor interface
var _ Executor = (*PodmanExecutor)(nil)
