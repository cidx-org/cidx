# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is automatically updated by [Commitizen](https://commitizen-tools.github.io/commitizen/).

## [0.1.0] - 2025-11-19

### Added

#### Core Features

- **Convention Over Configuration**: 20+ built-in presets (trivy, gitleaks, prettier, golangci-lint, go-test, godog, gh-release, docker-buildx, commitizen, etc.)
- **Zero-boilerplate pipelines**: Named pipelines with automatic phase execution
- **Smart defaults**: Tools run with sensible configurations out of the box

#### Environment-Aware Execution

- **Automatic environment detection**: Local vs CI (GitHub Actions, GitLab CI, Jenkins, CircleCI)
- **Local safety modes**: Semi dry-run behavior for dangerous operations (release, docker)
  - `draft` mode: GitHub releases created as drafts locally
  - `no-push` mode: Docker images built without registry push
  - Automatic activation without flags - no `--dry-run` needed
- **CI production mode**: Full execution with deployment capabilities

#### Event-Driven Pipelines

- **Context-aware orchestration**: Different Git events trigger appropriate phases
  - Pull Request → validation only (security, code, test)
  - Merge to main → validation + build (security, code, test, build)
  - Tag push → complete pipeline (security, code, test, build, release, docker)
- **Named pipelines**: `pr`, `main`, `release`, `publish`, `quick`, `pre-push`

#### Behavior-Driven Development

- **Gherkin specifications**: 7 feature files defining CIDX behavior
- **Living documentation**: BDD scenarios serve as executable specifications
- **Dogfooding**: CIDX tests itself using godog and its own pipelines

#### Architecture

- **Phase-Based Organization**: security, code, test, build, release, docker
- **Docker-Native Execution**: Each tool runs in its own Docker container
- **Volume mounting**: Workspace and configuration files automatically mounted
- **Environment expansion**: Variables resolved at runtime (${WORKSPACE}, ${TAG})
- **Container reuse**: Cache preservation for tools like trivy

### Technical Details

- **Language**: Go 1.21+
- **Container Runtime**: Docker (via Docker SDK)
- **Testing**: godog (BDD), go-test (unit)
- **Configuration**: TOML (via BurntSushi/toml)
- **Version Management**: Commitizen for conventional commits and automated changelog

[0.1.0]: https://github.com/arcker/cidx/releases/tag/v0.1.0
