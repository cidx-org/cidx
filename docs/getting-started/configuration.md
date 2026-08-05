# Configuration

CIDX is configured via a `cidx.toml` file in the root of your project.

## Initialization

To create a default configuration:

```bash
cidx init
```

### 5. Minimum Version

To keep local development and CI on a cidx that knows the presets your config relies on, you can declare the oldest version that supports it.

```toml
# cidx.toml
required_version = "1.2.3"   # "v1.2.3" works too
```

This is a floor, not a pin: `1.2.3` and anything newer runs, an older cidx refuses to start with an upgrade hint. Dev builds (`go run ./cmd/cidx`) always bypass the check.

## Configuration Structure

The `cidx.toml` file's main purpose is to define **Pipelines**. These are sequences of execution phases that map to specific CI/CD events. CIDX reads this file, automatically detects the context (e.g., a Pull Request), and runs the pipeline that matches the event by convention.

### 1. Defining Pipelines

A pipeline is a sequence of phases, defined in a `[pipelines.<pipeline_name>]` section. The name of the pipeline should correspond to a CI/CD event (e.g., `pr`, `main`, `release`).

**Example: Event-Driven Pipelines**

```toml
# This pipeline will be automatically selected for Pull Requests.
[pipelines.pr]
phases = ["security", "code", "test"]
description = "Runs quick checks for all pull requests."

# This pipeline will be selected for pushes to the 'main' branch.
[pipelines.main]
phases = ["security", "code", "test", "build"]
description = "Builds a production-ready artifact from the main branch."

# This pipeline will be selected when a git tag is pushed.
[pipelines.release]
phases = ["security", "code", "test", "build", "release", "docker"]
description = "Publishes all artifacts for a new release."
```

**Optional: `workflow`** — names the CI workflow file that implements the pipeline. `cidx check workflow` compares the two. Unset, it follows the convention `<pipeline>.yml`; set it when the workflow is named otherwise, or to `"none"` when no workflow implements the pipeline at all.

```toml
[pipelines.release]
phases = ["security", "code", "test", "build", "docker", "release"]
# The release workflow publishes natively and only delegates one phase to cidx,
# so it is not this pipeline's implementation — nothing to compare.
workflow = "none"
```

### 2. Defining Phases and Containers

A `phase` is a logical group of containers. You define a phase by creating a top-level section with its name.

```toml
# Defines the 'security' phase and the containers it includes.
[security]
containers = ["trivy", "gitleaks"]

# Defines the 'build' phase.
[build]
containers = ["go-build"]
```

#### Overriding Container Settings

You can override any preset configuration for a container by creating a `[containers.<container_name>]` section. This override will apply wherever the container is used.

```toml
[containers.trivy]
severity = "HIGH,CRITICAL"
exit_code = 1
```

#### Setting a Severity Floor

Security presets ship **no severity or confidence floor**: each reports whatever its tool reports by default. That is deliberate — a floor is a policy about a codebase, and it belongs to that codebase rather than to the catalogue. It is also the safer direction to be wrong in: a default that reports too much is noisy and visible, while one that reports too little is invisible, because the phase goes green and nothing says what was skipped (issue #341).

Set one per project when a tool is too noisy for it:

```toml
[containers.gosec]
severity = "medium"    # low | medium | high
confidence = "medium"

[containers.bandit]
severity = "medium"    # all | low | medium | high
confidence = "medium"

[containers.trivy]
severity = "HIGH,CRITICAL"
```

`cargo-audit` is not a floor but a switch: it already fails on vulnerabilities, and `deny` additionally fails on warnings — unmaintained, unsound or yanked crates. It stays off by default, because an unmaintained crate frequently has no patched version to move to, so failing on one turns the phase red for something the project cannot fix.

```toml
[containers.cargo-audit]
deny = true
```

Tools with their own config file are honoured too, and are the better home for anything longer than a line: `bandit` reads `.bandit`, `bandit.yaml` or `pyproject.toml`, `gitleaks` reads `.gitleaks.toml`. Run `cidx preset info <name>` to see which options and config files a preset declares.

`env` overrides are merged **per key**: entries in `[containers.<name>].env` replace the preset's value for the same key, and preset keys you don't mention are preserved. For example, given a preset with `env = { RUSTUP_HOME = "/tmp/rustup", CARGO_HOME = "/tmp/cargo" }`:

```toml
[containers.clippy]
env = { RUSTUP_HOME = "/usr/local/rustup" }
# Effective env: RUSTUP_HOME=/usr/local/rustup, CARGO_HOME=/tmp/cargo
```

#### Defining Custom Tools

If you need a tool that does not have a built-in preset, define it in `presets.toml` rather than directly in `cidx.toml`.

Use:

- `~/.config/cidx/presets.toml` for user-level presets
- `.cidx/presets.toml` for project-level presets

`cidx.toml` should stay focused on phases, pipelines, and overrides.

### 3. Custom Presets (Advanced)

While `cidx.toml` configures _how_ tools are used (which phase, which pipeline), `presets.toml` allows you to configure _what_ the tools are (images, commands).

You can override built-in presets or define new ones by creating a `presets.toml` file in:

- `~/.config/cidx/presets.toml` (User-level, affects all projects)
- `.cidx/presets.toml` (Project-level, affects this project only)

> **Tip:** You can export all built-in presets to use as a starting point:
>
> ```bash
> cidx preset export > .cidx/presets.toml
> ```

**Example `.cidx/presets.toml`:**

```toml
[presets.my-custom-tool]
name = "my-custom-tool"
image = "myorg/tool:latest"
command = "run-check"
phase = "test"
description = "My custom internal tool"
workdir = "/work"
volumes = ["${WORKSPACE}:/work"]
```

Always set `workdir` and `volumes`: without them the container starts in `/` with nothing mounted, so the tool cannot see your project (e.g. `could not find Cargo.toml in /`). Every built-in preset mounts the project with `volumes = ["${WORKSPACE}:/work"]` and runs from `workdir = "/work"`.

### 4. Environment Variables

CIDX supports environment variable expansion in the configuration using `${VAR}` syntax. This is useful for passing dynamic information from a CI environment to your containers.

```toml
[containers.commitlint]
env = { FROM = "${CI_COMMIT_BEFORE_SHA}", TO = "${CI_COMMIT_SHA}" }
```
