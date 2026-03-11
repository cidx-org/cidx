# Getting Started with CIDX

## Quick Start (5 minutes)

### 1. Build CIDX

```bash
make build
```

This creates the binary at `bin/cidx`.

### 2. Initialize Configuration

```bash
./bin/cidx init
```

This creates a `cidx.toml` file with sensible defaults:

```toml
[security]
containers = ["trivy", "gitleaks"]

[code]
containers = ["prettier"]

[pipelines.ci]
phases = ["security", "code"]
```

### 3. List Available Tools

```bash
./bin/cidx preset list
```

Output:

```text
Available presets:

  code:
   - commitlint
   - prettier

  security:
   - gitleaks
   - trivy
```

### 4. Get Tool Information

```bash
./bin/cidx preset info trivy
```

See complete preset configuration including Docker image, volumes, and options.

### 5. Dry-Run

```bash
./bin/cidx run --dry-run ci
```

See what would be executed without actually running containers.

### 6. Run Tools

```bash
# Run a single container
./bin/cidx run trivy

# Run a pipeline
./bin/cidx run ci
```

## Understanding the Configuration

### Minimal Configuration

90% of users need just this:

```toml
[security]
containers = ["trivy"]

[code]
containers = ["megalinter"]

[pipelines.ci]
phases = ["security", "code"]
```

CIDX knows:

- Which Docker image to use
- What volumes to mount
- What command to run
- Environment variables needed

### With Overrides

Only override what you need:

```toml
[security]
containers = ["trivy"]

[pipelines.ci]
phases = ["security"]

[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

### Multiple Pipelines

```toml
[pipelines.security]
phases = ["security"]

[pipelines.code]
phases = ["code"]

[pipelines.full]
phases = ["security", "code", "test"]
```

Run with:

```bash
./bin/cidx run security
./bin/cidx run code
./bin/cidx run full
```

## Command Reference

### Global Flags

```bash
--config, -c    Path to config file (default: auto-detect)
--verbose       Enable verbose output
```

### Commands

#### `cidx init`

Create a new configuration file:

```bash
cidx init                # Creates cidx.toml
```

#### `cidx preset list`

List all available presets:

```bash
cidx preset list
```

#### `cidx preset info <container>`

Show detailed information about a container:

```bash
cidx preset info trivy
cidx preset info megalinter
```

#### `cidx validate`

Validate your configuration:

```bash
cidx validate
cidx -c path/to/cidx.toml validate
```

#### `cidx run <target>`

Run a container or pipeline:

```bash
cidx run trivy           # Run single container
cidx run ci              # Run pipeline
cidx run --dry-run ci    # Dry-run (don't execute)
```

## Examples

### Example 1: Security Scanning Only

```toml
[security]
containers = ["trivy", "gitleaks"]

[pipelines.security]
phases = ["security"]
```

```bash
./bin/cidx run security
```

### Example 2: Fast local checks and fuller CI

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
./bin/cidx run quick
./bin/cidx run ci
```

### Example 3: Minimal overrides

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
./bin/cidx run ci
```

## Development Workflow

### Typical Usage

```bash
# 1. Create config
cidx init

# 2. Edit cidx.toml to enable tools you need

# 3. Validate config
cidx validate

# 4. Test with dry-run
cidx run --dry-run ci

# 5. Run for real
cidx run ci
```

### In CI/CD

```yaml
# .gitlab-ci.yml
security:
  stage: security
  script:
    - cidx run security

code-quality:
  stage: code
  script:
    - cidx run code
```

```yaml
# .github/workflows/ci.yml
- name: Run CIDX Security
  run: |
    cidx run security

- name: Run CIDX Code Quality
  run: |
    cidx run code
```

## Tips & Best Practices

### 1. Start Small

Begin with 2-3 tools and expand gradually:

```toml
[security]
containers = ["trivy"]

[code]
containers = ["megalinter"]
```

### 2. Use Dry-Run

Always test with `--dry-run` first:

```bash
cidx run --dry-run ci
```

### 3. Organize by Phase

Group tools logically:

- **security**: vulnerability scanning, secrets detection
- **code**: linting, formatting, quality checks
- **test**: unit tests, integration tests
- **build**: Docker builds, packaging

### 4. Create Multiple Pipelines

```toml
[pipelines.quick]
phases = ["security"]

[pipelines.full]
phases = ["security", "code", "test"]
```

Run quick checks locally, full checks in CI.

### 5. Override Minimally

Only override when defaults don't work:

```toml
# Good - only what's needed
[containers.trivy]
severity = "HIGH,CRITICAL"

# Avoid - overriding everything
[containers.trivy]
image = "aquasec/trivy:latest"
command = "trivy fs /scan"
# ... etc
```

## Troubleshooting

### "no cidx config file found"

Solution: Run `cidx init` or specify config path with `-c`.

### "preset not found"

Check container name with `cidx preset list`. Names are lowercase with hyphens (e.g., `ansible-lint`).

### "Docker daemon not running"

Ensure Docker is installed and running:

```bash
docker ps
```

### Tool fails to execute

1. Check the tool is listed in one of your phases
2. Validate config: `cidx validate`
3. Dry-run to see command: `cidx run --dry-run <target>`
4. Run with verbose: `cidx --verbose run <target>`

## Next Steps

1. **Read the README**: Comprehensive guide with all features
2. **Check examples/**: See `cidx-advanced.toml` for more options
3. **Read CLAUDE.md**: Architecture and development guide
4. **Read docs/getting-started/configuration.md**: Understand `cidx.toml`
5. **Contribute**: Add new presets, report issues, submit PRs

## Getting Help

- View container details: `cidx preset info <container>`
- Check command help: `cidx help <command>`
- Validate config: `cidx validate`
- Dry-run first: `cidx run --dry-run <target>`

Happy DevSecOps! 🚀
