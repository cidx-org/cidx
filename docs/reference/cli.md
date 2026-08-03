# CLI Reference

## Commands

### `cidx init`

Initialize a new configuration file in the current directory.

```bash
cidx init
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
- `--quiet, -q`: Suppress output and only show logs on failure.
- `--parallel, -p`: Run containers in parallel (local only).
- `--concurrency, -j`: Max concurrent containers (default: 2).

### `cidx preset`

Manage built-in container presets.

```bash
cidx preset <command>
```

**Subcommands:**

- `list`: List all available presets
- `info <name>`: Show details of a preset
- `show <name>`: Show raw TOML definition
- `export`: Dump all embedded presets to stdout (useful for creating a base `presets.toml`)
- `search <term>`: Search presets

### `cidx list` (deprecated)

Deprecated alias for `cidx preset list`.

```bash
cidx list
```

### `cidx info` (deprecated)

Deprecated alias for `cidx preset info <container>`.

```bash
cidx info <container>
```

### `cidx validate`

Validate the syntax and structure of the configuration file, then resolve every
`cidx` invocation found in the CI workflows against the current command tree — a
workflow calling a subcommand that has moved or disappeared fails validation
(issue #239).

```bash
cidx validate
cidx validate --workflow-dir .github/workflows
```

The invocation check reads the `run:` steps of the workflow files. It stays
silent on what it cannot read with certainty: invocations through a variable
(`$CIDX run ci`) or an expanded token (`cidx ${{ matrix.cmd }}`), heredoc
bodies, comments, quoted command names, and arguments of a command that handles
its own arguments. It reports nothing rather than accuse wrongly.

### `cidx release tag`

Tag management commands with prepare/preview/create workflow.

```bash
cidx release tag <command> [flags]
```

**Subcommands:**

#### `cidx release tag prepare`

Prepare a tag version and message for human review before creation.

```bash
cidx release tag prepare [flags]
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

#### `cidx release tag preview`

Preview what will happen during tag creation.

```bash
cidx release tag preview
```

**What it shows:**

- Prepared version and tag name
- Tag message preview
- Recent tags list
- Configuration summary
- Blockers (uncommitted changes, missing preparation)

#### `cidx release tag create`

Create and optionally push a git tag.

```bash
cidx release tag create [flags]
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

#### `cidx release tag delete`

Delete a git tag locally and optionally from remote.

```bash
cidx release tag delete <tag-name> [flags]
```

**Arguments:**

- `<tag-name>`: Name of the tag to delete (e.g., `v1.2.3`)

**Flags:**

- `--remote, -r`: Also delete from remote
- `--force, -f`: Force deletion of protected tags
- `--dry-run`: Show what would be done without making changes

**Protection:**

Tags matching patterns in `protected_tags` config cannot be deleted without `--force`.

#### `cidx release tag list`

List git tags with optional filtering.

```bash
cidx release tag list [flags]
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

### `cidx release`

Release management commands. See [Development Workflow](../guides/development-workflow.md) for detailed usage.

```bash
cidx release <command> [flags]
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

**Which workflow a pipeline is compared with:**

By convention, `[pipelines.ci]` is compared with `ci.yml`, `[pipelines.release]` with `release.yml`. A pipeline with no workflow file of that name is skipped.

Not every workflow mirrors a pipeline, though. A release workflow that publishes natively — cross-compilation, then `softprops/action-gh-release` — and only delegates one phase to `cidx run` is not an implementation of `[pipelines.release]`, and comparing the two reports a difference that is not a difference. Say so in the pipeline:

```toml
[pipelines.release]
phases = ["security", "code", "test", "build", "docker", "release"]
workflow = "none"   # no workflow implements this pipeline
```

`workflow` accepts a filename too, for a workflow that implements a pipeline under another name (`workflow = "main.yml"`). Left unset, the convention applies.

This is declared rather than inferred on purpose: from the outside, a job doing its phase natively and a job that lost its `cidx run` call look exactly alike, so any rule clever enough to excuse the first would also excuse the second — which is the drift this command exists to catch (issue #233).

**Example output:**

```text
✅ Pipeline 'ci' ↔ Workflow ci.yml
   Both execute phases: [security, code, test, build]
   Status: In sync ✓

⚠️  Pipeline 'release' ↔ Workflow release.yml
   Status: Out of sync ✗

   📄 cidx.toml [pipelines.release]:
      phases = [security, code, test, build, docker, release]

   🔧 GitHub Actions [release.yml]:
      executes = [docker]

   Differences:
      • Missing in GitHub workflow: security, code, test, build, release
```

---

### `cidx action` (deprecated — removed in v3.0.0)

The command tree that preceded the `repo` / `release` / `security` namespaces. Hidden since 2026-04-09, kept working since, and **removed in v3.0.0** (issue #235).

Every invocation still runs, and prints the command that replaces it:

```text
$ cidx action pr create --help
⚠️  'cidx action pr create' is deprecated and will be removed in cidx v3.0.0 -- use: 'cidx pr create'
```

The warning goes to stderr, so redirecting the command's output does not hide it.

| Deprecated                      | Replacement              |
| ------------------------------- | ------------------------ |
| `cidx action pr ...`            | `cidx pr ...`            |
| `cidx action cpw`               | `cidx cpw`               |
| `cidx action commit-push-watch` | `cidx cpw`               |
| `cidx action tag ...`           | `cidx release tag ...`   |
| `cidx action release ...`       | `cidx release ...`       |
| `cidx action artifact ...`      | `cidx repo artifact ...` |

Subcommands are unchanged on both sides: `cidx action tag prepare` is `cidx release tag prepare`, and so on.
