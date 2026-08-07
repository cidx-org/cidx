# CIDX BDD Scenarios

This directory contains **executable specifications** of CIDX behavior using Gherkin scenarios.

## Purpose

These scenarios serve three critical purposes:

1. **Living Documentation** - Human-readable specs that explain how CIDX works
2. **Automated Tests** - Executable tests using [godog](https://github.com/cucumber/godog)
3. **Scope Guardian** - If it's not specified here, we don't build it

## Structure

```
features/
├── events/              # Event-driven behavior
│   ├── pull_request.feature
│   ├── tag_release.feature
│   └── merge_to_main.feature
├── security/            # Safety and environment
│   ├── local_safety.feature
│   └── environment_detection.feature
├── pipelines/           # Pipeline execution
│   └── pipeline_execution.feature
└── README.md           # This file
```

## Event-Driven Scenarios

### Pull Requests (`events/pull_request.feature`)

Specifies how CIDX behaves when a pull request is created:

- **Phases**: security, code, test only (no build/release/docker)
- **Purpose**: Fast validation and feedback
- **Duration**: ~5-10 minutes
- **Fail Fast**: Stops on first failure

**Key Scenarios**:

- PR triggers validation phases only
- PR fails fast on security issues
- PR works in local and CI environments

### Tag Releases (`events/tag_release.feature`)

Specifies how CIDX behaves when a version tag is pushed:

- **Phases**: All phases (security → code → test → build → release → docker)
- **Purpose**: Complete deployment to production
- **Duration**: ~15-30 minutes
- **Safety**: Local = draft/dry-run, CI = publish/push

**Key Scenarios**:

- Tag triggers full pipeline in CI
- Docker pushes in CI, builds without push locally
- Release publishes in CI, creates draft locally

### Merge to Main (`events/merge_to_main.feature`)

Specifies how CIDX behaves when code is merged to main branch:

- **Phases**: security, code, test, build (no release/docker)
- **Purpose**: Build production-ready artifacts
- **Duration**: ~10-15 minutes
- **Output**: Artifacts ready for release

**Key Scenarios**:

- Main builds artifacts but doesn't deploy
- Main ensures production-ready state
- Artifacts can be released immediately with a tag

## Security Scenarios

### Local Safety (`security/local_safety.feature`)

Specifies how dangerous operations are protected in local environment:

**Docker Operations**:

- `local_behavior = "dry-run"` → Prints what CI would run, builds nothing
- CI environment → Builds and pushes normally

**Release Operations**:

- `local_behavior = "draft"` → Creates draft releases
- CI environment → Publishes releases normally

**Key Scenarios**:

- docker-buildx safe in local, pushes in CI
- gh-release creates draft locally, publishes in CI
- Preset can require CI environment

### Environment Detection (`security/environment_detection.feature`)

Specifies how CIDX detects where it's running:

**CI Providers Detected**:

- GitHub Actions (`GITHUB_ACTIONS=true`)
- GitLab CI (`GITLAB_CI=true`)
- Jenkins (`JENKINS_URL` set)
- CircleCI (`CIRCLECI=true`)

**Git Context Detected**:

- Pull requests (`IsPR`)
- Tag pushes (`IsTag`, `TagName`)
- Branch names (`BranchName`)

## Pipeline Scenarios

### Pipeline Execution (`pipelines/pipeline_execution.feature`)

Specifies how named pipelines execute their phases:

**Pipeline Types**:

- `pr` → security, code, test
- `main` → security, code, test, build
- `release` → security, code, test, build, release, docker

**Key Behaviors**:

- Phases execute in defined order
- Pipeline stops on first failure
- Named pipelines provide clear intent

## Running Scenarios

The suites live in this directory, together with the `*_steps_test.go` step
definitions they need — a scenario and the code implementing it are one `ls`
apart. Always name a package, never a single file:
`go test ./features/features_test.go` fails to build with
`undefined: RegisterCommonSteps`.

### Run All Scenarios

```bash
go test -run TestFeatures .
```

`-run` matches as an unanchored regexp, so this runs both `TestFeatures` (strict)
and `TestFeaturesDocker` (best-effort). Use `-run 'TestFeatures$'` for the strict
suite alone.

### Run with Pretty Output

```bash
GODOG_FORMAT=pretty go test -v -run TestFeatures .
```

`features_test.go` reads `GODOG_FORMAT` and passes it to `godog.Options.Format`
(`pretty`, `progress`, `junit`, `cucumber`). `progress` is what CI uses.

### Run Specific Feature or Scenario

Scenarios are reported as Go subtests, so the standard `-run` filter selects
them by name (spaces become underscores):

```bash
go test -v -run 'TestFeatures$/Drift_detected' .
```

### Tags

`@docker-required` marks the scenarios that need control over the container
runtime. `TestFeatures` excludes them and runs strict; `TestFeaturesDocker` runs
only them, best-effort, and skips outright when no runtime is reachable.
Filtering is set in `features_test.go` via `godog.Options.Tags` -- the suites do
not call `godog.BindCommandLineFlags`, so `-godog.tags` is not available.

## Dogfooding

CIDX tests itself using CIDX:

```bash
# Run every package's tests, BDD scenarios included
cidx run test
```

The test phase runs the `go-test` preset, whose command is `go test -v ./...`.
It reaches every package, this one included. It used to run
`./pkg/... ./cmd/...` and reach neither these suites — which then sat in the
repository root — nor `internal/commands`, so `cidx.toml` repaired it with an
override; #357 fixed the catalogue instead, on the grounds that a default every
project has to repair is not a default.

## Writing New Scenarios

### Scenario Template

```gherkin
Feature: Feature Name
  As a [role]
  I want [feature]
  So that [benefit]

  Background:
    Given [common setup]

  Rule: [business rule]

    Scenario: [specific behavior]
      Given [precondition]
      When [action]
      Then [expected outcome]
      And [additional assertion]
      But [negative assertion]
```

### Step Patterns

**Common Steps** (available everywhere):

- `Given I am in (local|CI) environment`
- `Given the environment variable "X" is set to "Y"`
- `When I run "cidx run pr"`
- `Then I should see "message"`
- `Then the exit code should be 0`

**Event Steps** (for event scenarios):

- `Given I create a pull request`
- `Given I push a tag "v1.0.0"`
- `Then it should execute the "security" phase`
- `Then it should NOT execute the "release" phase`

**Security Steps** (for safety scenarios):

- `Given the "docker-buildx" preset has local_behavior = "dry-run"`
- `Then Docker image should NOT be pushed to registry`

## Philosophy

These scenarios embody CIDX's philosophy:

1. **Event-Driven** - Different events trigger different phases
2. **Convention over Configuration** - CIDX knows what to do based on context
3. **Safe by Default** - Dangerous operations protected locally
4. **Clear Scope** - Only behaviors in scenarios get implemented

See [docs/philosophy.md](../docs/philosophy.md) for the complete philosophy.

## Contributing

When adding features to CIDX:

1. **Write scenario first** - Specify the behavior in Gherkin
2. **Implement step definitions** - Make scenario executable
3. **Implement feature** - Build the actual functionality
4. **Verify** - Run scenario to confirm it passes

If you can't write a clear Gherkin scenario for it, maybe we shouldn't build it!
