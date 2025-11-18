# CIDX Documentation

Welcome to the CIDX (CI with Declarative eXecution) documentation.

## Table of Contents

### Getting Started

- [Main README](../README.md) - Project overview and quick start
- [Getting Started Guide](../GETTING_STARTED.md) - Step-by-step setup

### Core Concepts

- **Convention over Configuration**: Built-in presets eliminate boilerplate
- **Declarative Execution**: Declare what to run, not how to run it
- **Phase-based Organization**: Tools grouped by phases (security, code, test, build)

### Features

- [**Container Reuse & Caching**](container-reuse.md) - Performance optimization through container reuse
- [**Presets Development**](presets-development.md) - External TOML presets for fast iteration

### Configuration

- **TOML Configuration**: Simple, readable configuration format
- **Phase Definitions**: Organize tools by execution phases
- **Tool Overrides**: Customize preset behavior when needed

### Architecture

- **Presets Registry** (`pkg/presets/`) - Built-in tool configurations
- **Config Parser** (`pkg/config/`) - TOML parsing and validation
- **Docker Executor** (`pkg/executor/`) - Container execution via Docker SDK
- **Pipeline Runner** (`pkg/pipeline/`) - Phase-based orchestration
- **CLI** (`cmd/cidx/`) - User-facing commands

## Quick Links

### Commands

```bash
cidx init                    # Initialize new configuration
cidx list                    # List available tools
cidx info <tool>            # Show tool information
cidx validate               # Validate configuration
cidx run <phase|tool|all>   # Execute phase, tool, or all phases
cidx run --dry-run <target> # Preview execution without running
```

### Configuration Examples

#### Minimal Configuration

```toml
# cidx.toml
[security]
tools = ["trivy", "gitleaks"]

[code]
tools = ["prettier"]
```

#### With Overrides

```toml
# cidx.toml
[security]
tools = ["trivy"]

[trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

## Development

### Building

```bash
go build -o bin/cidx ./cmd/cidx
```

### Running Locally

```bash
go run ./cmd/cidx list
go run ./cmd/cidx run security
```

### Testing

```bash
go test ./...
go test -v ./pkg/presets
```

## Contributing

See [CLAUDE.md](../CLAUDE.md) for development guidelines and architecture details.

## Performance Features

### Container Reuse

CIDX reuses Docker containers across runs for **6x faster** subsequent executions. See [Container Reuse documentation](container-reuse.md) for details.

### Workspace Auto-detection

The workspace automatically defaults to the current directory where `cidx` is executed. No configuration needed.

## Future Documentation

Coming soon:

- Tool preset development guide
- Custom phase configuration
- Pipeline orchestration patterns
- Integration with CI/CD platforms
- Advanced configuration techniques
