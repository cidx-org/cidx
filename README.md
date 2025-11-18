# CIDX - CI with Declarative eXecution

🚀 **Ultra-declarative DevSecOps pipeline runner** with **Convention over Configuration**

## Philosophy

CIDX makes running DevSecOps tools ridiculously simple:

1. **Just name the tool** - No need to specify Docker images, volumes, commands
2. **Built-in presets** - CIDX knows how to run 6+ common tools out of the box
3. **5-line config** - Most projects need less than 10 lines of configuration
4. **Zero learning curve** - If you know the tool name, you can use it

## Why CIDX?

### The DevOps Repetition Problem

As DevOps engineers, **we're always doing the same things**:

- Run security scans (Trivy, Gitleaks, Semgrep)
- Check code quality (MegaLinter, ESLint, Prettier)
- Lint commit messages (Commitlint)
- Test infrastructure (Molecule, Terraform)
- Build and deploy

**Every project needs these tools. But the implementation is different everywhere:**

- GitLab CI uses `.gitlab-ci.yml`
- GitHub Actions uses `.github/workflows/*.yml`
- Local development uses `Makefile`, `Taskfile`, or shell scripts
- Jenkins uses `Jenkinsfile`

**Result:** You write the same tool configurations 3-4 times for each project.

### The Factorization Solution

**CIDX solves this with maximum factorization:**

```toml
# cidx.toml - ONE config for ALL environments
[security]
tools = ["trivy", "gitleaks"]

[code]
tools = ["megalinter", "commitlint"]
```

**This config works:**

- ✅ On your local machine (`cidx run security`)
- ✅ In GitLab CI (`.gitlab-ci.yml` calls `cidx run security`)
- ✅ In GitHub Actions (workflow calls `cidx run security`)
- ✅ In Jenkins (Jenkinsfile calls `cidx run security`)

**One config. Multiple runners. Zero duplication.**

### Universal Interface: Runner ↔ CIDX ↔ Project

CIDX acts as the **universal abstraction layer** between:

1. **CI/CD Runners** (where code runs) - GitLab CI, GitHub Actions, Jenkins, local
2. **DevSecOps Tools** (what you run) - Trivy, MegaLinter, Gitleaks, etc.
3. **Your Projects** (your codebase) - Any language, any framework

```
CI/CD Runner (GitLab/GitHub/Jenkins/Local)
           ↓
         CIDX (declarative config)
           ↓
     Your Project (any tech)
```

### Benefits

- **Write once, run everywhere** - Define tools once, works on all CI/CD platforms
- **Change once, apply everywhere** - Update tool versions in one place
- **Easy migration** - Switch CI platforms without changing tool configs
- **Consistent results** - Same Docker images, same versions, everywhere
- **Better DX** - Developers test locally with same commands as CI

### Docker-First Architecture

**CIDX executes all tools in Docker containers.** This is a deliberate choice that aligns with modern CI/CD practices:

- **GitLab CI** uses Docker-in-Docker (dind) for containerized jobs
- **GitHub Actions** uses container jobs
- **Jenkins** supports Docker agents
- **Local** development with Docker ensures parity with CI

**Why Docker?**

✅ **No host pollution** - Tools don't install dependencies on the host machine  
✅ **Reproducibility** - Same container = same environment everywhere  
✅ **Isolation** - Each tool runs in its own isolated container  
✅ **Version control** - Docker image tags ensure exact versions  
✅ **Security** - Containers provide sandboxing and resource limits

**Example:** Running Trivy doesn't require installing Trivy on your machine. CIDX pulls `aquasec/trivy:0.57.1` and runs it in a container.

```bash
# No installation needed, just Docker
cidx run trivy
# → Pulls aquasec/trivy:0.57.1
# → Mounts your project
# → Runs scan
# → Exits cleanly
```

Your host stays clean. Your CI stays clean. Everything is containerized.

## Quick Start

### Installation

```bash
go install github.com/arcker/cidx/cmd/cidx@latest
```

Or build from source:

```bash
git clone https://github.com/arcker/cidx.git
cd cidx
go build -o bin/cidx ./cmd/cidx
```

### Initialize Configuration

```bash
cidx init
```

This creates a `cidx.toml` file:

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

### Run Tools

```bash
# Run a specific tool
cidx run trivy

# Run a pipeline
cidx run ci

# Dry-run to see what would execute
cidx run megalinter --dry-run
```

## Available Tools

List all built-in presets:

```bash
cidx list
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

Get detailed information about a tool:

```bash
cidx info trivy
```

## Configuration

### Minimal Configuration (90% of cases)

Just enable tools by name:

```toml
[tools]
enabled = ["megalinter", "trivy", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]
```

**That's it!** CIDX knows:

- ✅ Which Docker image to use
- ✅ What volumes to mount
- ✅ What command to run
- ✅ What environment variables to set

### With Overrides (10% of cases)

Override only what you need:

```toml
[tools]
enabled = ["megalinter", "trivy"]

[tools.megalinter]
flavor = "ansible"
env = { LINTERS_DISABLED = "v8r" }

[tools.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

### Custom Tools

Define your own tools when presets don't exist:

```toml
[tools]
enabled = ["megalinter", "my-scanner"]

[tools.my-scanner]
phase = "security"
image = "myregistry/scanner:latest"
command = "scan ."
volumes = ["${WORKSPACE}:/scan"]
```

## Built-in Presets

### Security Phase

#### Trivy

Vulnerability scanner for containers and filesystems.

```toml
[tools.trivy]
severity = "HIGH,CRITICAL"  # Filter by severity
exit_code = 1               # Fail on vulnerabilities
```

#### Gitleaks

Detect hardcoded secrets in git repositories.

```toml
[tools.gitleaks]
# No configuration needed - works out of the box
```

### Code Phase

#### MegaLinter

Code quality and security scanning with 50+ linters.

```toml
[tools.megalinter]
flavor = "ansible"  # Use specific flavor
env = { LINTERS_DISABLED = "v8r" }
```

#### Commitlint

Enforce conventional commit messages.

```toml
[tools.commitlint]
# Automatically uses .commitlintrc.json if present
```

#### Ansible Lint

Lint Ansible playbooks and roles.

```toml
[tools.ansible-lint]
# Automatically uses .ansible-lint config
```

### Test Phase

#### Molecule

Test Ansible roles with Docker.

```toml
[tools.molecule]
scenario = "default"  # Specify scenario
```

## CLI Commands

### `cidx run`

Execute a tool or pipeline:

```bash
cidx run <tool|pipeline>
cidx run trivy
cidx run ci
cidx run megalinter --dry-run
```

### `cidx list`

List all available tools:

```bash
cidx list
```

### `cidx info`

Show detailed information about a tool:

```bash
cidx info trivy
cidx info megalinter
```

### `cidx validate`

Validate your configuration:

```bash
cidx validate
```

Output:

```
Validating: cidx.toml

✓ Configuration is valid
```

### `cidx init`

Create a new configuration file:

```bash
cidx init
cidx init --format yaml
```

## Pipelines

Define execution sequences:

```toml
[pipelines.security]
phases = ["security"]
description = "Security scanning only"

[pipelines.code]
phases = ["code"]
description = "Code quality checks only"

[pipelines.ci]
phases = ["security", "code", "test"]
description = "Complete CI pipeline"
```

Run with:

```bash
cidx run security
cidx run ci
```

## Environment Variables

CIDX supports environment variable expansion:

```toml
[settings]
workspace = "${PWD}"
registry = "${DOCKER_REGISTRY}"

[tools.commitlint]
env = { FROM = "${CI_COMMIT_BEFORE_SHA}", TO = "${CI_COMMIT_SHA}" }
```

## Project Structure

```
cidx/
├── cmd/
│   └── cidx/          # CLI commands
├── pkg/
│   ├── config/        # Configuration parser
│   ├── presets/       # Built-in tool presets
│   ├── executor/      # Docker execution engine
│   └── pipeline/      # Pipeline orchestration
├── examples/          # Example configurations
└── README.md
```

## Why CIDX?

### vs Just

- **Just**: You write Docker commands manually
- **CIDX**: Built-in presets, zero configuration

### vs Earthly

- **Earthly**: Custom DSL to learn
- **CIDX**: Simple TOML/YAML, just tool names

### vs Dagger

- **Dagger**: Write Go code for each tool
- **CIDX**: Enable by name, that's all

### vs Traditional CI Config

- **Traditional**: 50+ files, complex YAML
- **CIDX**: 1 file, 5-10 lines

## Development

### Build

```bash
go build -o bin/cidx ./cmd/cidx
```

### Test

```bash
go test ./...
```

### Add a New Preset

Edit `pkg/presets/registry.go`:

```go
"newtool": {
    Name:    "newtool",
    Phase:   "security",
    Image:   "myorg/newtool:latest",
    Command: "newtool scan .",
    Workdir: "/scan",
    Volumes: []string{"${WORKSPACE}:/scan"},
},
```

## Documentation

For detailed documentation, see the [docs/](docs/) directory:

- **[Container Reuse & Caching](docs/container-reuse.md)** - Performance optimization through container reuse (6x faster!)
- **[Documentation Index](docs/README.md)** - Complete documentation overview

### Key Features

#### Container Reuse for Performance

CIDX reuses Docker containers across runs instead of creating/deleting them each time. This provides:

- **6x faster** subsequent runs (e.g., security phase: 15s → 2.4s)
- **Cache preservation** (Trivy DB, node_modules, etc.)
- **Better developer experience** with instant feedback

Containers are automatically reused with fixed names (`cidx_trivy`, `cidx_gitleaks`, etc.). See [Container Reuse documentation](docs/container-reuse.md) for details.

## License

MIT License - see [LICENSE](LICENSE)

## Contributing

Contributions welcome! Please open an issue or PR.

### Adding New Presets

We welcome presets for popular DevSecOps tools:

- Security scanners (SAST, DAST, secrets detection)
- Code quality tools (linters, formatters)
- Testing tools (unit, integration, e2e)
- Compliance tools (license checking, SBOM generation)

## Roadmap

- [ ] 20+ presets for common tools
- [ ] Auto-detection of project type
- [ ] User preset extensions (~/.config/cidx/presets.toml)
- [ ] Parallel tool execution
- [ ] Web UI for configuration
- [ ] GitLab/GitHub CI templates
