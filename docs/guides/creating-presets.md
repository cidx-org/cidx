# Presets Development

This document explains how CIDX manages container presets during development vs production builds.

## Architecture Overview

CIDX uses a **dual-mode preset system**:

1. **Development Mode**: Presets loaded from external file (`presets.toml`)
   - Fast iteration without recompilation
   - Easy to test new containers
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
description = "Severities to report (comma-separated)"
command_flag = "--severity"

[presets.trivy.options.exit_code]
type = "int"
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
go run ./cmd/cidx preset list
go run ./cmd/cidx preset info trivy
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
./bin/cidx preset list  # No external files needed
```

### Verification

```bash
# Build binary
go build -o bin/cidx ./cmd/cidx

# Move to different directory
cd /tmp

# Binary works without source code
~/projects/cidx/bin/cidx preset list
~/projects/cidx/bin/cidx preset info trivy
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

Catalogue images are pinned `image:tag@sha256:...` — the tag stays readable, the
digest makes the reference immutable. See [rule 1 of the supply-chain
policy](../core-concepts/security.md#supply-chain-policy) for why, and
issue #242 for the decision record. `TestCatalogueImagesArePinnedByDigest`
rejects a preset added without a digest.

Resolve the digest of the **multi-arch index**, not of a single architecture —
pinning one platform's manifest would break every runner on another:

```bash
docker buildx imagetools inspect --format '{{.Manifest.Digest}}' myorg/newtool:1.4.0
```

```toml
[presets.newtool]
name = "newtool"
phase = "security"
image = "myorg/newtool:1.4.0@sha256:0d3ff80420a972e6966417c32a02340bfbd7ade2d6fdad9b162d4ce6cfb74a6a"
command = "scan /workspace"
workdir = "/workspace"
volumes = ["${WORKSPACE}:/workspace"]
```

This applies to the built-in catalogue only. Presets you define in your own
`.cidx/presets.toml` are yours to pin or not.

### Step 2: Test Immediately

```bash
go run ./cmd/cidx preset list
go run ./cmd/cidx preset info newtool
go run ./cmd/cidx run --dry-run newtool
```

### Step 3: Validate

```bash
# Check preset is recognized
go run ./cmd/cidx preset list | grep newtool

# Test execution
go run ./cmd/cidx run newtool
```

### Step 4: Build for Production

```bash
# Preset automatically embedded
go build -o bin/cidx ./cmd/cidx

# Verify
./bin/cidx preset info newtool
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
    description: string # Help text (required)
    command_flag: string # Maps to CLI flag (optional)
    env_var: string # Maps to env var (optional)
```

An option has no `default` key. It used to, and it was never applied — a
declared default that never runs is a default nobody has ever validated, and
documentation that says otherwise lies (#299). A value the preset genuinely
needs goes in its `command` or `env`, where every run exercises it. A residual
`default` in a preset file is reported as an unknown key.

`command_flag` appends `<flag> <value>` to the preset's `command`. A
`type = "bool"` option is a switch: enabled it appends the flag alone, disabled
it appends nothing. For a shell-wrapped command (`sh -c '<script>'`) the flag is
injected before the closing quote, so it reaches the wrapped tool instead of
`sh`:

```
command = "sh -c 'pip install --quiet mypy && mypy .'"   # strict = true
       →  sh -c 'pip install --quiet mypy && mypy . --strict'
```

Write the script so the flag-bearing tool invocation comes last.

A boolean override is accepted as a TOML boolean (`strict = true`) or as the
string form a quoted value or an expanded `${VAR}` produces (`strict = "true"`).
Anything else is reported as a warning and ignored, so the preset's own command
stands rather than a typo reaching the container.

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
- ✅ Validate with `cidx preset list` and `cidx preset info`

### Production

- ✅ Always build with `go build` (embeds automatically)
- ✅ Test binary in clean environment
- ✅ Verify preset availability with `cidx preset list`

### Version Control

- ✅ Commit `presets.toml` to git
- ✅ Don't commit built binaries
- ✅ Document preset changes in commit messages

## Troubleshooting

### Preset Not Found

**Problem**: Tool not listed in `cidx preset list`

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
- [ ] Preset validation container
- [ ] Auto-generated documentation from presets
- [ ] Preset marketplace/registry
