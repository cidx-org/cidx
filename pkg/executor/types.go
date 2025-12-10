package executor

import (
	"context"

	"github.com/cidx-org/cidx/pkg/config"
)

// Executor defines the interface for executing tools in containers
// Implementations include DockerExecutor, PodmanExecutor, KubernetesExecutor
type Executor interface {
	// Run executes a tool with the given configuration
	Run(ctx context.Context, config *config.ContainerConfig) error

	// Available checks if this executor backend is available
	// For Docker: checks if daemon is running
	// For Podman: checks if podman machine is running
	Available() bool

	// Name returns the executor backend name (docker, podman, kubernetes)
	Name() string

	// Close releases any resources held by the executor
	Close() error
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
	BackendAuto       BackendType = "auto"
	BackendDocker     BackendType = "docker"
	BackendPodman     BackendType = "podman"
	BackendKubernetes BackendType = "kubernetes"
)

// ParseBackendType converts a string to BackendType
func ParseBackendType(s string) BackendType {
	switch s {
	case "docker":
		return BackendDocker
	case "podman":
		return BackendPodman
	case "kubernetes", "k8s":
		return BackendKubernetes
	default:
		return BackendAuto
	}
}

// String returns the string representation of BackendType
func (b BackendType) String() string {
	return string(b)
}
