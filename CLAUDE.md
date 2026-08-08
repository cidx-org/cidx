# CLAUDE.md

## What is CIDX

**CIDX** (CI with Declarative eXecution) is a personal Go CLI tool that serves as both a practical CI/CD runner and a showcase of modern DevOps practices.

It does two things:

1. **Portable CI/CD Runner** -- A single `cidx.toml` config runs identically on local, GitHub Actions, GitLab CI, and Jenkins. Declare container names, CIDX resolves images, volumes, commands, and environment from built-in presets.
2. **Developer Workflow** -- Human-friendly commands for PR lifecycle, branch management, releases, and CI monitoring (`cidx repo pr`, `cidx release`, `cidx status`).

**This is not a product competing with Dagger or Earthly.** It's an opinionated tool built for its author's workflow, open-sourced as a reference implementation of a DevOps philosophy: convention over configuration, container-native execution, security by default.

## Project Philosophy

### Core Beliefs

- **Convention over Configuration** -- Presets eliminate boilerplate. Overrides are exceptional.
- **Container-native** -- Everything runs in containers. Nothing on the host. Clean, reproducible.
- **Security by Default** -- Docker Hardened Images, safe local modes (no-push, draft).
- **KISS** -- Single config file. Container names are the only required input.
- **Explicit over Magic** -- Dry-run mode, transparent merge logic, clear preset definitions.

### This Project is a Showcase

CIDX exists to demonstrate what a well-run project looks like in practice -- not the cargo-culted version with 47 badges and empty abstractions, but the real version:

- **BDD-first development** -- If you can't write a Gherkin scenario for it, don't build it
- **Trunk-based workflow** -- Short-lived branches, conventional commits, grouped releases
- **AI-assisted development** -- A human pilots, Claude executes. Every feature is discussed before a single line is written
- **Aggressive dogfooding** -- CIDX builds itself with its own pipeline AND we use every CIDX command in our daily workflow. If a command is missing, broken, or has bad UX, that becomes the next priority. We eat our own cooking -- `cidx repo branch pr`, `cidx cpw`, `cidx doctor`, `cidx check drift` are all used for real, not just tested
- **Minimal dependencies** -- Every import must justify its existence

## How We Work

### Product Guardrails

Before any feature, check against [docs/GUARDRAILS.md](docs/GUARDRAILS.md) and [docs/PRODUCT_SCOPE.md](docs/PRODUCT_SCOPE.md). Key rules:

1. **CIDX adapts to the project** -- never the other way around
2. **No phase without real value** -- no theoretical completeness, no "looks mature"
3. **Simplicity is a constraint** -- does it remove friction or add sophistication?
4. **User stays in control** -- understandable, predictable, observable, overridable
5. **Right level of abstraction** -- execution engine, not governance framework

If a feature increases CIDX's control over projects, the default answer is **no**.

### Scope: What CIDX Does and Doesn't Do

CIDX is an **execution engine** for CI phases (security, code, test, build). It runs containers, manages presets, and provides workflow helpers.

CIDX **does not** manage:

- **Release publishing** -- GitHub/GitLab release workflows handle this natively with `softprops/action-gh-release` or equivalent. Cross-compile + asset attachment is 5 lines of shell, executed once per release. Adding a CIDX preset for this would add complexity for near-zero friction reduction (guardrail 3).
- **Deployment** -- CIDX stops at build. Deployment is platform-specific and out of scope.
- **Team governance** -- No mandatory phases, no enforced workflows, no compliance frameworks (guardrail 5).

If a capability is better handled by the platform natively, CIDX should not duplicate it.

**What that costs, and where it is paid**: this repository's own `release.yml` is therefore a _publication_ workflow, not an implementation of `[pipelines.release]`. It cross-compiles natively, delegates the `docker` phase to `cidx run docker`, and publishes with `softprops/action-gh-release`; it does not re-run security/code/test, which `ci.yml` already ran on the commit the tag is cut from. `[pipelines.release]` is the end-to-end rehearsal `cidx run release` walks locally with the guardrails on. The two are not meant to coincide, so `cidx.toml` declares `workflow = "none"` on that pipeline and `cidx check workflow` leaves it alone (issue #233).

The declaration is written down rather than inferred because no inference can be sound here: from the outside, a job doing its phase natively and a job that lost its `cidx run` call look identical, so any rule lenient enough to excuse the first would also excuse the second — and the second is exactly the drift the check exists to catch.

### The Golden Rule: Discussion Before Code

Every feature follows this cycle:

1. **Discuss** -- The feature is talked through conversationally. What problem does it solve? What are the trade-offs? What's the simplest approach?
2. **Specify** -- Write BDD scenarios (Gherkin) that capture the expected behavior. This is the contract.
3. **Implement** -- Write the code to make scenarios pass. Nothing more.
4. **Validate** -- Run the full test suite. If it passes, it ships.

No code gets written before step 1 is complete. No implementation starts before step 2 has scenarios.

### Decision Tracking: GitHub Issues + BDD Scenarios

Instead of ADRs (too ceremonial for a solo project), decisions are tracked through:

- **GitHub Issues** -- Feature discussions, design decisions, trade-off analysis. The issue thread IS the decision record. Use labels to categorize (`design`, `decision`, `question`).
- **BDD Scenarios** -- The executable specification. Reading `features/` tells you what the system does and why.
- **Conventional Commits** -- The commit history tells the story of what changed and when.

The combination of issue discussion + scenario specification + commit history gives full traceability without the overhead.

### Trunk-Based Development

- `main` is the only long-lived branch
- Feature branches are short-lived (hours to days, not weeks)
- All changes go through PRs with CI validation
- Releases are grouped (3-5 PRs per release) and manually triggered
- Tags = Releases (1:1 mapping)
- **Changelog**: Must be updated at every release. Commitizen generates it from conventional commits. Verify CHANGELOG.md is current before tagging.
- **Owed to v3.0.0**: nothing. The hidden `cidx action ...` tree — the one thing the next major owed — is deleted (issue #235), so the release that ships that deletion is the one that has to be cut as `v3.0.0`; a minor bump would ship a removed command under a compatible version number. `TestTheDeprecatedActionTreeIsGone` keeps it from coming back, and the correspondence table survives in `docs/reference/cli.md` for anyone with the old spelling in a script.

Use `cidx pr create`, `cidx pr merge`, `cidx release create` for the workflow.

### Dogfooding: Use CIDX for Everything

**Never use `gh` CLI or raw `git` commands for PR/branch workflows.** Always use `go run ./cmd/cidx` (or the built binary). This is how we find bugs and UX issues.

```bash
# Core commands
go run ./cmd/cidx init                                # detect project, generate config
go run ./cmd/cidx run --dry-run ci                    # preview pipeline
go run ./cmd/cidx run ci                              # execute full pipeline
go run ./cmd/cidx generate github                     # generate CI workflow
go run ./cmd/cidx doctor                              # environment check
go run ./cmd/cidx check drift                         # compare cidx.toml vs CI YAML

# PR lifecycle (hidden top-level aliases for convenience)
go run ./cmd/cidx pr create "feat: description"
go run ./cmd/cidx cpw -m "commit message"             # commit + push + watch CI
go run ./cmd/cidx pr watch -q                         # watch PR checks (quiet)
go run ./cmd/cidx pr status                           # show PR status
go run ./cmd/cidx pr merge                            # merge current PR

# Full paths (also valid)
go run ./cmd/cidx repo pr create "feat: description"
go run ./cmd/cidx release create
go run ./cmd/cidx security vuln list
```

If a command is missing, broken, or has bad UX -- **that becomes the next priority**. We eat our own cooking.

### TDD/BDD Strategy

**BDD (Gherkin + godog)** -- System-level behavior:

- Feature files in `features/` organized by domain (events, security, pipelines, presets, executor)
- Step definitions in `features/*_steps_test.go`, beside the `.feature` files they implement
- Simulation engine (no real Docker needed to run specs)
- `Strict: true` for unit scenarios, `Strict: false` only for `@docker-required` scenarios
- `TestFeatures` (strict, 349 scenarios) and `TestFeaturesDocker` (best-effort, 19 scenarios, skipped when no container runtime answers)
- The suites live in **`features/`**, the package that also holds the `.feature` files: a scenario and the step that implements it are one `ls` apart. They used to sit in a root `package main` with no source file in it, which the `go-test` preset then skipped — along with `internal/commands`, where every CLI test has lived since #317. `cidx.toml` carried an override to put them back until #357 moved the fix into the catalogue: the preset says `go test -v ./...` now and this repository states no command at all. Two guards keep it that way — `TestNoTestPresetRunsOnlyPartOfTheProject` (in `pkg/presets`) fails on a catalogue test preset that names a subtree, and `TestTheTestPhaseRunsEveryPackageThatHasTests` (in `pkg/config`) fails when the resolved command stops covering a package that holds tests (#344)
- Scenarios that describe the CLI import `internal/commands` and resolve against `commands.NewApp()` — the real tree, never a copy of it (#317)

**Unit tests** -- Package-level correctness:

- Standard Go `*_test.go` files in each package
- Focus on edge cases and error paths that BDD doesn't cover

**Test playground**: `cidx-org/cidx-test-playground` on GitHub -- used for integration tests that need a real remote repo (PR creation, artifact management, workflow watching). Referenced in `features/features_test.go`.

**The hierarchy**: BDD scenarios define WHAT the system does. Unit tests verify HOW individual pieces work. BDD comes first.

## Architecture

### Package Structure

```
pkg/
  actions/       Git workflow commands (PR, release, tag, cpw)
  branch/        Branch listing, formatting, git operations
  config/        TOML parsing, validation, types
  environment/   CI provider detection, local safety modes
  executor/      Docker/Podman abstraction layer
  pipeline/      Phase-based orchestration (sequential/parallel)
  presets/       Built-in container configurations (40+ presets)
  registry/      Container registry operations
  remote/        Git remote provider abstraction (GitHub, GitLab)
  validator/     CI workflow validation
  vcs/           Version control operations

internal/
  commands/      CLI command tree and handlers (urfave/cli)
    app.go       Command hierarchy: core top-level, secondary under namespaces
    repo.go      cidx repo — PR, cpw, branch, workflow, artifact, cleanup
    release_cmd.go cidx release — prepare, preview, create, commit, tag
    security_cmd.go cidx security — vuln, registry
  guards/        Repository-wide invariants, tests only (locale, hints, phases)
  tui/           Shared terminal styles

cmd/cidx/        main() and the `Version` ldflags symbol — nothing else
features/        BDD scenarios (Gherkin) and the steps that run them
docs/            Project documentation
```

The module root holds no Go file. Everything is a package under `cmd/`, `pkg/`,
`internal/` or `features/` — the root used to be a `package main` with no source
in it and twenty-seven test files, which is neither a package anyone imports nor
a place anyone looks.

`cmd/cidx` is deliberately a three-line `package main`. The tree used to live
there, in a second `package main` that nothing could import, so the godog suite
kept a hand-written copy of it — and the copy drifted (issue #317). Everything
now lives in `internal/commands`, which the suite imports: scenarios resolve
against `commands.NewApp()`, the very tree the binary runs. `Version` stays in
`package main` because `-ldflags "-X main.Version=..."` is what the Makefile,
`release.yml`, `nightly.yml` and the `go-build` preset all target.

### Data Flow

1. `config.Load()` -- Parse TOML, expand `${ENV_VARS}`
2. `environment.Detect()` -- Identify CI provider, event type, safety mode
3. `presets.Get(name)` + `preset.MergeWith(overrides)` -- Resolve container config
4. `executor.Select()` -- Choose Docker or Podman
5. `pipeline.RunPhase()` / `pipeline.RunPipeline()` -- Execute

### Key Abstractions

- **Preset** -- Complete container definition (image, command, workdir, volumes, env, options)
- **ContainerConfig** -- Runtime-resolved config after merging preset + user overrides
- **Pipeline** -- Named sequence of phases, mapped to events by convention
- **Executor** -- Interface abstracting Docker/Podman (`Run()`, `Available()`, `Name()`, `Close()`)
- **Environment** -- Detected CI context (provider, event, branch, tag, PR state, safety mode)

## Development Commands

```bash
# Build
go build -o bin/cidx ./cmd/cidx

# Test
go test ./...                         # All tests (unit + BDD)
go test -v -run TestFeatures          # BDD scenarios only
go test -cover ./...                  # With coverage

# Quality
go fmt ./...
go vet ./...
golangci-lint run

# Run locally
go run ./cmd/cidx preset list         # List presets
go run ./cmd/cidx run --dry-run ci    # Dry-run pipeline

# Workflow (use cidx itself)
cidx pr create "feat: description"
cidx cpw -m "commit message"
cidx pr merge
cidx release create
cidx run ci                           # Full local CI
```

## Adding New Presets

```go
// In pkg/presets/registry.go
"toolname": {
    Name:    "toolname",
    Phase:   "security",  // security, code, test, build, docker, release
    Image:   "org/image:tag@sha256:...",
    Command: "tool scan .",
    Workdir: "/scan",
    Volumes: []string{"${WORKSPACE}:/scan"},
}
```

Rules: use official images (prefer DHI), defaults must work without overrides, test with `cidx run toolname --dry-run`.

Catalogue images are **pinned by digest** (`image:tag@sha256:...`) — rule 1 of the [supply-chain policy](docs/core-concepts/supply-chain-policy.md#the-three-rules), issue #242. Resolve the multi-arch index digest with `docker buildx imagetools inspect --format '{{.Manifest.Digest}}' <image:tag>`; `TestCatalogueImagesArePinnedByDigest` fails on a preset added without one.

## Configuration

```toml
# Minimal -- this is all you need
[security]
containers = ["trivy", "gitleaks"]

[pipelines.ci]
phases = ["security", "code"]

# Override only when defaults don't fit
[containers.trivy]
severity = "HIGH,CRITICAL"
```

## Dependencies

Core: `BurntSushi/toml`, `docker/docker`, `urfave/cli/v2`, `charmbracelet/bubbletea`, `sirupsen/logrus`
Test only: `cucumber/godog`

Every dependency must justify its presence. No utility libraries, no "just in case" imports.

## Code Style

- **Presets**: lowercase, hyphen-separated (`"ansible-lint"`)
- **Go types**: PascalCase (`Preset`, `ContainerConfig`)
- **Errors**: Always wrap with context: `fmt.Errorf("context: %w", err)`
- **Logging**: logrus. User errors vs system errors are distinct.
- **Running git**: always `vcs.Git(dir, args...)`, never `exec.Command("git", ...)`. CIDX decides what a git failure _was_ by matching git's own sentences — a worktree already holds the branch, the remote ref was already gone, the branch has no upstream — because git has no exit code that says which. Those sentences are translated, so the helper pins `LC_ALL=C` and makes git's output an interface CIDX controls rather than a setting the user happens to have (#364). `TestEveryGitInvocationPinsTheLocale` fails on a raw git command.
- **No dead code**: If it's not used, delete it. No `// TODO` for core functionality.
