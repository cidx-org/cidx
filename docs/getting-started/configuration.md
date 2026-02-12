# Configuration

CIDX is configured via a `cidx.toml` file in the root of your project.

## Initialization

To create a default configuration:

```bash
cidx init
```

### 5. Version Pinning (Security)

To ensure consistency between local development and CI, you can enforce a specific version of CIDX. This guarantees that everyone uses the exact same binary and embedded presets.

```toml
# cidx.toml
required_version = "1.2.3"
```

If a user tries to run the pipeline with a different version (e.g., `1.2.4`), CIDX will refuse to start.

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

#### Defining Custom Containers

You can also define your own containers that don't have built-in presets. You must assign them to a default `phase` so CIDX knows when to run them.

```toml
[containers.my-scanner]
phase = "security"
image = "myregistry/scanner:latest"
command = "scan ."
```

### 3. Custom Presets (Advanced)

While `cidx.toml` configures *how* tools are used (which phase, which pipeline), `presets.toml` allows you to configure *what* the tools are (images, commands).

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
```

### 4. Environment Variables

CIDX supports environment variable expansion in the configuration using `${VAR}` syntax. This is useful for passing dynamic information from a CI environment to your containers.

```toml
[containers.commitlint]
env = { FROM = "${CI_COMMIT_BEFORE_SHA}", TO = "${CI_COMMIT_SHA}" }
```
