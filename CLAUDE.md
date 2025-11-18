# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**CIDX** (CI with Declarative eXecution) is a Go-based CLI tool for running DevSecOps pipelines with ultra-declarative configuration following **Convention over Configuration** principles.

**Core Philosophy**: Users declare tool names only. CIDX has built-in presets that automatically know Docker images, volumes, commands, environment variables, and configuration files for each tool.

## Key Concepts

### Convention over Configuration
- Users enable tools by name: `enabled = ["trivy", "megalinter"]`
- CIDX provides complete presets with sensible defaults
- Overrides are optional and minimal (10% of cases)

### Architecture Layers
1. **Presets Registry** (`pkg/presets/`) - Built-in tool configurations
2. **Config Parser** (`pkg/config/`) - TOML parsing and validation
3. **Docker Executor** (`pkg/executor/`) - Container execution via Docker SDK
4. **Pipeline Runner** (`pkg/pipeline/`) - Phase-based orchestration
5. **CLI** (`cmd/cidx/`) - User-facing commands

## Development Commands

### Building
```bash
go build -o bin/cidx ./cmd/cidx      # Build binary
go build                              # Build to default output
```

### Testing
```bash
go test ./...                         # Run all tests
go test -v ./pkg/presets              # Test specific package
go test -cover ./...                  # With coverage
```

### Running Locally
```bash
go run ./cmd/cidx list                # List available tools
go run ./cmd/cidx info trivy          # Show tool information
go run ./cmd/cidx validate            # Validate config
go run ./cmd/cidx run --dry-run ci    # Dry-run a pipeline
```

### Code Quality
```bash
go fmt ./...                          # Format code
go vet ./...                          # Static analysis
golangci-lint run                     # Comprehensive linting
```

### Dependencies
```bash
go mod tidy                           # Clean up dependencies
go mod download                       # Download dependencies
go mod verify                         # Verify dependencies
```

## Project Architecture

### Package Structure

```
pkg/
├── presets/
│   ├── types.go        # Preset and Option structures
│   └── registry.go     # GlobalRegistry with built-in presets
├── config/
│   ├── types.go        # Config, Tools, Pipeline structures
│   ├── parser.go       # TOML loading and env expansion
│   └── validator.go    # Configuration validation
├── executor/
│   └── docker.go       # Docker SDK wrapper for container execution
└── pipeline/
    └── runner.go       # Phase-based pipeline orchestration

cmd/cidx/
├── main.go             # CLI app entry point
├── run.go              # Run tool or pipeline
├── list.go             # List available presets
├── info.go             # Show preset details
├── validate.go         # Validate configuration
└── init.go             # Initialize new config
```

### Data Flow

1. **Config Load**: `config.Load()` → Parse TOML → Expand env vars
2. **Preset Merge**: For each enabled tool:
   - `presets.Get(toolName)` → Load preset from registry
   - `preset.MergeWith(overrides)` → Apply user overrides
   - Convert to `config.ToolConfig`
3. **Execution**:
   - `executor.Run()` → Pull image → Create container → Stream logs
   - `pipeline.RunPhase()` → Execute all tools in phase
   - `pipeline.RunPipeline()` → Execute phases in sequence

### Key Abstractions

#### Preset
Complete tool definition with defaults:
- Docker image, command, workdir
- Volume mounts, environment variables
- Configurable options with type safety
- Config file auto-detection

#### ToolConfig
Runtime-resolved configuration after merging preset + overrides:
- Ready for Docker execution
- All variables expanded
- All overrides applied

#### Pipeline
Sequence of phases to execute:
- `phases = ["security", "code", "test"]`
- Tools grouped by phase
- Sequential execution within phases

## Adding New Presets

When adding a new tool preset to `pkg/presets/registry.go`:

### Required Fields
```go
"toolname": {
    Name:    "toolname",           // Tool identifier
    Phase:   "security",           // security, code, test, build
    Image:   "org/image:tag",      // Official Docker image
    Command: "tool scan .",        // Default command
    Workdir: "/scan",              // Container working directory
    Volumes: []string{"${WORKSPACE}:/scan"},  // Volume mounts
}
```

### Optional Fields
```go
Env: map[string]string{            // Default environment variables
    "TOOL_CONFIG": "/config",
},
ConfigFiles: []string{             // Auto-detected config files
    ".toolrc",
    "tool.config.yaml",
},
Options: map[string]Option{        // Configurable options
    "severity": {
        Type:        "string",
        Default:     "HIGH",
        Description: "Severity level",
        CommandFlag: "--severity",  // Maps to command flag
        // OR
        EnvVar:      "SEVERITY",    // Maps to env var
    },
},
```

### Preset Guidelines
1. **Use official images**: Prefer official registry images
2. **Sensible defaults**: Config should work without overrides
3. **Document options**: Clear descriptions for all options
4. **Test locally**: Verify with `cidx run <tool> --dry-run`
5. **Config detection**: List common config file names

## Configuration Patterns

### Minimal (Recommended)
```toml
[tools]
enabled = ["trivy", "megalinter", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]
```

### With Overrides
```toml
[tools]
enabled = ["trivy"]

[tools.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

### Custom Tool
```toml
[tools.custom-scanner]
phase = "security"
image = "myorg/scanner:latest"
command = "scan ."
volumes = ["${WORKSPACE}:/scan"]
```

## Design Principles

### 1. Convention over Configuration
- Built-in presets eliminate boilerplate
- Overrides are exceptional, not the norm
- Sensible defaults that work out of the box

### 2. Declarative over Imperative
- User declares **what** to run, not **how**
- CIDX handles Docker orchestration
- No shell scripting required

### 3. Simplicity over Features
- Single config file (TOML or YAML)
- Tool names are the only required input
- Advanced features available but hidden by default

### 4. Explicit over Magic
- Clear preset definitions in code
- Transparent merge logic
- Dry-run mode to inspect execution

## Common Development Tasks

### Adding a Security Tool
1. Research official Docker image
2. Identify required volumes (usually workspace only)
3. Determine default command
4. Add to `pkg/presets/registry.go` under `Phase: "security"`
5. Test: `cidx info newtool && cidx run newtool --dry-run`

### Adding a Code Quality Tool
1. Find official image (often language-specific)
2. Check for config file conventions
3. Set phase to `"code"`
4. Add `ConfigFiles` slice if tool uses config files
5. Document in README preset section

### Extending Options System
When a tool needs runtime configuration:
1. Add `Options` map to preset
2. Define option type, default, description
3. Use `CommandFlag` for CLI flags or `EnvVar` for env vars
4. Update `applyOption()` if custom logic needed

### Testing Changes
```bash
# Unit test changes
go test ./pkg/presets -v

# Integration test with dry-run
cidx run <tool> --dry-run

# Full validation
cidx validate && cidx list && cidx info <tool>
```

## Code Style Guidelines

### Naming
- Presets: lowercase, hyphen-separated (`"ansible-lint"`)
- Go types: PascalCase (`Preset`, `ToolConfig`)
- Functions: camelCase (`mergeWith`, `expandVolumes`)
- Files: lowercase with underscores (`registry.go`, `docker.go`)

### Error Handling
- Always wrap errors with context: `fmt.Errorf("context: %w", err)`
- Return early on errors
- Use logrus for informational logging
- Distinguish between user errors (invalid config) and system errors (Docker failure)

### Configuration Parsing
- Support both TOML and YAML formats
- Auto-detect format from file extension
- Expand environment variables after parsing
- Validate before execution

### Docker Execution
- Always pull images before running (unless cached)
- Stream logs to stdout/stderr in real-time
- Clean up containers after execution (even on error)
- Support dry-run mode for inspection

## Future Enhancements (Roadmap)

When implementing these features, maintain core simplicity:

1. **Auto-detection**: Detect project type, suggest tools
2. **User presets**: Load from `~/.config/cidx/presets.toml`
3. **Parallel execution**: Run independent tools concurrently
4. **Registry override**: Support private registries globally
5. **Validation hooks**: Pre/post execution scripts
6. **SBOM generation**: Track tool versions and results
7. **Web UI**: Visual configuration builder

## Dependencies

Core dependencies and their purpose:
- `github.com/BurntSushi/toml` - TOML parsing
- `github.com/docker/docker` - Docker SDK for container execution
- `github.com/urfave/cli/v2` - CLI framework
- `github.com/sirupsen/logrus` - Structured logging

Keep dependencies minimal. Evaluate carefully before adding new ones.

## Testing Philosophy

- **Unit tests**: Test preset logic, config parsing, validation
- **Integration tests**: Test Docker execution with common tools
- **Dry-run tests**: Verify correct Docker command generation
- **Example configs**: Keep examples/ in sync with features

Run tests before committing:
```bash
go test ./... && go vet ./... && golangci-lint run
```
