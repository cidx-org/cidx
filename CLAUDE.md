# CLAUDE.md

## What is CIDX

**CIDX** (CI with Declarative eXecution) is a personal Go CLI tool that serves as both a practical CI/CD runner and a showcase of modern DevOps practices.

It does two things:

1. **Portable CI/CD Runner** -- A single `cidx.toml` config runs identically on local, GitHub Actions, GitLab CI, and Jenkins. Declare container names, CIDX resolves images, volumes, commands, and environment from built-in presets.
2. **Developer Workflow** -- Human-friendly commands for PR lifecycle, branch management, releases, and CI monitoring (`cidx action`, `cidx branch`, `cidx status`).

**This is not a product competing with Dagger or Earthly.** It's an opinionated tool built for its author's workflow, open-sourced as a reference implementation of a DevOps philosophy: convention over configuration, container-native execution, security by default.

## Project Philosophy

### Core Beliefs

- **Convention over Configuration** -- Presets eliminate boilerplate. Overrides are exceptional.
- **Container-native** -- Everything runs in containers. Nothing on the host. Clean, reproducible.
- **Security by Default** -- Docker Hardened Images, safe local modes (no-push, draft).
- **KISS** -- Single config file. Container names are the only required input.
- **Explicit over Magic** -- Dry-run mode, transparent merge logic, clear preset definitions.

### This Project is a Showcase

CIDX exists to demonstrate what a well-run project looks like in practice -- not the bullshit version with 47 badges and empty abstractions, but the real version:

- **BDD-first development** -- If you can't write a Gherkin scenario for it, don't build it
- **Trunk-based workflow** -- Short-lived branches, conventional commits, grouped releases
- **AI-assisted development** -- A human pilots, Claude executes. Every feature is discussed before a single line is written
- **Aggressive dogfooding** -- CIDX builds itself with its own pipeline AND we use every CIDX command in our daily workflow. If a command is missing, broken, or has bad UX, that becomes the next priority. We eat our own cooking -- `cidx branch pr`, `cidx action cpw`, `cidx doctor`, `cidx check drift` are all used for real, not just tested
- **Minimal dependencies** -- Every import must justify its existence

## How We Work

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

Use `cidx action pr create`, `cidx action pr merge`, `cidx action release create` for the workflow.

### Dogfooding: Use CIDX for Everything

**Never use `gh` CLI or raw `git` commands for PR/branch workflows.** Always use `go run ./cmd/cidx` (or the built binary). This is how we find bugs and UX issues.

```bash
# PR lifecycle (top-level shortcuts)
go run ./cmd/cidx pr create "feat: description"
go run ./cmd/cidx cpw -m "commit message"             # commit + push + watch CI
go run ./cmd/cidx pr watch -q                         # watch PR checks (quiet)
go run ./cmd/cidx pr status                           # show PR status
go run ./cmd/cidx pr merge                            # merge current PR

# Diagnostics
go run ./cmd/cidx doctor                              # environment check
go run ./cmd/cidx check drift                         # compare cidx.toml vs CI YAML
go run ./cmd/cidx generate github                     # generate CI workflow
```

If a command is missing, broken, or has bad UX -- **that becomes the next priority**. We eat our own cooking.

### TDD/BDD Strategy

**BDD (Gherkin + godog)** -- System-level behavior:

- Feature files in `features/` organized by domain (events, security, pipelines, presets, executor)
- Step definitions in `*_steps_test.go` at project root
- Simulation engine (no real Docker needed to run specs)
- `Strict: true` for unit scenarios, `Strict: false` only for `@docker-required` scenarios
- `TestFeatures` (strict, 57+ scenarios) and `TestFeaturesDocker` (best-effort, 5 scenarios)

**Unit tests** -- Package-level correctness:

- Standard Go `*_test.go` files in each package
- Focus on edge cases and error paths that BDD doesn't cover

**Test playground**: `cidx-org/cidx-test-playground` on GitHub -- used for integration tests that need a real remote repo (PR creation, artifact management, workflow watching). Referenced in `features_test.go`.

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

cmd/cidx/        CLI entry point and command handlers (urfave/cli)
features/        BDD scenarios (Gherkin)
docs/            Project documentation
```

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
go run ./cmd/cidx list                # List containers
go run ./cmd/cidx run --dry-run ci    # Dry-run pipeline

# Workflow (use cidx itself)
cidx action pr create "feat: description"
cidx action cpw -m "commit message"
cidx action pr merge
cidx action release create
cidx run ci                           # Full local CI
```

## Adding New Presets

```go
// In pkg/presets/registry.go
"toolname": {
    Name:    "toolname",
    Phase:   "security",  // security, code, test, build, docker, release
    Image:   "org/image:tag",
    Command: "tool scan .",
    Workdir: "/scan",
    Volumes: []string{"${WORKSPACE}:/scan"},
}
```

Rules: use official images (prefer DHI), defaults must work without overrides, test with `cidx run toolname --dry-run`.

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
- **No dead code**: If it's not used, delete it. No `// TODO` for core functionality.
