## [Unreleased]

### Refactor

- **cli**: move the command tree out of `cmd/cidx` into `internal/commands`, leaving `main()` and the `main.Version` ldflags symbol behind. **No user-visible change**: `go build`, `go install` and `go run ./cmd/cidx` are untouched, and the shipped tree is byte-identical before and after — same 133 command paths, same flags, aliases, usage and help text, checked by diffing a full structural walk plus `--help` on every path from both binaries. The godog suite lives in the repository root's `package main`, `newApp()` lived in a second `package main` in `cmd/cidx`, and two `main` packages cannot import each other — so every scenario that needed the tree worked against `invocationCommandTree()`, a copy maintained by hand. #316 had to extend it with `run`'s flags, and a flag added tomorrow would not reach it: the scenarios would stay green describing a CLI that no longer exists, which is the failure #265 was about. The copy is deleted and the scenarios resolve against `commands.NewApp()`, the tree the binary runs. It also unblocks specifying CLI wiring that could not be described at all: the flag-placement guard of #268, which shipped with Go tests only for this reason, now has `features/cli/flag_placement.feature` (#317)

- **presets**: drop the `default` key from the preset option schema — a **schema change**, visible in `preset info`, `preset show` and `preset export`, which stop printing a `Default:` line. The catalogue declared 45 option defaults and applied none of them: `MergeWith` only visits the keys a user's `[containers.X]` section actually sets, so a declared default never reached a container. Applying the 19 that would have changed something was the worse repair — `trivy` declared `--exit-code 0`, which never fails a scan, and `bandit` declared values for `-ll` and `-i`, which are counters and reject one, so at least one of fifteen presets would have shipped broken on the strength of documentation alone. Every preset in the catalogue resolves to the same image, command, workdir, volumes and environment as before, which is the point: the field was inert. A preset that genuinely needs a value puts it in its `command` or `env`, where every run exercises it; a residual `default` in a user or project `presets.toml` is now named as an unknown key (#203) instead of being accepted and ignored (#299)

### Feat

- **repo**: `cidx repo artifact download` fetches a run's artifacts, so the command that reads them is no longer the one command needing another tool. `cidx security vuln prune --results DIR` and `cidx security baseline --results DIR` consume exactly the scanner results the audit uploads, and the only way to put them on disk was `gh run download` — a cidx command depending on artifacts with no cidx way to obtain them. `--output` defaults to `scan-results`, which is what `--results` defaults to, so the pair works with no path spelled twice. Three things the shell-out got wrong are handled: the repository comes from the git remote of the working directory and is printed, where `gh run download <id>` resolves it from gh's own notion of where you are and hands over another repository's artifacts without a word (#327); the files land in **one flat directory**, where gh unpacks a subdirectory per artifact — on the same audit run, `vuln prune` reports 21 of 21 catalogue repositories covered from this command's output and 0 of 21 from gh's (#333); and a file name two artifacts share is not fatal, identical content being skipped and differing content keeping the first copy, because aborting halfway through a 42-artifact download leaves a directory that reads as a complete scan and is not one. Archive entries are written under their base name, so nothing an artifact names can be written outside the destination. On GitLab an artifact is a job that uploaded an archive and `--run` takes a pipeline ID (#285)
- **repo**: `cidx repo workflow rerun` restarts a run, or with `--failed` only the jobs that failed — the recovery path when a job dies pulling an image (`read: connection reset by peer`) rather than on the change, which until now meant `gh run rerun --failed` because `cidx repo workflow run` only dispatches and `ci.yml` declares no `workflow_dispatch`. It reads the run first, so a run number handed over in place of a run ID says which column to read (#291) and `--failed` on a run with no failed job is refused with the command that does work, instead of GitHub's bare 403. It does not watch: a rerun starts a new attempt and the API reports the previous one as completed for a few seconds, so a chained watch would report the failure it was asked to clear. `cidx repo workflow list` now also takes no argument, meaning every workflow on the current branch — the question you have when a check has just failed and you only know the job's name — and its runs come from the provider rather than from `gh api`, so the listing speaks the same client and the same identifier as the commands beside it, and works on GitLab, where `--failed` maps to a pipeline retry and a full restart is refused as having no counterpart (#342)
- **cli**: warn on every `cidx action ...` invocation, naming the exact command that replaces it and the release it disappears in (v3.0.0); the tree keeps working until then (#235)
- **audit**: publish a vulnerability status summary to a tracking issue (#308)
- **presets**: flag images whose base is approaching end of life (#305)
- **audit**: publish the catalogue's scan results to GitHub code scanning (#301)

### Fix

- **check**: have `check workflow` say what it checked. One summary line served both paths, so `cidx check workflow ci` — which compares exactly one pipeline — signed off with `✅ All workflows are in sync with pipelines` and read as a clean bill for the repository; it cost a wrong conclusion on a repository whose `release.yml` was out of sync at the time. A pipeline named on the command line is now named in the summary, the sweep states how many workflows it covered, and both wordings are pinned by a test (#318)
- **security**: refuse to render a report from an exceptions file that could not be read. `cidx security sarif` resolved `known-vulnerabilities.toml` relative to the working directory and treated its absence as "nothing accepted", so run from anywhere but the repository root it wrote a successful SARIF with **zero** expired-exception alerts and said nothing — 18 alerts from the root, 0 from one directory up, both exit 0. The same swallow had been copied into `security baseline`, `security summary` and `preset audit`, which published a catalogue that had accepted nothing on the same terms; all four now fail on the read, before any registry or scanner is contacted. An absent record and an empty one are not the same claim and only one of them is safe to publish. `vuln add` and `vuln ignore` keep tolerating the absence on purpose: the first creates the file, the second waives nothing without it and therefore errs towards a scan reporting too much (#304)
- **workflow**: print the run identifier `workflow watch` accepts. `cidx repo workflow list` showed `run #640` — the number GitHub displays in its UI — and handing it to `cidx repo workflow watch 640` sent it to the API as a run ID and got a flat `404 Not Found`, so the two commands of one namespace disagreed about which identifier they spoke and the only way through was to poll the list instead of watching. Both numbers are now listed, labelled, in the simple and the verbose view, and the closing hint names `cidx repo workflow watch <id>` instead of `gh run view <run-number>`, which was the wrong tool and the wrong number (#291)
- **presets**: pass arguments only to the images whose entrypoint is already the tool. `commitlint` named itself in its command, so the container ran `commitlint commitlint --from ...` and answered `Unknown argument: commitlint`: the preset could never run, on any pin, and nothing noticed because this repository's `cidx.toml` lists it in no phase. The sweep that finding asked for ran every catalogue preset no phase here dogfoods — 28 of the 31, the other three (`twine`, `cargo-publish`, `ansible-galaxy-publish`) refusing to run outside CI by design — and turned up two more of the same shape, both hidden behind a local-safety dry-run that never hands the command to a container: `goreleaser`, whose entrypoint script ends in `goreleaser "$@"` and answered `unknown command "goreleaser" for "goreleaser release"`, and `docker-buildx`, whose `sh -c` wrapper died on `docker: unknown command: docker sh` and could not have been rescued by clearing the entrypoint either, that DHI variant shipping no shell to exec. `commitlint` also gains `--default-config`, so a repository with no commitlint config is linted against conventional commits instead of exiting 9 on a missing rule set. Scenarios in `features/presets/workspace_hygiene.feature` now fail any of the three that grows its own name back (#278)
- **presets**: stop `ansible-lint` leaving a `.ansible/` in the workspace that the host user may not be able to delete. `ansible_compat` builds the cache under the project directory unconditionally and consults `ANSIBLE_HOME` only in `--offline` mode, which would also stop the linter installing the collections a `requirements.yml` asks for — so the preset clears the directory after the run instead of pointing it elsewhere, with `rmdir` rather than `rm -rf`: a cache that actually holds something, or one the user already had, refuses to be removed and survives untouched. The lint verdict is captured before the cleanup, so it cannot be masked. `yamllint`, `ansible-syntax`, `ansible-galaxy-build`, `ansible-test` and `molecule` were checked on the same image and write nothing. The same sweep found two more caches landing in the mount: `ruff` now lints with `--no-cache`, its image being distroless with a root-owned `/tmp` that a non-root run cannot redirect `--cache-dir` into, and `pytest` keeps both its cache and its bytecode out of the workspace (#279)
- **presets**: check `go.mod` with `go mod tidy -diff` instead of copying it. The preset used to `cp go.mod go.mod.bak`, run a real `go mod tidy`, diff against the copies and never remove them, leaving two untracked files no `.gitignore` covers in a workspace it was only asked to inspect — and, on an untidy module, a rewritten `go.mod` as well. `-diff` (Go 1.23+, and the image is 1.26) prints the same diff and exits non-zero while writing nothing at all (#295)
- **ci**: tell `peter-evans/create-pull-request` which paths to commit instead of letting it commit whatever is dirty. The container monitor downloads every scan artifact into its own workspace before deciding what to promote, so its workspace is dirty by construction: the promotion PR carried 43 files and 201,615 lines of scanner JSON next to its one-line image bump, and because gitleaks scans **every** ref, the Security job of every branch in the repository then failed on CVE text that pattern-matches an API token — unblocking CI meant closing the PR and deleting the branch. `.gitignore` (#324) closes that hole but not the next one, since the next artifact directory will not be listed on the day it is added; an allowlist is closed by default. The promotion commits `pkg/presets/presets.toml` and what `cz bump --changelog` rewrites, the Go version check commits `go.mod` and `go.sum`, and a test now fails any pull-request step that stops saying what it commits (#325)
- **cpw**: run the `code` phase before committing, so a formatting slip or a lint remark is caught locally in ~20 seconds instead of costing a full CI cycle plus a second commit. On by default with `--no-verify` to skip, the contract `git commit` already has with its hooks; the check runs before the commit, so a failure leaves the tree untouched. It is skipped, with a message and without blocking the push, when a pre-commit hook already runs the same phase, when no `[code]` phase is configured, or when no container runtime answers — `cidx doctor` says which (#307)
- **docker**: ship the cidx binary in the published image, and refuse to publish one that cannot run it. `.dockerignore` excluded `bin/`, the directory `COPY bin/cidx` reads, so every `ghcr.io/cidx-org/cidx` image up to v2.4.0 was a bare Docker CLI whose `ENTRYPOINT ["cidx"]` pointed at a file that was never copied. kaniko does not fail on that — it stats the context directory before applying `.dockerignore`, sees the file, then filters it out while copying, and exits 0 having copied nothing, where `docker build` fails outright on the same context. `bin/` stays ignored and the one consumed path is re-included, so a developer's leftovers still never reach the build context. The release binaries are now statically linked (`CGO_ENABLED=0`) — a native `go build` on the runner left linux/amd64 dynamically linked against the runner's glibc, which cannot exec on the image's Alpine base and puts a glibc floor on the binary users download. The base moves off `docker:27-cli`, built 2025-02-12 on an EOL Docker series, to a digest-pinned `docker:29-cli` from Docker Hub rather than the catalogue's `dhi.io` pin, which answers 401 without a subscription (#294) and would make the published image unpullable for third parties. The release workflow now runs the image it just pushed, which is the only check that sees either failure (#281)
- **branch**: scope `repo branch cleanup` to one branch — the one `--branch` names, or the current one — and refuse a branch with an open PR unless `--force`; the repository-wide sweep it used to do unconditionally now needs `--all`, a **behaviour change** for anyone running `cleanup -x` for it (#269)
- **validator**: compare a pipeline with the workflow it declares rather than the one its name suggests; `[pipelines.*] workflow` names that file, or `"none"` when no workflow implements the pipeline (#233)
- **validator**: read the phases of a workflow by parsing the cidx invocations instead of matching a substring, so a flag between the binary and `run` no longer hides a phase (#233)
- **security**: count the accepted findings the audit's ignore file removes from its own scan results, so `SECURITY-BASELINE.md` states what the catalogue carries rather than what is left after the acceptances are subtracted (465 against 447 on the same artifacts); a test now fails any change to the catalogue the committed file was not regenerated for, and the daily audit reports how far its numbers have drifted (#310)
- **security**: have the audit state what its ignore file suppressed instead of leaving the readers to infer it from an absence. `cidx security vuln ignore --results <dir>` writes the image, the repository, the number of entries it wrote and the number it left out as expired, next to the scan results; `vuln prune`, `security baseline` and `security summary` read it. An ignore file declared empty filtered nothing, so an absence in those results is an absence and the carried count is a total — the state every catalogue repository has been in since #303, and the one the guard added in #324 could not see, since an empty file records no suppression and reads exactly like a dropped `--show-suppressed`. Results carrying no declaration keep the conservative reading unchanged (#327)
- **security**: let an exception retire when its CVE is gone, by reading what the scanners recorded as suppressed alongside what their reports show; the audit passes `--show-suppressed` so Trivy keeps that record, and an absence with nothing recorded reads `unknown` rather than purgeable (#311)
- **security**: key code scanning alerts on the repository so a repin does not churn them (#313)
- **security**: report the exceptions on running repositories that are fixed upstream (#312)
- **presets**: hold Docker Hub and Quay.io to the same update rule as every other registry, so a candidate has to share the pinned tag's variant family — `buildpack-deps:trixie-curl`, a Debian 13 image, was being offered `buildpack-deps:26.10`, an Ubuntu **development branch**, past the cooldown and ready to promote. A candidate whose version reads as a calendar month that has not arrived is refused outright, and a pinned tag carrying no version at all (`trixie-curl`, `shellcheck:stable`) is now reported as such instead of counted among the images with nothing newer — those two are updated by rebuilds of the same tag, which the promotion path does not detect (#328)
- **security**: stop an expired acceptance from filtering anything — `cidx security vuln ignore` honours `expires` when it builds the scanners' ignore file, so a lapsed entry waives nothing and its finding is back in the audit's scan results. A **behaviour change**: the 18 entries that lapsed on 2026-03-02 stop suppressing today, no entry is renewed or removed on their account, and the code scanning alert for a lapsed acceptance supersedes the finding's own so one CVE on one repository stays one alert. The named day is included, and a missing or unreadable date waives nothing (#303)

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
