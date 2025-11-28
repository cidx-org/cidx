# CLI Reference

## Commands

### `cidx init`

Initialize a new configuration file in the current directory.

```bash
cidx init
cidx init --format yaml  # Not yet implemented, defaults to toml
```

### `cidx run`

Execute a container, pipeline, or phase.

```bash
cidx run <name> [flags]
```

**Arguments:**

- `<name>`: Name of a container (e.g., `trivy`), pipeline (e.g., `ci`), or phase (e.g., `security`).

**Flags:**

- `--dry-run`: Print what would be executed without running it.

### `cidx list`

List all available containers and pipelines.

```bash
cidx list
```

### `cidx info`

Show detailed information about a specific container.

```bash
cidx info <container>
```

### `cidx validate`

Validate the syntax and structure of the configuration file.

```bash
cidx validate
```

### `cidx check workflow`

Validate that cidx.toml pipelines match GitHub Actions workflows.

```bash
cidx check workflow              # Validate all workflows
cidx check workflow ci           # Validate specific workflow
cidx check workflow --verbose    # Show detailed validation info
```

**Options:**

- `--workflow-dir`: Directory containing workflow files (default: `.github/workflows`)
- `--verbose, -v`: Show detailed validation information

**What it checks:**

- Phase presence: Ensures all phases in pipelines exist in workflows
- Phase order: Verifies phases execute in the correct order
- Consistency: Detects mismatches between local and CI/CD configurations

**Example output:**

```text
✅ Workflow ci.yml matches pipeline 'ci'
   Phases: [security, code, test, build]

⚠️  Workflow release.yml has differences with pipeline 'release'
   Missing in GitHub:  [docker]
   Order mismatch:
     Local:  [security, code, test, build, docker, release]
     GitHub: [security, code, test, build, release]
```
