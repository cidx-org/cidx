# CIDX - CI with Declarative eXecution

[![Container Monitor](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/container-monitor.yml)
[![Security Audit](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml/badge.svg)](https://github.com/cidx-org/cidx/actions/workflows/security-audit.yml)

CIDX is **two tools in one**:

1. **CI/CD Abstraction Layer** - Write once, run everywhere (Local, GitHub Actions, GitLab CI, Jenkins)
2. **Git Workflow Facilitator** - Human-friendly commands that wrap git complexity

## The Problem

**CI/CD Pain:**

- You write the same tool configurations 3-4 times (Local, CI, pre-commit)
- Each CI platform has different syntax
- Configuration drift between local and CI

**Git Pain:**

- Git commands are designed by kernel devs, for kernel devs
- `checkout` does 3 different things, `reset` has 5 modes
- Common workflows require 4-5 commands

## The Solution

```
                    CIDX
                     │
        ┌────────────┴────────────┐
        │                         │
   CI/CD Layer              Git Workflow
        │                         │
   cidx.toml                 cidx action
   cidx run                  cidx branch
   cidx check                cidx demo
```

---

# Part 1: CI/CD Abstraction Layer

**Convention over Configuration** - Just name the container, CIDX knows the rest.

## Quick Start

```bash
# Install
go install github.com/cidx-org/cidx/cmd/cidx@latest

# Initialize
cidx init

# Run
cidx run security   # Run all security containers
cidx run trivy      # Run specific container
cidx run ci         # Run full CI pipeline
```

## Docker Hardened Images (DHI)

CIDX uses **Docker Hardened Images** (dhi.io) by default for maximum security:

- **Near-zero CVEs** - Guaranteed minimal vulnerabilities
- **95% smaller attack surface** - Stripped of unnecessary components
- **SBOM included** - Full software bill of materials
- **SLSA Level 3** - Cryptographic proof of authenticity

### Authentication

DHI requires Docker Hub credentials (free with any Docker Hub account):

```bash
# Check if DHI is ready
cidx registry check

# Login to DHI (uses Docker Hub credentials)
cidx registry login dhi.io

# List all configured registries
cidx registry list
```

For CI/CD, add these secrets to your GitHub repository:

- `DOCKERHUB_USERNAME` - Your Docker Hub username
- `DOCKERHUB_TOKEN` - Docker Hub access token ([create one here](https://hub.docker.com/settings/security))

## Configuration

One file, all environments:

```toml
# cidx.toml

# Enable security containers (CIDX knows Docker images, volumes, commands)
[security]
containers = ["trivy", "gitleaks", "semgrep"]

# Enable code quality containers
[code]
containers = ["megalinter"]

# Define pipelines
[pipelines.pr]
phases = ["security", "code", "test"]

[pipelines.release]
phases = ["security", "code", "test", "build", "docker"]
```

## How It Works

```
CI/CD Runner (GitLab/GitHub/Jenkins/Local)
                    │
                  CIDX
                    │
            ┌───────┴───────┐
            │               │
       cidx.toml     Built-in Presets
            │               │
            └───────┬───────┘
                    │
              Docker Containers
```

- **15+ built-in presets** - Trivy, Gitleaks, Semgrep, MegaLinter, etc.
- **Auto-detection** - Knows images, volumes, commands for each tool
- **5-line configs** - Most projects need less than 10 lines

## Workflow Validation

Ensure your local config matches CI:

```bash
cidx check workflow  # Validates cidx.toml ↔ GitHub Actions
```

---

# Part 2: Git Workflow Facilitator

**Human-friendly git** - Common workflows in single commands.

## Why?

| What you want           | Git commands                                                                    | CIDX command                      |
| ----------------------- | ------------------------------------------------------------------------------- | --------------------------------- |
| Create PR               | `git checkout -b feat/x && git push -u origin feat/x && gh pr create --draft`   | `cidx action pr create "feat: x"` |
| Commit, push, watch CI  | `git add . && git commit -m "x" && git push && gh run watch`                    | `cidx action cpw -m "x"`          |
| Merge PR                | `gh pr merge --squash && git checkout main && git pull && git branch -d feat/x` | `cidx action pr merge`            |
| List stale branches     | `git branch -a --format='%(refname:short) %(committerdate)' \| ...`             | `cidx branch list --stale`        |
| Cleanup merged branches | `git branch --merged \| grep -v main \| xargs git branch -d`                    | `cidx branch cleanup`             |

## Action Commands

```bash
# PR Workflow
cidx action pr create "feat: new feature"  # Create branch + draft PR
cidx action pr ready                        # Mark PR ready for review
cidx action pr merge                        # Merge + cleanup

# Development
cidx action cpw -m "message"               # Commit, Push, Watch CI
cidx action release create                 # Bump version + create release
```

## Branch Management

```bash
# List branches with status
cidx branch list              # All branches with PR/merge status
cidx branch list --stale      # Branches inactive > 30 days
cidx branch list --merged     # Already merged branches
cidx branch list --orphan     # PR closed without merge

# Cleanup
cidx branch cleanup           # Dry-run: show what would be deleted
cidx branch cleanup -x        # Actually delete merged branches
cidx branch cleanup --stale   # Include stale branches

# PR Status
cidx branch pr                # Show PR status for current branch
cidx branch pr -w             # Watch CI checks until complete
cidx branch pr -o             # Open PR in browser
```

## Status Dashboard

Interactive TUI showing project context at a glance:

```bash
cidx status                   # Launch interactive dashboard
cidx status --no-tui          # Simple text output (auto in CI)
```

**Features:**

- GitHub account and authentication status
- Current branch with ahead/behind commits
- Local changes (staged, modified, untracked)
- PR info with CI check status
- Watch mode: press `w` to auto-refresh every 5s

**Keyboard shortcuts:**

- `w` - Toggle watch mode (polls CI checks)
- `r` - Refresh status
- `q` - Quit

**Environment detection:** Automatically uses simple text output in CI environments (GitHub Actions, GitLab CI, Jenkins, etc.) to avoid blocking pipelines.

## Demo

```bash
cidx demo spinner             # See the snake spinner animation
cidx demo spinner -d 10       # Run for 10 seconds
```

---

# Development Workflow

CIDX uses **trunk-based development** with **manual releases**:

```bash
# 1. Create feature branch with draft PR
cidx action pr create "feat: add new feature"

# 2. Work and commit
cidx action cpw -m "feat: implement feature"

# 3. Mark ready and merge
cidx action pr ready
cidx action pr merge

# 4. After 3-5 PRs, create release
cidx action release create
```

---

# Documentation

| Topic                | Link                                                                           |
| -------------------- | ------------------------------------------------------------------------------ |
| Installation         | [docs/getting-started/installation.md](docs/getting-started/installation.md)   |
| Configuration        | [docs/getting-started/configuration.md](docs/getting-started/configuration.md) |
| CI Integration       | [docs/guides/ci-integration.md](docs/guides/ci-integration.md)                 |
| Available Containers | [docs/reference/tools.md](docs/reference/tools.md)                             |
| CLI Reference        | [docs/reference/cli.md](docs/reference/cli.md)                                 |
| Philosophy           | [docs/core-concepts/philosophy.md](docs/core-concepts/philosophy.md)           |
| Developer Guide      | [CLAUDE.md](CLAUDE.md)                                                         |

---

# Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

---

# License

MIT License - see [LICENSE](LICENSE)
