# Configuration

CIDX is configured via a `cidx.toml` file in the root of your project.

## Initialization

To create a default configuration:

```bash
cidx init
```

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

### Environment Variables

CIDX supports environment variable expansion in the configuration using `${VAR}` syntax. This is useful for passing dynamic information from a CI environment to your containers.

```toml
[containers.commitlint]
env = { FROM = "${CI_COMMIT_BEFORE_SHA}", TO = "${CI_COMMIT_SHA}" }
```
