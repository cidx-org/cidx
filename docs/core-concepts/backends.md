# Container Backends

CIDX supports multiple container runtimes through its executor abstraction layer. This allows you to use the same configuration regardless of which container runtime is installed.

## Philosophy

**Zero installation on host.** CIDX's core principle is that everything runs in containers. You never need to install security scanners, linters, or build tools locally. The only requirement is a container runtime.

## Supported Backends

| Backend    | Status      | Description                          |
| ---------- | ----------- | ------------------------------------ |
| Docker     | ✅ Stable   | Docker Engine via Docker SDK         |
| Podman     | ✅ Stable   | Podman via CLI (`podman` command)    |
| Kubernetes | 🚧 Planned  | Run containers as Kubernetes Jobs    |

## Auto-Detection

By default, CIDX automatically detects and selects the best available backend:

```bash
cidx run trivy   # Auto-detect
```

**Selection order:**

1. **Docker** - Preferred if Docker daemon is running
2. **Podman** - Fallback if Docker is unavailable
3. **Error** - Helpful message if no runtime is available

## Forcing a Backend

You can force a specific backend with the `--backend` (or `-b`) flag:

```bash
cidx run trivy --backend docker    # Force Docker
cidx run trivy --backend podman    # Force Podman
cidx run trivy -b podman           # Short flag
```

## Backend Differences

### Docker

- Uses Docker SDK (Go client library)
- Communicates with Docker daemon via socket
- Supports container reuse for caching (e.g., Trivy DB)
- Requires Docker daemon running

```bash
# Check Docker availability
docker info
```

### Podman

- Uses Podman CLI via `os/exec`
- Daemonless architecture
- Rootless by default
- Uses `--userns=keep-id` for user namespace mapping

```bash
# Check Podman availability
podman info
```

## Requirements

### Docker

```bash
# Linux
sudo apt install docker.io
sudo systemctl start docker

# macOS
brew install --cask docker
open -a Docker

# Windows
# Install Docker Desktop
```

### Podman

```bash
# Linux
sudo apt install podman

# macOS
brew install podman
podman machine init
podman machine start

# Windows
# Install Podman Desktop
```

## Troubleshooting

### "Docker daemon is not running"

```bash
# Linux
sudo systemctl start docker

# macOS
open -a Docker

# Windows
# Start Docker Desktop
```

### "Podman is not responding"

```bash
# macOS/Windows
podman machine start

# Linux (rootless)
systemctl --user start podman.socket
```

### No container runtime available

CIDX requires at least one container runtime. Install Docker or Podman:

```bash
# Quick check
docker --version || podman --version
```

## CI/CD Environments

Most CI/CD environments have Docker pre-installed:

| Environment      | Default Runtime | Notes                           |
| ---------------- | --------------- | ------------------------------- |
| GitHub Actions   | Docker          | Pre-installed on all runners    |
| GitLab CI        | Docker          | Via Docker-in-Docker or socket  |
| Jenkins          | Docker          | Requires Docker plugin          |
| CircleCI         | Docker          | Pre-installed                   |
| Local            | Auto-detect     | Docker or Podman                |

## Architecture

```
                    cidx run trivy
                          │
                    ┌─────▼─────┐
                    │  Selector │
                    └─────┬─────┘
                          │
          ┌───────────────┼───────────────┐
          │               │               │
    ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
    │  Docker   │   │  Podman   │   │   Kube    │
    │ Executor  │   │ Executor  │   │ Executor  │
    └─────┬─────┘   └─────┬─────┘   └─────┬─────┘
          │               │               │
    ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
    │  Docker   │   │  Podman   │   │   K8s     │
    │  Daemon   │   │   CLI     │   │   API     │
    └───────────┘   └───────────┘   └───────────┘
```

All executors implement the same `Executor` interface:

```go
type Executor interface {
    Run(ctx context.Context, config *ContainerConfig) error
    Available() bool
    Name() string
    Close() error
}
```

This allows CIDX to swap backends transparently without changing user configuration.
