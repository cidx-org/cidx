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

## v0.2.0 (2025-11-24)

### Feat

- add dynamic actions system with release-create action
- use git binary for commit/push to ensure pre-commit hooks execution
- add commit-push-watch action with gh CLI auth support

### Fix

- correct commitizen command in release-create action
- expand WORKSPACE variable in action volumes
- ignore untracked files in HasChanges check
- **lint**: use tagged switch for job conclusion checks

## v0.1.0 (2025-11-20)

### BREAKING CHANGE

- none

### Feat

- **executor**: add privileged mode for tools requiring root
- initial release - convention-based CI/CD orchestration
- Add environment detection and local safety modes
- Add Docker and Release phases managed by CIDX
- Add Docker image publishing to GitHub Container Registry
- Separate GitHub Actions jobs per CIDX phase
- Add named pipelines and GitHub Actions integration
- Add pre-commit hooks for security and code quality
- Add dogfooding setup - CIDX runs on itself

### Fix

- download artifacts to bin/ directory to preserve structure
- use GH_TOKEN for gh CLI auth and fix artifact directory structure
- **presets**: enable privileged mode for gh-release to allow apk package installation
- **presets**: use Alpine with GITHUB_TOKEN env var for gh-release
- **presets**: use maniator/gh Docker image for gh-release
- **presets**: use ubuntu:latest with official gh CLI installation for gh-release
- **presets**: use official ghcr.io/cli/gh Docker image for gh-release
- **presets**: install git before configuring safe.directory
- **presets**: add git safe.directory config for gh-release
- **presets**: add privileged flag to gh-release for package installation
- **presets**: use alpine with gh CLI installation for gh-release
- **presets**: use correct GitHub CLI image for gh-release
- **docker**: revert to Go 1.25.4 and use golang:alpine
- **go**: correct Go version from 1.25.4 to 1.24.0
- **presets**: load Privileged field from TOML
- **presets**: add TOML tag to Privileged field for proper parsing
- **presets**: correct kaniko command and gh-release image
- **docker**: switch from docker-buildx to kaniko for rootless builds
- **ci**: add fetch-depth and HOME for release/docker jobs
- **ci**: fetch full git history for gitleaks and commitizen
- Correct Mermaid diagram syntax for proper rendering
- Code quality improvements - fix all golangci-lint issues

### Refactor

- simplify GitHub Release with maniator/gh container
- use pre-built binary in Docker image instead of rebuilding
