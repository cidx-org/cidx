## v1.6.2 (2026-04-08)

### Fix

- strip debug symbols from release binaries (-s -w) (#104)
- remove unused version-bump container from cidx.toml (#94)

## v1.6.1 (2026-04-07)

### Fix

- multi-platform release binaries with checksums (#92)

## v1.6.0 (2026-04-07)

### Feat

- cidx init generates CI workflow alongside cidx.toml (#91)

### Fix

- add .trivyignore and handle empty commit range in commitizen (#89)

## v1.5.0 (2026-04-07)

### Feat

- auto-quiet mode in CI environments (#87)

### Fix

- resolve env references in command expansion (#88)

## v1.4.0 (2026-04-07)

### Feat

- smart project detection in cidx init (#85)
- add cidx generate gitlab command (#83)
- add quiet mode to branch pr -w for minimal output (#79)
- implement Podman support via Docker-compatible socket (#75)

### Fix

- commitizen scans from last tag and fix empty entrypoint parsing (#78)
- block cpw from pushing directly to main/master (#77)
- add userns keep-id for Podman rootless volume permissions
- override gh-release entrypoint to allow shell commands (#74)

### Refactor

- move pr status/watch/open under cidx pr subcommand (#82)

## v1.3.1 (2026-04-04)

### Fix

- disable required_version check that blocks release workflow (#72)

## v1.3.0 (2026-04-04)

### Feat

- add cidx preset audit for compliance reporting (#70)
- promote pr and cpw as top-level commands (#69)
- add configurable timeout per container (#68)
- add cidx cleanup command to remove stopped containers (#67)
- add pull_policy support for container image management (#63)
- add cidx check drift to compare cidx.toml with CI workflow (#58)
- add cidx generate github command (#57)
- add cidx doctor command for environment diagnostics (#55)
- **presets**: migrate to Docker Hardened Images (DHI) by default (#41)
- **pipeline**: add parallel execution mode for local runs (#40)
- **executor**: add executor interface abstraction layer (#38)
- **executor**: add executor interface abstraction layer
- **pr**: reuse existing branch without PR when running pr create (#37)
- **pr**: return to main TUI screen in merging mode after merge (#35)
- **pr**: add merge post actions (#33)
- **tui**: add interactive PR merge interface (#31)
- **tui**: add interactive TUI for tag and release creation (#30)
- **artifact**: add artifact management commands for GitHub Actions (#29)
- **remote**: add GitLab support with auto-detection (#28)
- auto cleanup after PR merge (#26)
- **tag**: add tag management commands with prepare/preview/create workflow (#24)
- **release**: add prepare and preview commands for human-friendly releases (#23)

### Fix

- graceful handling when cpw finds no CI workflow (#71)
- deduplicate known-vulnerabilities.toml on every save (#64)
- auto-set upstream on push for new branches (#62)
- treat skipped/neutral conclusions as success across all displays (#61)
- use Getwd over PWD and add pipeline execution summary (#60)
- treat skipped/neutral CI checks as success, not failure (#59)
- **test**: enable strict BDD mode and tag docker-dependent scenarios (#54)
- **ci**: build CIDX binary once and share via artifact (#45)
- **test**: resolve all BDD test failures and add missing step definitions
- **pr**: apply defaults when [pr] config section missing (#34)
- **pr**: wait for CI to start before merge checks (#25)

### Refactor

- split github client.go by domain (#43)
- extract shared TUI styles and split large files (#42)
- comprehensive code quality improvements across codebase
- **ci**: simplify release workflow

## v1.2.0 (2025-12-04)

### Feat

- **status**: add watch mode for real-time CI monitoring (#22)
- **status**: add interactive TUI dashboard (#21)
- **presets**: add Python and Rust language packs (#20)
- **presets**: add complete Ansible language pack (#19)
- **vuln**: add report command and enhance check with auto-cleanup (#18)
- **scan**: add --verbose flag for real-time container logs
- **security**: add vulnerability exceptions for ansible dev-tools
- **presets**: migrate Ansible containers to community-ansible-dev-tools
- **vuln**: add GHSA support for Grype scanning
- **vuln**: add verify command for local testing
- **vuln**: add vulnerability exception management system
- **security**: enhance container monitoring with multi-scanner support
- add auto version bump with commitizen to container monitor
- add container version monitoring (#14)
- add preset management commands and Go security presets (#12)
- add about command with credits
- equalizer spinner and demo command (#10)
- add branch management commands with cidx branch list (#9)
- add workflow validation to check cidx.toml against GitHub Actions (#6)

### Fix

- **release**: use github-release pipeline in CI
- **security**: handle registry auth failures gracefully
- **security**: add Trivy-specific vulnerability exceptions
- **security**: restore fail behavior and add vulnerability exceptions
- **lint**: check error return from os.Remove in defer
- **security**: convert audit to reporting mode
- **security**: use job-index for unique artifact names
- **security**: improve audit report with detailed vulnerabilities
- **security**: remove read-only flag from Trivy cache mount
- **security**: fix database permissions after Docker download
- **security**: use registry scheme for Grype scans
- **security**: make workflow fail on vulnerabilities
- exclude BDD tests from container monitor validation
- preserve variant suffix when checking container updates
- filter non-semver tags in container update check
- proper exit codes for container monitor workflow
- clean JSON output for check-updates and fix workflow field names

### Refactor

- simplify container-monitor and add security-audit workflow
- rename tools to containers to reflect container-first philosophy (#5)
- reorganize cidx.toml for better clarity (#4)

### Perf

- **security**: add database caching and consolidated report
- optimize container-monitor with parallel scanning and deduplication

## v1.1.1 (2025-11-26)

### Fix

- disable changelog update on version bump to fix exit code 16
- add git safe.directory via environment variables
- remove cz prefix from release-create command
- keep command as single element when entrypoint is set
- add entrypoint override support for commitizen container

## v1.1.0 (2025-11-25)

### Feat

- implement PR workflow with GitHub API (#1)

## v1.0.0 (2025-11-24)

### Fix

- use GIT_TAG env var in release workflow for proper release naming
- expand environment variables in tool config

## v0.3.0 (2025-11-24)

### Feat

- split CI and Release workflows

## v0.2.0 (2025-11-24)

### Feat

- add dynamic actions system with release-create action
- use git binary for commit/push to ensure pre-commit hooks execution
- add commit-push-watch action with gh CLI auth support

### Fix

- reset CHANGELOG to v0.1.0 for clean version bump
- add git safe.directory config to release action
- remove files-only flag and reset to 0.1.0
- reset version to 0.1.0 and use files-only for bump
- add changelog flag to commitizen bump
- add --no-verify flag to commitizen bump command
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
