package executor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/registry"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	registrytypes "github.com/docker/docker/api/types/registry"
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
func (e *DockerExecutor) Run(ctx context.Context, containerConfig *config.ContainerConfig) error {
	e.logger.Infof("  ▸ Running [%s] %s", containerConfig.Phase, containerConfig.Name)

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

	// Get or create container
	containerID, containerName, err := e.getOrCreateContainer(ctx, containerConfig, volumes, command)
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

	e.logger.Infof("  ✓ %s completed", containerConfig.Name)
	return nil
}

// pullImage pulls a Docker image
func (e *DockerExecutor) pullImage(ctx context.Context, imageName string) error {
	e.logger.Debugf("Pulling image: %s", imageName)

	// Get authentication for the registry
	pullOpts := image.PullOptions{}
	if authStr := e.getAuthForImage(imageName); authStr != "" {
		pullOpts.RegistryAuth = authStr
	}

	out, err := e.client.ImagePull(ctx, imageName, pullOpts)
	if err != nil {
		// Check for authentication errors and provide helpful suggestions
		if isUnauthorizedError(err) {
			reg := extractRegistry(imageName)
			return &AuthError{
				Registry: reg,
				Image:    imageName,
				Err:      err,
			}
		}
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

// getAuthForImage returns the base64-encoded auth config for an image's registry
func (e *DockerExecutor) getAuthForImage(imageName string) string {
	reg := extractRegistry(imageName)
	regManager := registry.NewManager()

	var creds *registry.Credentials

	// For DHI, use Docker Hub credentials
	if reg == registry.DHIRegistry {
		creds = regManager.GetDockerHubCredentials()
	} else {
		// Try to get credentials for this specific registry
		// For now, we only handle DHI specially
		return ""
	}

	if creds == nil {
		return ""
	}

	// Encode credentials for Docker SDK
	authConfig := registrytypes.AuthConfig{
		Username: creds.Username,
		Password: creds.Secret,
	}

	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return ""
	}

	return base64.URLEncoding.EncodeToString(encodedJSON)
}

// getOrCreateContainer gets an existing container or creates a new one
func (e *DockerExecutor) getOrCreateContainer(ctx context.Context, containerConfig *config.ContainerConfig, volumes []string, command string) (string, string, error) {
	containerName := fmt.Sprintf("cidx_%s", containerConfig.Name)

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

	// If container exists, check if image has changed
	if len(containers) > 0 {
		existingContainer := containers[0]

		// Check if image has changed (compare cidx.image label with expected image)
		existingImage := existingContainer.Labels["cidx.image"]
		if existingImage != "" && existingImage != containerConfig.Image {
			e.logger.Infof("  🔄 Image updated: %s → %s, recreating container", existingImage, containerConfig.Image)

			// Remove old container
			if err := e.client.ContainerRemove(ctx, existingContainer.ID, container.RemoveOptions{Force: true}); err != nil {
				return "", "", fmt.Errorf("failed to remove old container: %w", err)
			}

			// Create new container with updated image
			return e.createContainer(ctx, containerConfig, volumes, command)
		}

		e.logger.Debugf("♻ Reusing container %s (preserves cache)", containerName)
		return existingContainer.ID, containerName, nil
	}

	// Container doesn't exist, create new one
	e.logger.Debugf("Creating new container: %s", containerName)
	return e.createContainer(ctx, containerConfig, volumes, command)
}

// createContainer creates a Docker container and returns containerID and containerName
func (e *DockerExecutor) createContainer(ctx context.Context, containerConfig *config.ContainerConfig, volumes []string, command string) (string, string, error) {
	// Parse volumes into binds
	binds := make([]string, len(volumes))
	copy(binds, volumes)

	// Convert env map to slice and expand environment variables
	env := make([]string, 0, len(containerConfig.Env))
	for k, v := range containerConfig.Env {
		// Expand ${VAR} in values
		expandedValue := os.ExpandEnv(v)
		env = append(env, fmt.Sprintf("%s=%s", k, expandedValue))
	}

	// Parse command
	// If custom entrypoint is set, keep command as single element
	var cmdParts []string
	if len(containerConfig.Entrypoint) > 0 {
		// With custom entrypoint, command should be a single element
		cmdParts = []string{command}
	} else {
		// Without entrypoint, parse normally
		cmdParts = parseCommand(command)
	}

	if len(cmdParts) == 0 {
		return "", "", fmt.Errorf("empty command")
	}

	// Generate container name with cidx_ prefix (fixed name for reuse)
	containerName := fmt.Sprintf("cidx_%s", containerConfig.Name)

	dockerConfig := &container.Config{
		Image:      containerConfig.Image,
		Cmd:        cmdParts,
		WorkingDir: containerConfig.Workdir,
		Env:        env,
		Labels: map[string]string{
			"managed-by": "cidx",
			"cidx.tool":  containerConfig.Name,
			"cidx.phase": containerConfig.Phase,
			"cidx.image": containerConfig.Image, // Track image for update detection
		},
	}

	// Override entrypoint if specified
	if len(containerConfig.Entrypoint) > 0 {
		dockerConfig.Entrypoint = containerConfig.Entrypoint
	}

	// Only set user for non-privileged containers
	if !containerConfig.Privileged {
		dockerConfig.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}

	hostConfig := &container.HostConfig{
		Binds:      binds,
		AutoRemove: false, // We'll remove manually
	}

	resp, err := e.client.ContainerCreate(ctx, dockerConfig, hostConfig, nil, nil, containerName)
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
func (e *DockerExecutor) printDryRun(containerConfig *config.ContainerConfig, volumes []string, command string) {
	containerName := fmt.Sprintf("cidx_%s", containerConfig.Name)

	fmt.Printf("Would execute:\n")
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

// Available checks if Docker daemon is running and accessible
func (e *DockerExecutor) Available() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := e.client.Ping(ctx)
	return err == nil
}

// Name returns the executor backend name
func (e *DockerExecutor) Name() string {
	return "docker"
}

// Ensure DockerExecutor implements Executor interface
var _ Executor = (*DockerExecutor)(nil)

// AuthError represents an authentication failure when pulling images
type AuthError struct {
	Registry string
	Image    string
	Err      error
}

func (e *AuthError) Error() string {
	var suggestion string
	if e.Registry == "dhi.io" {
		suggestion = fmt.Sprintf(`
Authentication required for Docker Hardened Images (DHI).

Image: %s

To authenticate (uses Docker Hub credentials):
  cidx registry login dhi.io

DHI is free and included with any Docker Hub account.
`, e.Image)
	} else {
		suggestion = fmt.Sprintf(`
Authentication required for registry: %s

Image: %s

To authenticate:
  cidx registry login %s
`, e.Registry, e.Image, e.Registry)
	}
	return suggestion
}

func (e *AuthError) Unwrap() error {
	return e.Err
}

// isUnauthorizedError checks if an error is an authentication failure
func isUnauthorizedError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "authentication required") ||
		strings.Contains(errStr, "denied") ||
		strings.Contains(errStr, "forbidden")
}

// extractRegistry extracts the registry hostname from an image name
func extractRegistry(imageName string) string {
	// Default Docker Hub registry
	const defaultRegistry = "docker.io"

	// Split on first /
	parts := strings.SplitN(imageName, "/", 2)
	if len(parts) == 1 {
		// No slash, it's a Docker Hub official image
		return defaultRegistry
	}

	// Check if first part looks like a registry (contains . or :)
	firstPart := parts[0]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") {
		return firstPart
	}

	// Otherwise it's a Docker Hub user/org
	return defaultRegistry
}
