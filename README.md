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

## Development Workflow

CIDX itself uses **trunk-based development** with **manual releases**:

### Daily Development (PRs)
```bash
# 1. Create feature branch with draft PR
cidx action pr create "feat: add new feature"

# 2. Implement and commit
git commit -m "feat: implement feature"
git push

# 3. Mark ready for review
cidx action pr ready

# 4. Merge to main (no tag created)
cidx action pr merge
```

### Creating Releases
After merging 3-5 PRs to main, create a release:

```bash
cidx action release create
```

This will:
- Analyze commits since last tag
- Bump version automatically (PATCH/MINOR/MAJOR)
- Create and push git tag (e.g., v1.1.1)
- Trigger GitHub Release workflow
- Publish release with binary + Docker image

**Key principle:** Tags = Releases (1:1). PRs don't create tags, releases are manual and group multiple features.

[📚 Full Development Workflow Guide](docs/guides/development-workflow.md)

## Contributing

We welcome contributions! See [CONTRIBUTING.md](CONTRIBUTING.md) for:
- How to submit PRs
- Commit message conventions
- Development setup
- Testing guidelines

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
