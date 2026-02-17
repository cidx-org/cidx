# CLAUDE.md

This file provides guidance to Claude Code when working with this repository.

## Project Overview

**CIDX** (CI with Declarative eXecution) is a Go CLI tool that serves two purposes:

1. **Portable CI/CD Runner** — A single `cidx.toml` config runs identically on Local, GitHub Actions, GitLab CI, and Jenkins. Users declare container names, CIDX resolves images, volumes, commands, and environment variables from built-in presets.
2. **Developer Workflow** — Human-friendly commands for PR lifecycle, branch management, releases, and CI monitoring (`cidx action`, `cidx branch`, `cidx status`).

**Core Principles**: Convention over Configuration, KISS, security by default (Docker Hardened Images). Everything runs in containers — nothing is installed on the host, the workspace stays clean, and results are reproducible on any platform where Docker or Podman is available.

## Architecture

### Package Structure

```
pkg/
├── actions/         # Git workflow commands (PR, release, tag, cpw)
│   ├── pr.go            # PR create/ready/merge
│   ├── commit_push_watch.go  # Commit + push + watch CI
│   ├── release*.go      # Release lifecycle
│   ├── tag_*.go         # Tag prepare/create/delete/list
│   ├── artifact.go      # Build artifact handling
│   └── workflow_*.go    # CI workflow display/list
├── branch/          # Branch listing, formatting, git operations
│   ├── list.go          # Branch listing with filters (stale, merged, orphan)
│   ├── git.go           # Git operations wrapper
│   ├── format.go        # Output formatting
│   └── types.go         # Branch-related types
├── config/          # TOML/YAML config parsing and validation
│   ├── parser.go        # Load, parse, expand env vars
│   ├── validator.go     # Config validation
│   └── types.go         # Config, Pipeline, ContainerConfig types
├── environment/     # CI environment detection
│   ├── detector.go      # Auto-detect CI provider (GH Actions, GitLab, Jenkins)
│   └── security.go      # Local safety behaviors (no-push, draft, dry-run)
├── executor/        # Container execution layer
│   ├── docker.go        # Docker SDK wrapper
│   ├── selector.go      # Docker/Podman runtime selection
│   └── types.go         # Executor interface
├── pipeline/        # Phase-based orchestration
│   └── runner.go        # Sequential/parallel phase execution
├── presets/         # Built-in container configurations
│   ├── registry.go      # GlobalRegistry with 15+ presets
│   ├── loader.go        # Load custom presets from .cidx/presets.toml
│   └── types.go         # Preset, Option types
├── registry/        # Container registry operations (DHI, login)
│   └── registry.go
├── remote/          # Git remote provider abstraction
│   ├── provider.go      # Remote provider interface
│   └── factory.go       # GitHub/GitLab provider factory
├── validator/       # CI workflow validation
│   └── workflow.go      # Validate cidx.toml ↔ CI config consistency
└── vcs/             # Version control abstraction
    └── repository.go    # Git repository operations

cmd/cidx/
├── main.go          # CLI entry point (urfave/cli)
├── run.go           # `cidx run` command
├── list.go          # `cidx list` command
├── info.go          # `cidx info` command
├── init.go          # `cidx init` command
├── validate.go      # `cidx validate` command
├── preset.go        # `cidx preset` subcommands
├── action.go        # `cidx action` subcommands
├── branch.go        # `cidx branch` subcommands
├── status.go        # `cidx status` TUI dashboard
├── check.go         # `cidx check` workflow validation
├── registry.go      # `cidx registry` commands
├── workflow.go      # CI workflow commands
├── about.go         # `cidx about` version info
├── demo.go          # `cidx demo` spinner animation
├── vuln.go          # Vulnerability display
├── artifact_tui.go  # Artifact TUI components
├── merge_tui.go     # Merge TUI components
└── release_tui.go   # Release TUI components
```

### Data Flow

1. **Config Load**: `config.Load()` → Parse TOML → Expand `${ENV_VARS}`
2. **Environment Detect**: `environment.Detect()` → CI provider, event type, local safety mode
3. **Preset Merge**: For each enabled container:
   - `presets.Get(name)` → Load from registry (built-in + custom)
   - `preset.MergeWith(overrides)` → Apply user overrides from `cidx.toml`
4. **Execution**:
   - `executor.Select()` → Choose Docker or Podman
   - `executor.Run()` → Pull image → Create container → Stream logs
   - `pipeline.RunPhase()` → Execute containers in phase
   - `pipeline.RunPipeline()` → Execute phases in sequence (or parallel locally)

### Key Abstractions

- **Preset**: Complete container definition (image, command, workdir, volumes, env, options). Ships with sensible defaults.
- **ContainerConfig**: Runtime-resolved config after merging preset + user overrides. Ready for Docker execution.
- **Pipeline**: Named sequence of phases (`phases = ["security", "code", "test"]`). Mapped to events by convention.
- **Executor**: Interface abstracting Docker/Podman with `Run()`, `Available()`, `Name()`, `Close()`.
- **Environment**: Detected CI context (provider, event type, branch, tag, PR state, local safety mode).

## Development Commands

### Building

```bash
go build -o bin/cidx ./cmd/cidx
```

### Testing

```bash
go test ./...                         # All tests (unit + BDD)
go test -v ./pkg/presets              # Specific package
go test -cover ./...                  # With coverage
go test -v -run TestFeatures          # BDD scenarios only (godog)
```

BDD tests use [godog](https://github.com/cucumber/godog) with feature files in `features/`. Step definitions are in `*_steps_test.go` at the project root. Tests use a simulation engine (not the real binary) and `Strict: false` so Docker-dependent scenarios are pending, not failing.

### Running Locally

```bash
go run ./cmd/cidx list                # List available containers
go run ./cmd/cidx info trivy          # Show container information
go run ./cmd/cidx validate            # Validate config
go run ./cmd/cidx run --dry-run ci    # Dry-run a pipeline
```

### Preset Management

```bash
cidx preset list                      # List all presets by phase
cidx preset info trivy                # Show preset details
cidx preset search security           # Search presets
cidx preset export -o presets.toml    # Export all presets
cidx preset images                    # List images (deduplicated)
cidx preset check-updates             # Check for newer image versions
cidx preset scan                      # Scan images with Trivy + Grype
```

### Code Quality

```bash
go fmt ./...
go vet ./...
golangci-lint run
```

## Adding New Presets

Add to `pkg/presets/registry.go`:

```go
"toolname": {
    Name:    "toolname",
    Phase:   "security",                      // security, code, test, build, docker, release
    Image:   "org/image:tag",                 // Official Docker image
    Command: "tool scan .",
    Workdir: "/scan",
    Volumes: []string{"${WORKSPACE}:/scan"},
    // Optional:
    Env:         map[string]string{"KEY": "value"},
    ConfigFiles: []string{".toolrc", "tool.yaml"},
    Options: map[string]Option{
        "severity": {Type: "string", Default: "HIGH", CommandFlag: "--severity"},
    },
}
```

Guidelines:
1. Use official images (prefer DHI variants)
2. Defaults must work without overrides
3. Test with `cidx run toolname --dry-run`

## Configuration Patterns

```toml
# Minimal (recommended)
[security]
containers = ["trivy", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]

# With overrides
[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1

# Custom container
[containers.custom-scanner]
phase = "security"
image = "myorg/scanner:latest"
command = "scan ."
volumes = ["${WORKSPACE}:/scan"]
```

## Design Principles

1. **Convention over Configuration** — Built-in presets eliminate boilerplate. Overrides are exceptional.
2. **Declarative over Imperative** — User declares what to run, not how. No shell scripting needed.
3. **KISS** — Single config file. Container names are the only required input.
4. **Container-native** — Everything runs in containers. Nothing installed on the host. Clean workspace, portable results.
5. **Security by Default** — Docker Hardened Images, vulnerability scanning presets included.
6. **Explicit over Magic** — Dry-run mode, transparent merge logic, clear preset definitions.

## Code Style

- **Presets**: lowercase, hyphen-separated (`"ansible-lint"`)
- **Go types**: PascalCase (`Preset`, `ContainerConfig`)
- **Functions**: camelCase (`mergeWith`, `expandVolumes`)
- **Errors**: Always wrap with context: `fmt.Errorf("context: %w", err)`
- **Logging**: Use logrus. Distinguish user errors from system errors.

## Dependencies

- `github.com/BurntSushi/toml` — TOML parsing
- `github.com/docker/docker` — Docker SDK
- `github.com/urfave/cli/v2` — CLI framework
- `github.com/sirupsen/logrus` — Structured logging
- `github.com/cucumber/godog` — BDD testing (test only)

Keep dependencies minimal.
