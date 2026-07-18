## [Unreleased]

## v2.1.0 (2026-07-18)

### Feat

- **presets**: add probatum preset (test phase) (#160)
- **actions**: add `cidx pr edit` to update the current branch's PR title/body — titles are no longer frozen at `pr create` time, so a diagnosis that changes mid-investigation can be reflected before the squash-merge reuses the title as the commit message (#169, #176)

### Fix

- **presets**: use the prebuilt cargo-audit release binary in the `cargo-audit` preset — `cargo install cargo-audit` compiled the tool from source on every pipeline run (minutes of build time, and compilation could fail for reasons unrelated to the audited project). The preset now downloads the pinned v0.22.2 binary from RustSec's GitHub releases (seconds, `$(uname -m)` covers x86_64 and aarch64) and runs `cargo audit` as before. (#161, #164)
- **executor**: look up registry-specific credentials (e.g. the `dhi.io` key written by `cidx registry login dhi.io`) instead of only Docker Hub's, and retry once anonymously when a registry rejects the attached credentials — previously the very login command suggested by the error message could not fix the failure (#162, #165)
- **generate**: pin the bootstrapped cidx in generated workflows to the generating version (`go install ...@vX.Y.Z`), falling back to `@latest` with a warning on dev builds — the same commit can no longer be green locally and red in CI because of preset drift (#163, #166)
- **actions**: `cpw` reuses the PR-checks wait path with the freshly pushed HEAD SHA pinned, instead of a single hardcoded workflow-file lookup after a fixed 5s sleep — no more spurious "No CI workflow found / Create a PR first" while GitHub Actions is still starting (#167, #172)
- **actions**: derive the PR branch prefix from the conventional-commit type (`fix:` → `fix/...`), and point `pr create` hints at `cidx cpw`/`cidx pr ready` instead of raw git and the deprecated `cidx action` (#168, #173)
- **drift**: resolve the workflow file (`cidx.yml`, then `ci.yml`) through one shared helper instead of hardcoding divergent names in `check drift` and `GetLatestWorkflow` — freshly generated projects no longer need `--file`, and workflow lookups work on repos whose workflow is `ci.yml` (#170, #177)
- **infra**: gofmt sweep (20 files) and enforcement — new `.golangci.yml` enables the gofmt formatter so CI fails on formatting drift (#171, #179)

## v2.0.0 (2026-05-20)

### Feat

- **init**: detect fullstack monorepo layouts (Python `backend/` + Node `frontend/`, `apps/*`, `services/*`, `packages/*`) — `cidx init` now walks immediate subdirectories in addition to the repo root and aggregates per-phase containers across all detected stacks, eliminating the "No language detected" fallback on real fullstack projects (#145)

### Fix

- **presets**: install rustfmt component in `rustfmt` preset — `rust:1.95.0` does not ship rustfmt by default, so `cargo fmt --check` failed immediately on first run. Now runs `sh -c 'rustup component add rustfmt && cargo fmt -- --check'`, matching the existing `clippy` preset pattern. (#150)
- **presets**: unify built-in mount paths at `/work` across all 40+ presets — previously `/src`, `/work`, `/scan`, `/repo`, `/app`, `/workspace` were used inconsistently, breaking the override mental model in monorepos. A `[containers.prettier] workdir = "/src/client-react"` override silently failed because the preset still mounted at `/work`. The runner now refuses to start a container whose workdir is not covered by any volume mount target and reports the available targets in the error. `cidx preset info` documents the mount contract explicitly. **Breaking change** for any cidx.toml that pins `workdir` to one of the legacy paths without also setting `volumes`; migrate by either dropping the override (defaults work) or by overriding `volumes` so the workdir is covered. (#151)
- **executor**: detect stale container config via SHA-256 label hash; recreate `cidx_<tool>` containers when cidx.toml's behavior-affecting fields (image, command, workdir, entrypoint, volumes, env) change between runs. Containers from pre-#144 cidx versions (no `cidx.config_hash` label) are also treated as stale. `CIDX_NO_REUSE=1` forces recreate. Also writes a `cidx.version` label on every created container. (#144)
- **config**: accept `[containers.NAME]` with `image` field as a custom container declaration instead of rejecting it as "unknown container" (#142)
- **presets**: honor `volumes` and `entrypoint` overrides in `[containers.NAME]` — previous `[]string` type assertion silently dropped them since arrays decode as `[]any` (#143)

## v1.7.0 (2026-05-11)

### Feat

- **workflow**: add `cidx workflow watch` for non-PR branches (#125) (#132)
- init --diff/--update + public release sanitization (#120)

### Fix

- **executor**: trim replay of previous-run logs on reused containers (#127) (#134)
- **generate-github**: emit go install for external projects (#124) (#131)
- **presets**: per-key env override for [containers.X] in cidx.toml (#130)
- **presets**: use public Docker Hub for Rust + drop hardcoded toolchain env (#129)
- **infra**: commitizen preset scope + QF1012 sweep (#126)
- optional DHI login for Dependabot PRs (#107)
- align Go version in CI/release workflows with go.mod (#106)

### Refactor

- **cidx.toml**: extract complete catalog to examples/cidx-complete.toml (#135)
- reorganize CLI hierarchy around product core

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
