package executor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/sirupsen/logrus"
)

// PodmanExecutor executes tools using Podman CLI
type PodmanExecutor struct {
	logger  *logrus.Logger
	dryRun  bool
	verbose bool
}

// NewPodmanExecutor creates a new Podman executor
func NewPodmanExecutor(dryRun, verbose bool) (*PodmanExecutor, error) {
	// Check if podman is in PATH
	_, err := exec.LookPath("podman")
	if err != nil {
		return nil, fmt.Errorf("podman not found in PATH: %w", err)
	}

	logger := logrus.New()
	if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	return &PodmanExecutor{
		logger:  logger,
		dryRun:  dryRun,
		verbose: verbose,
	}, nil
}

// Run executes a tool configuration using Podman
func (e *PodmanExecutor) Run(ctx context.Context, containerConfig *config.ContainerConfig) error {
	e.logger.Infof("  ▸ Running [%s] %s (podman)", containerConfig.Phase, containerConfig.Name)

	// Expand environment variables in volumes and command
	volumes := expandVolumes(containerConfig.Volumes)
	command := expandCommand(containerConfig.Command, containerConfig.Env)

	if e.dryRun {
		e.printDryRun(containerConfig, volumes, command)
		return nil
	}

	// Pull image if needed
	if err := e.pullImage(ctx, containerConfig.Image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Run container
	if err := e.runContainer(ctx, containerConfig, volumes, command); err != nil {
		return err
	}

	e.logger.Infof("  ✓ %s completed", containerConfig.Name)
	return nil
}

// pullImage pulls a Podman image
func (e *PodmanExecutor) pullImage(ctx context.Context, imageName string) error {
	e.logger.Debugf("Pulling image: %s", imageName)

	var args []string
	if e.verbose {
		args = []string{"pull", imageName}
	} else {
		args = []string{"pull", "--quiet", imageName}
	}

	cmd := exec.CommandContext(ctx, "podman", args...)
	if e.verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("podman pull failed: %w", err)
	}

	return nil
}

// runContainer runs a container with the given configuration
func (e *PodmanExecutor) runContainer(ctx context.Context, containerConfig *config.ContainerConfig, volumes []string, command string) error {
	containerName := fmt.Sprintf("cidx_%s", containerConfig.Name)

	// Build podman run command
	args := []string{"run", "--rm", "--name", containerName}

	// Add working directory
	if containerConfig.Workdir != "" {
		args = append(args, "-w", containerConfig.Workdir)
	}

	// Add volumes
	for _, vol := range volumes {
		args = append(args, "-v", vol)
	}

	// Add environment variables
	for k, v := range containerConfig.Env {
		expandedValue := os.ExpandEnv(v)
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, expandedValue))
	}

	// Add user mapping (non-privileged containers)
	if !containerConfig.Privileged {
		args = append(args, "--userns=keep-id")
	}

	// Add entrypoint if specified
	if len(containerConfig.Entrypoint) > 0 {
		args = append(args, "--entrypoint", containerConfig.Entrypoint[0])
	}

	// Add image
	args = append(args, containerConfig.Image)

	// Add command
	if len(containerConfig.Entrypoint) > 0 {
		// With custom entrypoint, command is passed as single argument
		args = append(args, command)
	} else {
		// Parse command into parts
		cmdParts := parseCommand(command)
		args = append(args, cmdParts...)
	}

	e.logger.Debugf("Running: podman %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "podman", args...)

	// Stream stdout
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Stream stderr
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Stream output in goroutines
	done := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
		done <- struct{}{}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintln(os.Stderr, scanner.Text())
		}
		done <- struct{}{}
	}()

	// Wait for output streams to finish
	<-done
	<-done

	// Wait for container to exit
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("container exited with code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("container execution failed: %w", err)
	}

	return nil
}

// printDryRun prints what would be executed
func (e *PodmanExecutor) printDryRun(containerConfig *config.ContainerConfig, volumes []string, command string) {
	containerName := fmt.Sprintf("cidx_%s", containerConfig.Name)

	fmt.Printf("Would execute (podman):\n")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Tool: %s\n", containerConfig.Name)
	fmt.Printf("  Image: %s\n", containerConfig.Image)
	fmt.Printf("  Command: %s\n", command)
	fmt.Printf("  Workdir: %s\n", containerConfig.Workdir)
	fmt.Printf("  Volumes:\n")
	for _, vol := range volumes {
		fmt.Printf("    - %s\n", vol)
	}
	if len(containerConfig.Env) > 0 {
		fmt.Printf("  Environment:\n")
		for k, v := range containerConfig.Env {
			fmt.Printf("    %s=%s\n", k, v)
		}
	}
	fmt.Println()
}

// Available checks if Podman is installed and responsive
func (e *PodmanExecutor) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "podman", "version", "--format", "{{.Version}}")
	err := cmd.Run()
	return err == nil
}

// Name returns the executor backend name
func (e *PodmanExecutor) Name() string {
	return "podman"
}

// Close releases resources (no-op for CLI-based executor)
func (e *PodmanExecutor) Close() error {
	return nil
}

// Ensure PodmanExecutor implements Executor interface
var _ Executor = (*PodmanExecutor)(nil)
