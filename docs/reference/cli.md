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

### `cidx action tag`

Tag management commands with prepare/preview/create workflow.

```bash
cidx action tag <command> [flags]
```

**Subcommands:**

#### `cidx action tag prepare`

Prepare a tag version and message for human review before creation.

```bash
cidx action tag prepare [flags]
```

**Flags:**

- `--dry-run`: Show what would be generated without saving

**What it does:**

1. Reads current VERSION file and last git tag
2. Determines next version (via commitizen or patch increment)
3. Generates tag message with commit summary
4. Saves version to `.cidx/tag-version` (editable)
5. Saves message to `.cidx/tag-message` (editable)
6. Opens editor for review

#### `cidx action tag preview`

Preview what will happen during tag creation.

```bash
cidx action tag preview
```

**What it shows:**

- Prepared version and tag name
- Tag message preview
- Recent tags list
- Configuration summary
- Blockers (uncommitted changes, missing preparation)

#### `cidx action tag create`

Create and optionally push a git tag.

```bash
cidx action tag create [flags]
```

**Flags:**

- `--dry-run`: Show what would be done without making changes

**Requires:**

- No uncommitted changes
- Prepared version (via `tag prepare`)

**What it does:**

1. Creates annotated tag with prepared message
2. Signs with GPG if configured
3. Pushes to origin if `auto_push = true`
4. Cleans up prepared files

#### `cidx action tag delete`

Delete a git tag locally and optionally from remote.

```bash
cidx action tag delete <tag-name> [flags]
```

**Arguments:**

- `<tag-name>`: Name of the tag to delete (e.g., `v1.2.3`)

**Flags:**

- `--remote, -r`: Also delete from remote
- `--force, -f`: Force deletion of protected tags
- `--dry-run`: Show what would be done without making changes

**Protection:**

Tags matching patterns in `protected_tags` config cannot be deleted without `--force`.

#### `cidx action tag list`

List git tags with optional filtering.

```bash
cidx action tag list [flags]
```

**Flags:**

- `--limit, -n`: Limit number of tags shown (default: 20)
- `--pattern, -p`: Filter tags by pattern (e.g., `v1.*`)
- `--verbose, -v`: Show detailed tag information (type, date, commit)

**Example output (verbose):**

```text
🏷️  Tags (5):

  TAG                  TYPE       DATE                 COMMIT
  ---                  ----       ----                 ------
  v1.2.0               annotated  2025-12-04           18f0af6 🔒
  v1.1.1               lightweight 2025-11-26          95d8ae7 🔒
  v1.1.0               lightweight 2025-11-25          215beec 🔒

  🔒 = protected tag
```

---

### `cidx action release`

Release management commands. See [Development Workflow](../guides/development-workflow.md) for detailed usage.

```bash
cidx action release <command> [flags]
```

**Subcommands:**

- `prepare`: Prepare release notes for human review
- `preview`: Preview what will happen during release
- `commit`: Commit prepared release notes
- `create`: Create a new release (bump version, tag, push)

---

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
