## v3.4.1 (2026-09-04)

### Fix

- **executor**: make runtime detection resilient (#470)

## v3.4.0 (2026-09-03)

### Feat

- **repo**: make cpw resume the current pull request (#460)
- **security**: validate Renovate container promotions (#455)
- **preset**: generate the catalogue reference page the hand-written one rotted into (#453)
- **security**: convert the five context-sensitive groups to grouped decisions (#448)
- **security**: convert the Ansible group to grouped decisions (#446)
- **security**: surface grouped-decision verdicts in the views (#444)
- **security**: resolve waivers through grouped decisions (#443)
- **security**: specify grouped vulnerability decisions (#441)

### Fix

- **release**: count truncated tag changes correctly (#468)
- **repo**: explain completed workflow failures (#467)
- **security**: refresh vulnerability databases daily (#462)
- **repo**: guide unfinished pull requests before starting (#461)
- **deps**: scope Renovate to public catalogue images (#459)
- **presets**: keep catalogue reports and promotions current (#454)
- **presets**: repin the DHI images their publisher rebuilt, and lift trivy to 0.74 (#452)
- **doctor**: stage the hardened-registry credential the scenario asserts on (#451)
- **security**: count what is left for a human, not what was already argued (#439)

## v3.3.0 (2026-08-20)

### Feat

- **config**: let a container declare it must not be reused (#435)

### Fix

- **security**: gate the audit on what needs a human, not on every finding (#436)
- **run**: let a dry run answer where nothing can run (#427)
- **presets**: build an artifact that runs where the user runs it (#423)

## v3.2.0 (2026-08-09)

### Feat

- **cli**: refuse a merge that would land a commit you do not have (#415)
- **cli**: refuse a watch that reports on a commit you do not have (#414)
- **security**: see a tag rebuilt under the same name (#408)

### Fix

- **release**: commit the release notes, not the scratch state beside them (#418)
- **cli**: stop forcing a branch deletion git had just refused (#417)
- **cli**: push what cpw has, not only what it just made (#416)
- **security**: pin the tools that enforce the pinning policy, and map the machinery (#411)

## v3.1.0 (2026-08-08)

### Feat

- **presets**: record the five DHI entrypoints, completing the catalogue (#397)
- **presets**: catch a command that hands its tool its own name, offline (#396)
- **monitor**: report a pinned tag nobody has republished in over a year (#393)
- **pr**: warn when the commit and the PR title disagree about what kind of change this is (#392)

### Fix

- **release**: stop release create refusing the next release on its own leftover (#404)
- **release**: read BREAKING CHANGE as the footer it is, not as a substring (#403)
- **release**: move the module path to /v3 and refuse a major the path cannot publish (#401)
- **cli**: make a broken registry credential legible (#400)
- **artifact**: keep one run's evidence in a results directory, never two (#391)
- **monitor**: give a promotion PR a current baseline, and stop pr watch calling an empty check list green (#386)

### Refactor

- **test**: move the godog suite into features/ and the repository guards into internal/guards (#390)

## v3.0.0 (2026-08-06)

### BREAKING CHANGE

- cidx.toml is now strict. A config carrying a key cidx does
not read — a typo, a key removed in v3.0.0 such as [branch] auto_cleanup, or
config_files in a container override — fails on load, for every command rather
than only cidx validate. The error names every offending key at once, so a
file with three mistakes takes one run to fix rather than three.
- local_behavior = 'no-push' is removed. Use 'dry-run', which
is what it did. A preset or config still declaring it fails with an error
naming the replacement. Nothing changes about what runs locally: no-push
already built nothing.
- [branch] auto_cleanup is removed. It never had an effect, so
nothing changes at runtime for anyone who set it; the key simply stops being
declared. Branch deletion after a merge is unconditional, as it already was.
- the go-test preset runs 'go test -v ./...' instead of
'go test -v ./pkg/... ./cmd/...'. A project whose root-package or internal/
tests were failing unnoticed will see its test phase go red on upgrade. That
is the correct outcome — those tests were never being run — but it arrives as
a surprise, so it is called out here rather than left to be discovered. A
project that genuinely wants the old scope can restore it with a
'[containers.go-test] command' override.
- 'cidx action ...' is removed. Every subcommand it wrapped is
reachable under 'repo', 'release' or 'security': 'cidx action pr' is 'cidx pr',
'cidx action cpw' is 'cidx cpw', 'cidx action tag' is 'cidx release tag',
'cidx action release' is 'cidx release', 'cidx action artifact' is
'cidx repo artifact'. Subcommands are unchanged on both sides.

### Feat

- **config**: refuse cidx.toml keys that cidx does not read (#375)
- **config**: honour the pipeline description the decoder was dropping (#374)
- **presets**: run every package in the go-test default (#366)
- **cli**: remove the deprecated 'cidx action' command tree (#363)
- **repo**: download run artifacts and rerun failed jobs from cidx (#346)
- **cli**: warn on deprecated action commands and name their replacement (#320)
- **audit**: publish a vulnerability status summary to a tracking issue (#308)
- **presets**: flag images whose base is approaching end of life (#305)
- **audit**: publish catalogue scan results to code scanning (#301)

### Fix

- **monitor**: hold a candidate only for what it introduces over the running image (#380)
- **release**: name each PR once in the notes, and take the squash marker not the first number (#377)
- **pr**: stop reporting the trunk having no PR as a failure (#373)
- **presets**: make the security floor options usable, and leave the floors to the project (#372)
- **pr**: stop concluding CI is done when the next jobs do not exist yet (#368)
- **git**: pin git's locale so CIDX reads the messages it matches on (#365)
- **pr**: name the step a failing check died on (#360)
- **cli**: guard the test scope and build the provider only when needed (#356)
- **cli**: name the failing check and stop printing the verdict twice (#354)
- **cli**: keep pr create --dry-run offline and stream container output without debug (#348)
- **cli**: say what was checked, fail on a missing exceptions file, print the identifier watch accepts (#339)
- **presets**: repair commitlint and stop ansible and go-mod-tidy polluting the workspace (#336)
- **ci**: tell create-pull-request what to commit, and check before pushing (#335)
- **docker**: ship the cidx binary in the published image (#334)
- **security**: have the audit state what its ignore file suppressed instead of inferring it from an absence (#333)
- **presets**: refuse an update candidate outside the pinned tag's variant family or from a development channel (#331)
- **security**: stop an expired acceptance from filtering anything (#327)
- **security**: count what the ignore file hides and keep the baseline honest (#326)
- **security**: let an exception retire when its CVE is gone (#324)
- **branch**: scope cleanup to one branch and refuse an open PR without --force (#321)
- **validator**: pair a pipeline with the workflow it declares, not the one its name suggests (#319)
- **validator**: parse cidx invocations instead of matching a substring (#316)
- **security**: key alerts on the repository and report fixable live exceptions (#314)

### Refactor

- **safety**: remove no-push, which was dry-run with a promise it could not keep (#370)
- **config**: remove the inert [branch] auto_cleanup key (#369)
- **cli**: make the real command tree reachable from the BDD suite (#343)
- **presets**: drop the option default nothing ever applied (#340)

## v2.4.0 (2026-08-01)

### Feat

- **security**: key exceptions by CVE and triage findings by fixability (#297)
- **security**: expire dead vulnerability exceptions and generate the catalogue security baseline (#283)
- **workflow**: add cidx workflow run to trigger a workflow (#267)
- **validate**: catch stale cidx invocations in workflow files (#263)

### Fix

- **presets**: run cargo-audit off a minimal image instead of the Rust toolchain (#298)
- **presets**: give the Python and Go presets writable paths, refresh trivy and the GitLab generator (#294)
- **presets**: move the DHI images off their frozen variant lines (#293)
- **ci**: check formatting instead of rewriting it (#290)
- **audit**: give the scanners the credentials they need for hardened images (#288)
- **presets**: move catalogue images to their slim variants (#286)
- **presets**: replace abandoned and unmaintained catalogue images (#280)
- **presets**: update catalogue images carrying known vulnerabilities (#277)
- **cli**: refuse flags placed after positional arguments (#274)
- **checks**: scope PR checks to the pull request, honour --config everywhere (#264)
- **presets**: scope the catalogue, flag frozen variant lines, decouple registry logins (#262)
- **cli**: resolve remotes lazily and stop overstating what ran (#261)

## v2.3.0 (2026-07-28)

### Feat

- **presets**: detect image updates on OCI registries (#251)

### Fix

- **actions**: wait for a workflow check, not any check (#258)
- **monitor**: gate promotion on newly introduced vulnerabilities (#253)

## v2.2.0 (2026-07-28)

### Feat

- **presets**: hold new image versions for 14 days unless they fix a CVE (#246)
- **presets**: pin catalogue images by digest (#244)

## v2.1.5 (2026-07-27)

### Fix

- **cli**: point every hint at the current command paths (#234)
- **drift**: compare each pipeline against its own workflow (#232)
- **release**: watch the run the pushed tag actually triggered (#231)
- run custom containers by name and harden generated workflows (#228)
- **release**: share conventional-commit parsing and treat required_version as a floor (#226)
- **presets**: emit boolean option flags without a value and surface pull_policy/timeout (#225)
- stop silently dropping untracked files and broken preset files (#224)

## v2.1.4 (2026-07-27)

### Fix

- **release**: skip the editor when prepare runs without a TTY (#221)
- **release**: reconcile version from the latest tag across the release flow (#220)
- **release**: route the version bump through a pull request (#219)

## v2.1.3 (2026-07-27)

### Fix

- **executor**: scope container names per project and reconcile name conflicts (#215)
- **presets**: inject option flags inside shell-wrapped commands (#213)
- **presets**: decode pull_policy and timeout in custom presets (#209)
- **version**: resolve version from build info for go-installed binaries (#208)

## v2.1.2 (2026-07-27)

### Fix

- **presets**: document probatum musl-only execution constraint (#201)
- **presets**: extract cargo-audit to a writable dir for non-root containers (#199)
- **gomod**: move module to /v2 path so v2 releases are go-installable (#198)

## v2.1.1 (2026-07-27)

### Fix

- **doctor**: warn instead of pass when only Podman is available (#191)

## v2.1.0 (2026-07-18)

### Feat

- **actions**: add pr edit to update PR title/body (#176)
- add probatum preset (test phase) (#160)

### Fix

- **drift**: resolve workflow file instead of hardcoding names (#177)
- **actions**: derive branch prefix from commit type, fix next-steps hints (#173)
- **actions**: cpw waits for CI workflow to start (#172)
- **generate**: pin bootstrapped cidx to generating version (#166)
- **executor**: fall back to anonymous pull when registry rejects credentials (#165)
- **presets**: use prebuilt cargo-audit binary (#164)

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
