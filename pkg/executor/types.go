package executor

import (
	"context"

	"github.com/cidx-org/cidx/pkg/config"
)

// Executor defines the interface for executing tools
// Implementations include DockerExecutor, NativeExecutor, PodmanExecutor, etc.
type Executor interface {
	// Run executes a tool with the given configuration
	Run(ctx context.Context, config *config.ContainerConfig) error

	// Available checks if this executor backend is available
	// For Docker: checks if daemon is running
	// For Native: always returns true (individual tool check via CanRun)
	Available() bool

	// Name returns the executor backend name (docker, native, podman, etc.)
	Name() string

	// Close releases any resources held by the executor
	Close() error
}

// NativeCapable is an optional interface for executors that support native execution
// Native executors can run tools directly without containers
type NativeCapable interface {
	Executor

	// CanRun checks if a specific tool can be run natively
	// Returns true if the tool binary is installed and available
	CanRun(toolName string) bool

	// GetInstallHint returns installation instructions for a tool
	GetInstallHint(toolName string) string
}

// ExecutionContext provides runtime context for executors
type ExecutionContext struct {
	DryRun    bool   // If true, show what would be executed without running
	Verbose   bool   // If true, show detailed output
	Workspace string // Project workspace path
}

// BackendType represents the executor backend type
type BackendType string

const (
	BackendAuto   BackendType = "auto"
	BackendDocker BackendType = "docker"
	BackendNative BackendType = "native"
	BackendPodman BackendType = "podman"
)

// ParseBackendType converts a string to BackendType
func ParseBackendType(s string) BackendType {
	switch s {
	case "docker":
		return BackendDocker
	case "native":
		return BackendNative
	case "podman":
		return BackendPodman
	default:
		return BackendAuto
	}
}

// String returns the string representation of BackendType
func (b BackendType) String() string {
	return string(b)
}
