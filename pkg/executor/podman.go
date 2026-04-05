package executor

import (
	"fmt"
	"os"
	"path/filepath"
)

// findPodmanSocket locates the Podman Docker-compatible socket.
// Podman exposes its API on different paths depending on the OS and setup.
func findPodmanSocket() string {
	candidates := podmanSocketCandidates()

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// podmanSocketCandidates returns potential socket paths in priority order.
func podmanSocketCandidates() []string {
	// DOCKER_HOST env can point to Podman's socket
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return []string{host}
	}

	uid := os.Getuid()

	return []string{
		// Rootless podman (most common on Linux/macOS)
		fmt.Sprintf("/run/user/%d/podman/podman.sock", uid),
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "podman", "podman.sock"),

		// Rootful podman
		"/run/podman/podman.sock",
		"/var/run/podman/podman.sock",

		// macOS podman machine
		filepath.Join(os.Getenv("HOME"), ".local", "share", "containers", "podman", "machine", "podman.sock"),
	}
}
