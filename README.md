# CIDX - CI with Declarative eXecution

🚀 **Ultra-declarative DevSecOps pipeline runner** with **Convention over Configuration**

## Philosophy

CIDX makes running DevSecOps tools ridiculously simple:

1. **Just name the tool** - No need to specify Docker images, volumes, commands
2. **Built-in presets** - CIDX knows how to run 15+ common tools out of the box
3. **5-line config** - Most projects need less than 10 lines of configuration
4. **Run Everywhere** - Same config for Local, GitHub Actions, GitLab CI, Jenkins

[📚 Read the full Philosophy](docs/core-concepts/philosophy.md)

## Why CIDX?

**The Problem:** You write the same tool configurations 3-4 times (Local, CI, pre-commit).
**The Solution:** Write once in `cidx.toml`, run everywhere.

```
CI/CD Runner (GitLab/GitHub/Jenkins/Local)
           ↓
         CIDX (declarative config)
           ↓
     Your Project (any tech)
```

## Quick Start

### Installation

```bash
go install github.com/arcker/cidx/cmd/cidx@latest
```

[More installation options](docs/getting-started/installation.md)

### Usage

Initialize configuration:

```bash
cidx init
```

Run tools:

```bash
cidx run security   # Run all security tools
cidx run trivy      # Run just trivy
cidx run ci         # Run full CI pipeline
```

## Configuration

Configuration is handled in `cidx.toml`. You define `pipelines` that map to CI/CD events like Pull Requests or Git tags. CIDX automatically detects the context and runs the correct pipeline.

**Example `cidx.toml`:**
```toml
# The 'pr' pipeline runs on Pull Requests.
[pipelines.pr]
phases = ["security", "code", "test"]

# The 'release' pipeline runs when a tag is pushed.
[pipelines.release]
phases = ["security", "code", "test", "build", "release", "docker"]
```

This allows you to manage your entire CI/CD logic in one file, with CIDX handling the "when to run what" automatically, based on convention.

[📚 Full Configuration Guide](docs/getting-started/configuration.md)

## Documentation

- **[Getting Started](docs/getting-started/quick-start.md)**
- **[CI/CD Integration](docs/guides/ci-integration.md)**
- **[Available Tools](docs/reference/tools.md)**
- **[CLI Reference](docs/reference/cli.md)**
- **[Developer Guide](CLAUDE.md)**

## Key Features

- **Docker-First**: All tools run in isolated containers.
- **Container Reuse**: 6x faster subsequent runs. [Learn more](docs/core-concepts/container-reuse.md)
- **BDD-Tested**: Behavior specified in executable Gherkin scenarios.
- **Event-Driven**: Automatically detects PRs, tags, and commits.

## License

MIT License - see [LICENSE](LICENSE)
