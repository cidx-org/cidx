package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/arcker/cidx/pkg/config"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sirupsen/logrus"
)

// DockerExecutor executes tools using Docker
type DockerExecutor struct {
	client  *client.Client
	logger  *logrus.Logger
	dryRun  bool
	verbose bool
}

// NewDockerExecutor creates a new Docker executor
func NewDockerExecutor(dryRun, verbose bool) (*DockerExecutor, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	logger := logrus.New()
	if verbose {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}

	return &DockerExecutor{
		client:  cli,
		logger:  logger,
		dryRun:  dryRun,
		verbose: verbose,
	}, nil
}

// Run executes a tool configuration
func (e *DockerExecutor) Run(ctx context.Context, toolConfig *config.ToolConfig) error {
	e.logger.Infof("  ▸ Running [%s] %s", toolConfig.Phase, toolConfig.Name)

	// Expand environment variables in volumes and command
	volumes := expandVolumes(toolConfig.Volumes)
	command := expandCommand(toolConfig.Command, toolConfig.Env)

	if e.dryRun {
		e.printDryRun(toolConfig, volumes, command)
		return nil
	}

	// Pull image if needed
	if err := e.pullImage(ctx, toolConfig.Image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Get or create container
	containerID, containerName, err := e.getOrCreateContainer(ctx, toolConfig, volumes, command)
	if err != nil {
		return fmt.Errorf("failed to get or create container: %w", err)
	}

	// Start container
	e.logger.Debugf("Starting container: %s", containerName)
	if err := e.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	// Stream logs
	if err := e.streamLogs(ctx, containerID); err != nil {
		return fmt.Errorf("failed to stream logs: %w", err)
	}

	// Wait for container to finish
	statusCh, errCh := e.client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("error waiting for container: %w", err)
		}
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return fmt.Errorf("container exited with code %d", status.StatusCode)
		}
	}

	e.logger.Infof("  ✓ %s completed", toolConfig.Name)
	return nil
}

// pullImage pulls a Docker image
func (e *DockerExecutor) pullImage(ctx context.Context, imageName string) error {
	e.logger.Debugf("Pulling image: %s", imageName)

	out, err := e.client.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			e.logger.Warnf("Failed to close image pull output: %v", closeErr)
		}
	}()

	// Consume output to ensure pull completes
	var copyErr error
	if e.verbose {
		_, copyErr = io.Copy(os.Stdout, out)
	} else {
		_, copyErr = io.Copy(io.Discard, out)
	}

	if copyErr != nil {
		return fmt.Errorf("failed to copy image pull output: %w", copyErr)
	}

	return nil
}

// getOrCreateContainer gets an existing container or creates a new one
func (e *DockerExecutor) getOrCreateContainer(ctx context.Context, toolConfig *config.ToolConfig, volumes []string, command string) (string, string, error) {
	containerName := fmt.Sprintf("cidx_%s", toolConfig.Name)

	// Try to find existing container
	filterArgs := filters.NewArgs()
	filterArgs.Add("name", containerName)

	containers, err := e.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})

	if err != nil {
		return "", "", fmt.Errorf("failed to list containers: %w", err)
	}

	// If container exists, reuse it (keeps cache like trivy DB)
	if len(containers) > 0 {
		existingContainer := containers[0]
		e.logger.Debugf("♻ Reusing container %s (preserves cache)", containerName)
		return existingContainer.ID, containerName, nil
	}

	// Container doesn't exist, create new one
	e.logger.Debugf("Creating new container: %s", containerName)
	return e.createContainer(ctx, toolConfig, volumes, command)
}

// createContainer creates a Docker container and returns containerID and containerName
func (e *DockerExecutor) createContainer(ctx context.Context, toolConfig *config.ToolConfig, volumes []string, command string) (string, string, error) {
	// Parse volumes into binds
	binds := make([]string, len(volumes))
	copy(binds, volumes)

	// Convert env map to slice
	env := make([]string, 0, len(toolConfig.Env))
	for k, v := range toolConfig.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Parse command
	cmdParts := parseCommand(command)
	if len(cmdParts) == 0 {
		return "", "", fmt.Errorf("empty command")
	}

	// Generate container name with cidx_ prefix (fixed name for reuse)
	containerName := fmt.Sprintf("cidx_%s", toolConfig.Name)

	containerConfig := &container.Config{
		Image:      toolConfig.Image,
		Cmd:        cmdParts,
		WorkingDir: toolConfig.Workdir,
		Env:        env,
		Labels: map[string]string{
			"managed-by": "cidx",
			"cidx.tool":  toolConfig.Name,
			"cidx.phase": toolConfig.Phase,
		},
	}

	// Only set user for non-privileged tools
	if !toolConfig.Privileged {
		containerConfig.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}

	hostConfig := &container.HostConfig{
		Binds:      binds,
		AutoRemove: false, // We'll remove manually
	}

	resp, err := e.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return "", "", err
	}

	return resp.ID, containerName, nil
}

// streamLogs streams container logs to stdout/stderr
func (e *DockerExecutor) streamLogs(ctx context.Context, containerID string) error {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	}

	out, err := e.client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); closeErr != nil {
			e.logger.Warnf("Failed to close container logs: %v", closeErr)
		}
	}()

	_, err = stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	return err
}

// printDryRun prints what would be executed
func (e *DockerExecutor) printDryRun(toolConfig *config.ToolConfig, volumes []string, command string) {
	containerName := fmt.Sprintf("cidx_%s", toolConfig.Name)

	fmt.Printf("Would execute:\n")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Tool: %s\n", toolConfig.Name)
	fmt.Printf("  Image: %s\n", toolConfig.Image)
	fmt.Printf("  Command: %s\n", command)
	fmt.Printf("  Workdir: %s\n", toolConfig.Workdir)
	fmt.Printf("  Volumes:\n")
	for _, vol := range volumes {
		fmt.Printf("    - %s\n", vol)
	}
	if len(toolConfig.Env) > 0 {
		fmt.Printf("  Environment:\n")
		for k, v := range toolConfig.Env {
			fmt.Printf("    %s=%s\n", k, v)
		}
	}
	fmt.Println()
}

// parseCommand parses a command string into parts, handling shell commands specially
func parseCommand(command string) []string {
	// Trim whitespace
	command = strings.TrimSpace(command)

	// Check if it's a shell command (sh -c 'script')
	if strings.HasPrefix(command, "sh -c ") {
		// Extract the script part after "sh -c "
		script := strings.TrimPrefix(command, "sh -c ")
		script = strings.TrimSpace(script)

		// Remove surrounding quotes if present
		if len(script) >= 2 {
			if (script[0] == '\'' && script[len(script)-1] == '\'') ||
				(script[0] == '"' && script[len(script)-1] == '"') {
				script = script[1 : len(script)-1]
			}
		}

		return []string{"sh", "-c", script}
	}

	// For non-shell commands, use standard field splitting
	return strings.Fields(command)
}

// expandVolumes expands environment variables in volume mounts
func expandVolumes(volumes []string) []string {
	expanded := make([]string, len(volumes))
	for i, vol := range volumes {
		expanded[i] = os.ExpandEnv(vol)
	}
	return expanded
}

// expandCommand expands environment variables in command
func expandCommand(command string, env map[string]string) string {
	expanded := command
	for k, v := range env {
		placeholder := fmt.Sprintf("${%s}", k)
		expanded = strings.ReplaceAll(expanded, placeholder, v)
	}

	// For shell commands (sh -c ...), don't expand environment variables
	// because they should be expanded inside the container shell
	if strings.HasPrefix(strings.TrimSpace(command), "sh -c") {
		return expanded
	}

	return os.ExpandEnv(expanded)
}

// Close closes the Docker client
func (e *DockerExecutor) Close() error {
	return e.client.Close()
}
