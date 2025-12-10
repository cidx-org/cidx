package executor

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
)

// Selector manages executor selection based on availability and user preference
type Selector struct {
	docker  *DockerExecutor
	native  *NativeExecutor
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

	// Native executor is always available
	native := NewNativeExecutor(dryRun, verbose)

	return &Selector{
		docker:  docker,
		native:  native,
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

	case BackendNative:
		return s.selectNative(toolName)

	case BackendPodman:
		return nil, errors.New("Podman executor not yet implemented")

	default: // BackendAuto
		return s.selectAuto(toolName)
	}
}

// selectDocker forces Docker backend
func (s *Selector) selectDocker() (Executor, error) {
	if s.docker == nil {
		return nil, errors.New("Docker client could not be initialized. Is Docker installed?")
	}

	if !s.docker.Available() {
		return nil, errors.New("Docker daemon is not running. Start Docker and try again.")
	}

	return s.docker, nil
}

// selectNative forces native backend
func (s *Selector) selectNative(toolName string) (Executor, error) {
	if !s.native.CanRun(toolName) {
		hint := s.native.GetInstallHint(toolName)
		return nil, fmt.Errorf("tool '%s' not available natively.\nInstall: %s", toolName, hint)
	}

	return s.native, nil
}

// selectAuto automatically selects the best available executor
func (s *Selector) selectAuto(toolName string) (Executor, error) {
	// 1. Prefer Docker if available (consistent execution)
	if s.docker != nil && s.docker.Available() {
		s.logger.Debugf("Auto-selected: Docker (daemon available)")
		return s.docker, nil
	}

	// 2. Fall back to native if tool is installed locally
	if s.native.CanRun(toolName) {
		s.logger.Debugf("Auto-selected: Native (%s installed locally)", toolName)
		return s.native, nil
	}

	// 3. No executor available - provide helpful error
	return nil, s.buildUnavailableError(toolName)
}

// buildUnavailableError creates a helpful error message when no executor is available
func (s *Selector) buildUnavailableError(toolName string) error {
	var msg string

	// Check if Docker was the issue
	if s.docker == nil {
		msg = "Docker is not installed or not accessible.\n"
	} else if !s.docker.Available() {
		msg = "Docker daemon is not running.\n"
	}

	// Add native installation hint
	hint := s.native.GetInstallHint(toolName)
	msg += fmt.Sprintf("\nTo run '%s' natively, install it:\n  %s", toolName, hint)

	msg += "\n\nOr start Docker daemon:\n  sudo systemctl start docker  # Linux\n  open -a Docker                # macOS"

	return errors.New(msg)
}

// GetDocker returns the Docker executor (may be nil)
func (s *Selector) GetDocker() *DockerExecutor {
	return s.docker
}

// GetNative returns the Native executor
func (s *Selector) GetNative() *NativeExecutor {
	return s.native
}

// DockerAvailable checks if Docker is available
func (s *Selector) DockerAvailable() bool {
	return s.docker != nil && s.docker.Available()
}

// Close releases resources held by all executors
func (s *Selector) Close() error {
	if s.docker != nil {
		return s.docker.Close()
	}
	return nil
}

// ListAvailableBackends returns which backends are currently available
func (s *Selector) ListAvailableBackends() []BackendType {
	var available []BackendType

	if s.docker != nil && s.docker.Available() {
		available = append(available, BackendDocker)
	}

	// Native is always "available" but individual tools may not be installed
	available = append(available, BackendNative)

	return available
}

// ListAvailableNativeTools returns tools that can be run natively
func (s *Selector) ListAvailableNativeTools() []string {
	var tools []string
	for _, tool := range GetSupportedTools() {
		if s.native.CanRun(tool) {
			tools = append(tools, tool)
		}
	}
	return tools
}
