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
[settings]
workspace = "${PWD}"

[tools]
enabled = [
    "megalinter",
    "trivy",
    "gitleaks",
]

[pipelines.ci]
phases = ["security", "code"]
description = "Run security and code quality checks"
```

### 3. List Available Tools

```bash
./bin/cidx list
```

Output:

```
Available tools:

  code:
    - ansible-lint
    - commitlint
    - megalinter

  security:
    - gitleaks
    - trivy

  test:
    - molecule
```

### 4. Get Tool Information

```bash
./bin/cidx info trivy
```

See complete preset configuration including Docker image, volumes, and options.

### 5. Dry-Run

```bash
./bin/cidx run --dry-run ci
```

See what would be executed without actually running containers.

### 6. Run Tools

```bash
# Run a single tool
./bin/cidx run trivy

# Run a pipeline
./bin/cidx run ci
```

## Understanding the Configuration

### Minimal Configuration

90% of users need just this:

```toml
[tools]
enabled = ["trivy", "megalinter"]

[pipelines.ci]
phases = ["security", "code"]
```

CIDX knows:

- ✅ Which Docker image to use
- ✅ What volumes to mount
- ✅ What command to run
- ✅ Environment variables needed

### With Overrides

Only override what you need:

```toml
[tools]
enabled = ["trivy", "megalinter"]

[tools.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1

[tools.megalinter]
flavor = "ansible"
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
cidx init --format yaml  # Creates cidx.yaml
```

#### `cidx list`

List all available presets:

```bash
cidx list
```

#### `cidx info <tool>`

Show detailed information about a tool:

```bash
cidx info trivy
cidx info megalinter
```

#### `cidx validate`

Validate your configuration:

```bash
cidx validate
cidx -c path/to/cidx.toml validate
```

#### `cidx run <target>`

Run a tool or pipeline:

```bash
cidx run trivy           # Run single tool
cidx run ci              # Run pipeline
cidx run --dry-run ci    # Dry-run (don't execute)
```

## Examples

### Example 1: Security Scanning Only

```toml
[tools]
enabled = ["trivy", "gitleaks"]

[pipelines.security]
phases = ["security"]
```

```bash
./bin/cidx run security
```

### Example 2: Code Quality for Ansible

```toml
[tools]
enabled = ["megalinter", "ansible-lint", "molecule"]

[tools.megalinter]
flavor = "ansible"

[pipelines.ansible-ci]
phases = ["code", "test"]
```

```bash
./bin/cidx run ansible-ci
```

### Example 3: Custom Tool

```toml
[tools]
enabled = ["my-scanner"]

[tools.my-scanner]
phase = "security"
image = "mycompany/scanner:latest"
command = "scan ."
volumes = ["${WORKSPACE}:/scan"]
```

```bash
./bin/cidx run my-scanner
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
[tools]
enabled = ["trivy", "megalinter"]
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
[tools.trivy]
severity = "HIGH,CRITICAL"

# Avoid - overriding everything
[tools.trivy]
image = "aquasec/trivy:latest"
command = "trivy fs /scan"
# ... etc
```

## Troubleshooting

### "no cidx config file found"

Solution: Run `cidx init` or specify config path with `-c`.

### "preset not found"

Check tool name with `cidx list`. Names are lowercase with hyphens (e.g., `ansible-lint`).

### "Docker daemon not running"

Ensure Docker is installed and running:

```bash
docker ps
```

### Tool fails to execute

1. Check tool is in enabled list
2. Validate config: `cidx validate`
3. Dry-run to see command: `cidx run --dry-run <tool>`
4. Run with verbose: `cidx --verbose run <tool>`

## Next Steps

1. **Read the README**: Comprehensive guide with all features
2. **Check examples/**: See `cidx-advanced.toml` for more options
3. **Read CLAUDE.md**: Architecture and development guide
4. **Check TODO.md**: Planned features and roadmap
5. **Contribute**: Add new presets, report issues, submit PRs

## Getting Help

- View tool details: `cidx info <tool>`
- Check command help: `cidx help <command>`
- Validate config: `cidx validate`
- Dry-run first: `cidx run --dry-run <target>`

Happy DevSecOps! 🚀
