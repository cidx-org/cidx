# CIDX — One CI config for local and CI

[![Container Monitor](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml)
[![Security Audit](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml)

CIDX is a container-first CI runner I built for myself. One `cidx.toml`, same checks locally and in CI.

Everything runs in containers. Nothing is installed directly on your machine, your workspace stays clean, and common tools can be used through built-in presets.

CIDX is also dogfooded on this repository: CIDX builds CIDX.

## Example setups

### Minimal CI

```toml
[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["megalinter"]

[pipelines.ci]
phases = ["security", "code"]
```

```bash
cidx run ci
```

### Fast local checks and fuller CI

```toml
[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["megalinter"]

[test]
containers = ["go-test"]

[pipelines.quick]
phases = ["security"]

[pipelines.ci]
phases = ["security", "code", "test"]
```

```bash
cidx run quick
cidx run ci
```

### Minimal overrides

```toml
[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

```bash
cidx run ci
```

## Why I built it

- Keep one CI config instead of duplicating workflow logic per platform
- Run the same checks locally before pushing
- Use containers and presets instead of hand-assembling tool commands
- Start small and override only what matters

## What CIDX is

- A single declarative config for local runs and CI
- A container-first way to run common checks and pipelines
- A small set of built-in presets, with overrides when needed
- A tool I actively use on this repository

## What CIDX is not

- Not a hosted CI platform
- Not a full DevOps suite
- Not a replacement for every platform-specific feature
- Not trying to hide everything; it tries to keep the common path simple

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

## How It Works

CIDX ships with **15+ built-in presets** (Trivy, Gitleaks, Semgrep, MegaLinter, GoSec, and others). Each preset knows its image, command, volumes, environment variables, and config files.

```text
cidx.toml          Built-in Presets       Custom Presets
(what to run)   +  (how to run it)    +  (.cidx/presets.toml)
     │                   │                      │
     └───────────┬───────┘──────────────────────┘
                 │
          Docker / Podman
```

You declare **what** to run. CIDX resolves **how**.

## Configuration

Start with phases and pipelines. Override only the tools that need custom flags, environment variables, or images.

### Minimal

```toml
[security]
containers = ["trivy", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]
```

### With overrides

Everything else can stay on preset defaults.

```toml
[security]
containers = ["trivy"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

### Custom presets

You can define new tools or override built-in ones via `presets.toml`:

- **User-level**: `~/.config/cidx/presets.toml` for all projects
- **Project-level**: `.cidx/presets.toml` for the current repository

```toml
[presets.my-scanner]
image = "myorg/scanner:latest"
command = "scan ."
phase = "security"
```

Export built-in presets as a starting point: `cidx preset export > .cidx/presets.toml`

### Version pinning

If you want local runs and CI to stay aligned on the same CIDX release:

```toml
required_version = "1.2.3"
```

## Built-in images

Built-in presets default to [Docker Hardened Images](https://dhi.io) where available.

- Smaller attack surface
- SBOM included
- Provenance metadata
- Better default baseline for security-sensitive tools

DHI requires Docker Hub credentials. In CI, set `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` as repository secrets.

```bash
cidx registry check    # Verify DHI access
cidx registry login    # Authenticate
```

## Common commands

### Running checks

```bash
cidx run <phase>              # Run a phase such as security, code, test, or build
cidx run <tool>               # Run a specific tool
cidx run <pipeline>           # Run a named pipeline such as ci or release
cidx run --parallel security  # Parallel execution within a phase (local only)
cidx run --quiet ci           # Show logs only on failure
cidx run --dry-run ci         # Preview the execution plan
```

### Managing presets

```bash
cidx preset list              # List all presets by phase
cidx preset info trivy        # Show preset details
cidx preset search security   # Search presets
cidx preset export            # Export all presets to stdout
cidx preset images            # List container images
cidx preset check-updates     # Check for newer image versions
cidx preset scan              # Scan preset images for vulnerabilities
```

## Workflow helpers

Because I use CIDX on CIDX, the project also includes a few helpers for day-to-day git and release work.

### PR lifecycle

```bash
cidx action pr create "feat: description"   # Create branch and draft PR
cidx action cpw -m "commit message"         # Commit, push, and watch CI
cidx action pr ready                         # Mark ready for review
cidx action pr merge                         # Squash merge and cleanup
```

### Branch management

```bash
cidx branch list              # All branches with PR and merge status
cidx branch list --stale      # Inactive for more than 30 days
cidx branch list --merged     # Already merged branches
cidx branch cleanup           # Dry-run cleanup
cidx branch cleanup -x        # Delete merged branches
cidx branch pr -w             # Watch CI checks for current branch
```

### Releases

```bash
cidx action tag prepare       # Generate version and message
cidx action tag create        # Create and push tag
cidx action release create    # Create GitHub release
```

### Status dashboard

```bash
cidx status                   # Interactive TUI dashboard
cidx status --no-tui          # Simple text output (auto in CI)
```

## Documentation

- **Installation**: [docs/getting-started/installation.md](docs/getting-started/installation.md)
- **Configuration**: [docs/getting-started/configuration.md](docs/getting-started/configuration.md)
- **CI Integration**: [docs/guides/ci-integration.md](docs/guides/ci-integration.md)
- **Available Tools**: [docs/reference/tools.md](docs/reference/tools.md)
- **CLI Reference**: [docs/reference/cli.md](docs/reference/cli.md)
- **Philosophy**: [docs/core-concepts/philosophy.md](docs/core-concepts/philosophy.md)
- **Development Notes**: [CLAUDE.md](CLAUDE.md)

## Contributing

Contributions welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE)
