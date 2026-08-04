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
- `--stream`: Stream container output live, at the normal log level.
- `--parallel, -p`: Run containers in parallel (local only).
- `--concurrency, -j`: Max concurrent containers (default: 2).

**Output on a CI runner.** `cidx run` turns `--quiet` on by itself when it
detects CI: container output is buffered and dropped when the container
succeeds, so a passing lint costs a line rather than a page. That is the wrong
default for a job whose output _is_ the evidence — a green test job printing
`go-test completed` cannot tell you whether the suite ran every test or none of
them. `--stream` turns the buffering off and nothing else:

| Invocation                | Container output | Log level |
| ------------------------- | ---------------- | --------- |
| `cidx run test` (local)   | streamed         | info      |
| `cidx run test` (CI)      | buffered         | info      |
| `cidx run --stream test`  | streamed         | info      |
| `cidx --verbose run test` | streamed         | **debug** |
| `cidx run --quiet test`   | buffered         | info      |

`--verbose` also defeats the buffering, but it switches logrus to Debug and
prints the raw JSON of every image pull with it — which is why this repository's
own `ci.yml` runs `./bin/cidx run --stream test` (issue #273). `--stream` wins
over `--quiet` when both are given.

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

### `cidx repo branch cleanup`

Delete a branch the repository is finished with, locally and on the remote.

```bash
cidx repo branch cleanup                      # dry run on the current branch
cidx repo branch cleanup --branch feat/x -x   # delete feat/x
cidx repo branch cleanup --all -x             # delete every merged branch
```

**Options:**

- `--execute, -x`: actually delete; without it the run only prints what it would do
- `--branch, -b`: branch to clean up (default: the current branch)
- `--all, -a`: sweep every merged branch in the repository
- `--stale`: with `--all`, also sweep branches inactive for more than `[branch] stale_days`
- `--orphan`: with `--all`, also sweep branches whose PR was closed without merging
- `--force, -f`: delete a branch the repository is not finished with

**Scope**

A run deletes one branch: the one `--branch` names, or the current one. `--all` restores the repository-wide sweep, which is what this command used to do with no way to narrow it — reaching for it to remove a single merged branch removed seventeen (issue #269). The scope is printed on every run.

The current branch cannot be deleted: git refuses to remove the branch you are standing on. So `cleanup` with no flags reports that and stops — on `main`, because it is protected; on a feature branch, because it is where you are. Deleting the branch you just merged is `cidx pr merge`'s job, which switches to `main` first.

**Which branches may go**

A named branch is deleted when the repository is visibly finished with it — merged into the main branch, or carrying a PR that was merged or closed without merging (the same verdicts `cidx repo branch list --merged` and `--orphan` show). Anything else, above all a branch with an open PR, is refused by name:

```text
Scope: branch 'feat/wip' (--all sweeps every merged branch)

Skipped branches:
  ⊘ feat/wip (PR #42 is still open -- pass --force to delete anyway)
```

`--force` overrides that. It does not override protection: `main`, `master` and `develop` (or whatever `[branch] protected` lists) are never deleted.

---

### `cidx repo artifact download`

Download the artifacts a workflow run produced, into one flat directory.

```bash
cidx repo artifact download --run 30892230196                        # every artifact
cidx repo artifact download --run 30892230196 'trivy-*' 'grype-*'    # by name pattern
cidx repo artifact download --run 30892230196 -o /tmp/audit          # elsewhere
cidx repo artifact download                                          # latest run on this branch
```

**Options:**

- `--run`: run whose artifacts to download (default: the most recent run on the current branch)
- `--output, -o`: destination directory (default: `scan-results`)

**Arguments:** artifact name patterns. Globs are honoured; no pattern means every artifact of the run. A pattern that matches nothing is refused, naming the artifacts the run does have — a typo would otherwise leave an empty directory that reads as a scan which found nothing.

**Why it exists**

`cidx security vuln prune --results DIR` and `cidx security baseline --results DIR` read the scanner results the Security Audit and Container Monitor workflows upload. Until this command, putting them on disk meant `gh run download` — a cidx command depending on artifacts with no cidx way to obtain them (issue #285).

`--output` defaults to `scan-results`, which is what `--results` defaults to, so the flow needs no path spelled twice:

```bash
cidx repo artifact download --run 30892230196
cidx security vuln prune
```

**What it does that `gh run download` does not**

- **The repository is this one.** It comes from the git remote of the working directory, and the command prints it. `gh run download <id>` resolves the repository from gh's own notion of where you are — a default, `GH_REPO`, the last thing it knew — and hands over another repository's artifacts without a word; that skewed a before/after measurement in #327.
- **One flat directory.** `gh run download` unpacks a subdirectory per artifact. The readers join the results directory with a bare file name, so 42 `trivy-N/` subdirectories are 42 directories none of them looks in: on the same audit run, `vuln prune` reports 21 of 21 catalogue repositories covered from this command's output and **0 of 21** from `gh run download`'s (#333).
- **A shared file name is not fatal.** Identical content is skipped; differing content keeps the first copy and names both artifacts. Aborting halfway through a 42-artifact download would leave a directory that reads as a complete scan and is not one.

Archive entries are written under their base name, so an artifact naming `../../.ssh/authorized_keys` cannot write outside the destination.

**GitLab:** artifacts belong to jobs rather than to pipelines, so `--run` takes a pipeline ID and each job that uploaded an archive is one artifact, named after the job.

---

### `cidx repo workflow rerun`

Restart a run, or only the jobs of it that failed.

```bash
cidx repo workflow rerun --failed                 # latest run on the current branch
cidx repo workflow rerun --failed 30819803199     # a specific run
cidx repo workflow rerun 30819803199              # every job of it
```

**Options:**

- `--failed`: restart only the jobs that failed (or were cancelled, or timed out)
- `--run`: run to restart, by ID — the same thing as the positional argument

**Why it exists**

A job that dies pulling a pinned image (`read: connection reset by peer`) failed on the infrastructure, not on the change. Recovery meant `gh run rerun --failed`, because `cidx repo workflow run` only does `workflow_dispatch` and `ci.yml` declares none — the one step of the loop that still had to leave cidx, reached for exactly when something has just gone wrong (issue #342).

The identifier is the `id` column of `cidx repo workflow list`, not the `#` run number beside it; a run that cannot be read says so. `--failed` on a run with no failed job is refused here rather than as GitHub's bare 403, and names the command that does work.

The rerun is not watched. It starts a new attempt of the same run and the API reports the previous one as completed for a few seconds, so a watch chained onto it would report the failure it was asked to clear; the command to watch it is printed instead.

**GitLab:** `--failed` retries the pipeline's failed jobs. Restarting a pipeline that has none has no counterpart on the platform — the answer there is a new pipeline, which is `cidx repo workflow run --ref <ref>` — so it is refused, saying so.

---

### `cidx repo workflow list`

List recent runs, for one workflow or for a branch.

```bash
cidx repo workflow list                    # every workflow, current branch
cidx repo workflow list ci                 # the ci workflow, every branch
cidx repo workflow list --branch main      # every workflow, main
cidx repo workflow list -n 5 -v security-audit
```

**Options:**

- `--branch, -b`: branch to list runs for (default: the current branch, when no workflow is named)
- `--limit, -n`: how many runs to show (default: 10)
- `--verbose, -v`: one row per run with branch, workflow and title

With no workflow name it lists the runs of every workflow on a branch, which is what you want when a check has just failed and you do not yet know which workflow owns it — the failing check names a job, not a workflow file (issue #342).

The `id` column is the identifier `workflow watch`, `workflow rerun` and `artifact download` take. The `#` column is the number the web UI shows. They are different numbers and handing over the wrong one gets a flat 404 (issue #291).

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
