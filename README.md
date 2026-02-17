# CIDX - CI with Declarative eXecution

[![Container Monitor](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml)
[![Security Audit](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml)

**One config, all environments.** CIDX is a portable CI/CD runner that replaces platform-specific YAML with a single declarative configuration. Name the tools you want to run, CIDX handles the rest.

Everything runs in containers. Nothing is installed on your machine, your workspace stays clean, and results are reproducible on any platform where Docker or Podman is available.

```toml
# cidx.toml — this is a complete configuration
[security]
containers = ["trivy", "gitleaks", "semgrep"]

[code]
containers = ["megalinter"]

[pipelines.ci]
phases = ["security", "code", "test"]
```

```bash
cidx run ci          # Same result locally, in GitHub Actions, GitLab CI, or Jenkins
```

## Why CIDX

| Problem | Without CIDX | With CIDX |
|---------|-------------|-----------|
| Running Trivy | 30+ lines of CI YAML per platform | `containers = ["trivy"]` |
| Local vs CI drift | Different configs, different results | Same `cidx.toml` everywhere |
| Adding security scans | Research image, volumes, flags per tool | Name the tool, run it |
| Switching CI platform | Rewrite all pipeline definitions | Change nothing |

### vs. Other Tools

| | CIDX | dagger | earthly | act | taskfile |
|---|---|---|---|---|---|
| Zero-config presets | Yes | No | No | No | No |
| Hardened images (DHI) | Default | No | No | No | No |
| Platform-portable | Yes | Yes | Yes | GitHub only | N/A |
| Config complexity | 5 lines | SDK code | Earthfile | GH YAML | Taskfile |
| Security-first | Built-in | Manual | Manual | Mirrors GH | Manual |

## How It Works

CIDX ships with **15+ built-in presets** (Trivy, Gitleaks, Semgrep, MegaLinter, GoSec, etc.). Each preset knows its Docker image, volumes, commands, environment variables, and config files.

```
cidx.toml          Built-in Presets       Custom Presets
(what to run)   +  (how to run it)    +  (.cidx/presets.toml)
     │                   │                      │
     └───────────┬───────┘──────────────────────┘
                 │
           Docker / Podman
```

You declare **what** to run. CIDX resolves **how**.

## Quick Start

```bash
# Install
go install github.com/cidx-org/cidx/cmd/cidx@latest

# Initialize a config in the current project
cidx init

# Run security scans
cidx run security

# Run a specific tool
cidx run trivy

# Run full CI pipeline
cidx run ci

# Preview without executing
cidx run --dry-run ci
```

## Configuration

### Minimal (recommended)

```toml
[security]
containers = ["trivy", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]
```

### With overrides

Override only what you need. Everything else uses preset defaults.

```toml
[security]
containers = ["trivy"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

### Custom presets

Define new tools or override built-in ones via `presets.toml`:

- **User-level**: `~/.config/cidx/presets.toml` (all projects)
- **Project-level**: `.cidx/presets.toml` (this project only)

```toml
[presets.my-scanner]
image = "myorg/scanner:latest"
command = "scan ."
phase = "security"
```

Export built-in presets as a starting point: `cidx preset export > .cidx/presets.toml`

### Version pinning

Lock the CIDX version to ensure consistency across developers and CI:

```toml
required_version = "1.2.3"
```

## Docker Hardened Images (DHI)

CIDX defaults to [Docker Hardened Images](https://dhi.io) for all built-in presets:

- Near-zero CVEs
- 95% smaller attack surface
- SBOM included
- SLSA Level 3 provenance

DHI requires Docker Hub credentials (free account). For CI, set `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` as repository secrets.

```bash
cidx registry check    # Verify DHI access
cidx registry login    # Authenticate
```

## Pipeline Commands

```bash
cidx run <phase>              # Run a phase (security, code, test, build)
cidx run <tool>               # Run a specific tool
cidx run <pipeline>           # Run a named pipeline (ci, release)
cidx run --parallel security  # Parallel execution (local only)
cidx run --quiet ci           # Suppress output, show logs only on failure
cidx run --dry-run ci         # Preview execution plan
```

## Preset Management

```bash
cidx preset list              # List all presets by phase
cidx preset info trivy        # Show preset details
cidx preset search security   # Search presets
cidx preset export            # Export all presets to stdout
cidx preset images            # List container images (deduplicated)
cidx preset check-updates     # Check for newer image versions
cidx preset scan              # Scan preset images for vulnerabilities
```

---

## Developer Workflow

CIDX also provides commands that simplify common git and CI workflows.

### PR Lifecycle

```bash
cidx action pr create "feat: description"   # Create branch + draft PR
cidx action cpw -m "commit message"         # Commit, push, watch CI
cidx action pr ready                         # Mark ready for review
cidx action pr merge                         # Squash merge + cleanup
```

### Branch Management

```bash
cidx branch list              # All branches with PR/merge status
cidx branch list --stale      # Inactive > 30 days
cidx branch list --merged     # Already merged
cidx branch cleanup           # Dry-run cleanup
cidx branch cleanup -x        # Delete merged branches
cidx branch pr -w             # Watch CI checks for current branch
```

### Releases

```bash
cidx action tag prepare       # Generate version + message
cidx action tag create        # Create and push tag
cidx action release create    # Create GitHub release
```

### Status Dashboard

```bash
cidx status                   # Interactive TUI dashboard
cidx status --no-tui          # Simple text output (auto in CI)
```

---

## Documentation

| Topic | Link |
|-------|------|
| Installation | [docs/getting-started/installation.md](docs/getting-started/installation.md) |
| Configuration | [docs/getting-started/configuration.md](docs/getting-started/configuration.md) |
| CI Integration | [docs/guides/ci-integration.md](docs/guides/ci-integration.md) |
| Available Tools | [docs/reference/tools.md](docs/reference/tools.md) |
| CLI Reference | [docs/reference/cli.md](docs/reference/cli.md) |
| Philosophy | [docs/core-concepts/philosophy.md](docs/core-concepts/philosophy.md) |
| Developer Guide | [CLAUDE.md](CLAUDE.md) |

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE)
