# Presets Development

This document explains how CIDX manages tool presets during development vs production builds.

## Architecture Overview

CIDX uses a **dual-mode preset system**:

1. **Development Mode**: Presets loaded from external file (`presets.toml`)
   - Fast iteration without recompilation
   - Easy to test new tools
   - Validation on every run

2. **Production Mode**: Presets embedded in binary
   - Single binary with all presets
   - No external dependencies
   - Fast startup

## Development Workflow

### Preset File Location

During development, presets are defined in:

```
pkg/presets/presets.toml
```

This file is **NOT embedded** when running with `go run` or in development mode.

### File Format

```toml
# pkg/presets/presets.toml
[presets.trivy]
name = "trivy"
phase = "security"
image = "aquasec/trivy:latest"
command = "fs /scan"
workdir = "/scan"
volumes = ["${WORKSPACE}:/scan"]

[presets.trivy.env]
TRIVY_CACHE_DIR = "/tmp/trivy-cache"

[presets.trivy.options.severity]
type = "string"
default = "UNKNOWN,LOW,MEDIUM,HIGH,CRITICAL"
description = "Severities to report (comma-separated)"
command_flag = "--severity"

[presets.trivy.options.exit_code]
type = "int"
default = 0
description = "Exit code when vulnerabilities are found"
command_flag = "--exit-code"

[presets.gitleaks]
name = "gitleaks"
phase = "security"
image = "zricethezav/gitleaks:latest"
command = "git ."
workdir = "/repo"
volumes = ["${WORKSPACE}:/repo"]
config_files = [".gitleaks.toml"]

[presets.prettier]
name = "prettier"
phase = "code"
image = "tmknom/prettier:latest"
command = "prettier --check ."
workdir = "/work"
volumes = ["${WORKSPACE}:/work"]
config_files = [".prettierrc", ".prettierrc.json", ".prettierrc.yml", "prettier.config.js"]

[presets.prettier.options.write]
type = "bool"
default = false
description = "Write formatted files (instead of check)"
command_flag = "--write"
```

### Development Commands

```bash
# Edit presets without recompiling
vim pkg/presets/presets.toml

# Test immediately
go run ./cmd/cidx run security

# Validate preset syntax
go run ./cmd/cidx list
go run ./cmd/cidx info trivy
```

### Hot Reload During Development

Changes to `presets.toml` are picked up immediately:

```bash
# Modify preset
echo "    command: fs /scan --verbose" >> pkg/presets/presets.toml

# Test without rebuilding
go run ./cmd/cidx run --dry-run security
```

## Production Build

### Embedding Process

During build, presets are **embedded** into the binary using Go's `embed` directive:

```go
// pkg/presets/registry.go
package presets

import (
    _ "embed"
    "github.com/BurntSushi/toml"
)

//go:embed presets.toml
var presetsData []byte

func init() {
    // Load embedded presets at startup
    if err := toml.Unmarshal(presetsData, &GlobalRegistry); err != nil {
        panic(fmt.Sprintf("failed to load embedded presets: %v", err))
    }
}
```

### Build Command

```bash
# Standard build embeds presets.toml
go build -o bin/cidx ./cmd/cidx

# Binary contains all presets
./bin/cidx list  # No external files needed
```

### Verification

```bash
# Build binary
go build -o bin/cidx ./cmd/cidx

# Move to different directory
cd /tmp

# Binary works without source code
~/projects/cidx/bin/cidx list
~/projects/cidx/bin/cidx info trivy
```

## Implementation Details

### Loading Logic

```go
// pkg/presets/registry.go
package presets

import (
    _ "embed"
    "os"
    "path/filepath"
)

//go:embed presets.toml
var embeddedPresets []byte

var GlobalRegistry map[string]Preset

func init() {
    // Try loading from file first (development mode)
    if data, err := loadFromFile(); err == nil {
        GlobalRegistry = parsePresets(data)
        return
    }

    // Fallback to embedded presets (production mode)
    GlobalRegistry = parsePresets(embeddedPresets)
}

func loadFromFile() ([]byte, error) {
    // Look for presets.toml in source tree
    paths := []string{
        "pkg/presets/presets.toml",
        "presets.toml",
    }

    for _, path := range paths {
        if data, err := os.ReadFile(path); err == nil {
            return data, nil
        }
    }

    return nil, fmt.Errorf("presets.toml not found")
}
```

### Detection Strategy

1. **Development**: File exists → Load from file
2. **Production**: File missing → Use embedded data

This allows seamless transition between development and production.

## Benefits

### Development Mode (File-based)

✅ **Fast Iteration**

- No recompilation needed
- Instant feedback on preset changes
- Easy to experiment

✅ **Easy Debugging**

- Readable TOML format
- Can use comments
- Version control friendly

✅ **Validation**

- Syntax errors caught on load
- Clear error messages
- Can validate schema

### Production Mode (Embedded)

✅ **Single Binary**

- No external dependencies
- Easy distribution
- Container-friendly

✅ **Performance**

- No file I/O at runtime
- Faster startup
- Smaller container images

✅ **Reliability**

- Presets always available
- No file path issues
- Consistent behavior

## Adding New Presets

### Step 1: Edit presets.toml

```toml
[presets.newtool]
name = "newtool"
phase = "security"
image = "myorg/newtool:latest"
command = "scan /workspace"
workdir = "/workspace"
volumes = ["${WORKSPACE}:/workspace"]
```

### Step 2: Test Immediately

```bash
go run ./cmd/cidx list
go run ./cmd/cidx info newtool
go run ./cmd/cidx run --dry-run newtool
```

### Step 3: Validate

```bash
# Check preset is recognized
go run ./cmd/cidx list | grep newtool

# Test execution
go run ./cmd/cidx run newtool
```

### Step 4: Build for Production

```bash
# Preset automatically embedded
go build -o bin/cidx ./cmd/cidx

# Verify
./bin/cidx info newtool
```

## File Format Reference

### Preset Structure

```yaml
presets:
  <tool_name>:
    name: string              # Tool identifier (required)
    phase: string             # security|code|test|build (required)
    image: string             # Docker image (required)
    command: string           # Command to run (required)
    workdir: string           # Working directory (required)
    volumes: []string         # Volume mounts (required)
    env: map[string]string    # Environment variables (optional)
    config_files: []string    # Config file detection (optional)
    options: map[string]Option # Configurable options (optional)
```

### Option Structure

```yaml
options:
  <option_name>:
    type: string # string|int|bool (required)
    default: any # Default value (required)
    description: string # Help text (required)
    command_flag: string # Maps to CLI flag (optional)
    env_var: string # Maps to env var (optional)
```

## Migration Guide

### From Hardcoded to File-based

**Before** (`pkg/presets/registry.go`):

```go
var GlobalRegistry = map[string]Preset{
    "trivy": {
        Name:    "trivy",
        Phase:   "security",
        Image:   "aquasec/trivy:latest",
        Command: "fs /scan",
        // ... more fields
    },
}
```

**After** (`pkg/presets/presets.toml`):

```toml
[presets.trivy]
name = "trivy"
phase = "security"
image = "aquasec/trivy:latest"
command = "fs /scan"
# ... more fields
```

### Conversion Script

```bash
# Convert existing Go registry to TOML
go run ./tools/convert-registry.go
```

## Best Practices

### Development

- ✅ Edit `presets.toml` for all changes
- ✅ Test with `go run` before building
- ✅ Use comments to document complex presets
- ✅ Validate with `cidx list` and `cidx info`

### Production

- ✅ Always build with `go build` (embeds automatically)
- ✅ Test binary in clean environment
- ✅ Verify preset availability with `cidx list`

### Version Control

- ✅ Commit `presets.toml` to git
- ✅ Don't commit built binaries
- ✅ Document preset changes in commit messages

## Troubleshooting

### Preset Not Found

**Problem**: Tool not listed in `cidx list`

**Solutions**:

1. Check `presets.toml` exists in correct location
2. Validate TOML syntax
3. Ensure preset has required fields
4. Check for typos in preset name

### Changes Not Reflected

**Problem**: Modifications to `presets.toml` not working

**Solutions**:

1. Verify using `go run` (not built binary)
2. Check file path is correct
3. Restart if running as daemon/service
4. Clear any caches

### Embedded Presets Wrong Version

**Problem**: Binary has old preset definitions

**Solutions**:

1. Rebuild binary: `go build -o bin/cidx ./cmd/cidx`
2. Verify build timestamp: `./bin/cidx --version`
3. Clean build cache: `go clean -cache`

## Future Enhancements

Planned improvements:

- [ ] User-defined presets: `~/.config/cidx/presets.toml`
- [ ] Preset inheritance and composition
- [ ] Preset validation tool
- [ ] Auto-generated documentation from presets
- [ ] Preset marketplace/registry
