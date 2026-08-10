# CIDX Documentation

Welcome to the CIDX (CI with Declarative eXecution) documentation.

## 📚 Documentation Structure

### 🚀 Getting Started

- [Quick Start](getting-started/quick-start.md) - Step-by-step guide to get running
- [Installation](getting-started/installation.md) - Installation methods (Binary, Go, Docker)
- [Configuration](getting-started/configuration.md) - Understanding `cidx.toml`

### 🧠 Core Concepts

- [Philosophy](core-concepts/philosophy.md) - Why CIDX exists and its design principles
- [DevOps Integration](core-concepts/devops.md) - How CIDX fits into your DevOps loop
- [Environments & Local Safety](core-concepts/environments.md) - Environment detection and local safety modes
- [Supply-Chain Policy](core-concepts/supply-chain-policy.md) - How the images CIDX runs are pinned and chosen
- [The Image Lifecycle](core-concepts/image-lifecycle.md) - How CIDX notices an image has moved, frozen or died
- [Vulnerability Management](core-concepts/vulnerability-management.md) - Judging findings, exceptions, where to look
- [The Image Supply Chain, in Diagrams](core-concepts/image-supply-chain.md) - The same machinery as a map
- [What Leaves the Container](core-concepts/artifacts-across-containers.md) - Why a built binary reports "not found" when it is right there
- [The Image Supply Chain](core-concepts/image-supply-chain.md) - Diagrams: how a pinned image is watched, gated and replaced
- [Container Reuse](core-concepts/container-reuse.md) - Performance optimization details

### 📖 Guides

- [Development Workflow](guides/development-workflow.md) - Trunk-based development with PRs and releases
- [Creating Presets](guides/creating-presets.md) - How to add new containers to CIDX
- [CI/CD Integration](guides/ci-integration.md) - Setting up GitHub Actions, GitLab CI, etc.

### 🔧 Reference

- [CLI Reference](reference/cli.md) - Command line interface documentation
- [Containers Registry](reference/tools.md) - List of all available container presets

## 🤝 Contributing

See [CLAUDE.md](../CLAUDE.md) for development guidelines.
