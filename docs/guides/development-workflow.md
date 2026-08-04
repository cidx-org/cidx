# Development Workflow

This guide documents the development workflow for CIDX, following **trunk-based development** with **pull requests** and **grouped releases**.

## Philosophy

CIDX follows industry best practices for CLI tool development:

- **Trunk-based development**: Single main branch with short-lived feature branches
- **Pull requests**: All changes go through PR review workflow
- **Grouped releases**: Multiple features/fixes bundled into meaningful releases
- **Semantic versioning**: Automated version bumping based on conventional commits

## Table of Contents

1. [Daily Development Workflow](#daily-development-workflow)
2. [Pull Request Workflow](#pull-request-workflow)
3. [Release Workflow](#release-workflow)
4. [Tags and Releases](#tags-and-releases)
5. [Conventional Commits](#conventional-commits)
6. [Best Practices](#best-practices)

---

## Daily Development Workflow

### Starting New Work

All development follows trunk-based principles with short-lived feature branches:

```bash
# Ensure you're on main and up to date
git checkout main
git pull

# Create PR with CIDX (creates branch + draft PR automatically)
cidx pr create "feat: add new security scanner preset"

# OR manually create feature branch
git checkout -b feat/new-scanner
```

**Branch naming conventions**:

- `feat/description` - New features
- `fix/description` - Bug fixes
- `docs/description` - Documentation updates
- `refactor/description` - Code refactoring
- `test/description` - Test additions/updates

**Important**: Feature branches should be **short-lived** (1-3 days maximum). Commit and merge frequently to main.

### During Development

```bash
# Make your changes
git add .

# Commit with conventional commit format
git commit -m "feat: add trivy preset with custom severity"

# Push to remote
git push
```

Your commits automatically appear in the draft PR.

### CI Validation

Every push triggers CI checks on your branch:

- Security scanning (Trivy, Gitleaks)
- Code quality (golangci-lint, prettier)
- Tests (go test)
- Build validation

Fix any issues before marking PR as ready.

---

## Pull Request Workflow

CIDX provides automated PR commands following a GitLab-style workflow:

### 1. Create Draft PR

```bash
cidx pr create "feat: your feature title"
```

This command:

- Creates feature branch from main
- Creates initial empty commit (allows PR creation)
- Pushes branch to remote
- Creates **draft pull request** on GitHub
- Links to issue if provided with `--issue` flag

**Options**:

```bash
cidx pr create "feat: new feature" --issue 42  # Link to issue #42
cidx pr create "fix: bug" --dry-run            # Preview without creating
```

### 2. Work on Your Feature

```bash
# Make changes
git add .
git commit -m "feat: implement core logic"
git push

# Make more changes
git add .
git commit -m "test: add unit tests"
git push
```

All commits automatically appear in the PR. CI checks run on each push.

`cidx cpw -m "..."` does the same round trip and runs the `code` phase first,
so a prettier reflow or a golangci-lint remark is caught before the push
instead of costing a full CI cycle plus a second commit (issue #307). The phase
takes about 20 seconds once the images are local; the first run pulls them and
takes around three minutes.

It is on by default, with the same escape hatch git gives its hooks:

```bash
cidx cpw -m "feat: implement core logic"              # runs the code phase, then pushes
cidx cpw --no-verify -m "wip: checkpoint"             # pushes without checking
```

Three situations skip the phase, each with a message saying so, and none of
them blocks the push:

- `--no-verify` was given
- a pre-commit hook is installed (`git config core.hooksPath .githooks`) — it
  runs the same phase on the commit, and running it twice buys nothing
- there is nothing to run, or nothing to run it in: no `[code]` phase in
  `cidx.toml`, or no container runtime cidx can use. Run `cidx doctor` to see
  which

Only a real failure stops cpw, and it stops it _before_ the commit, so the tree
is left exactly as it was.

### 3. Mark PR Ready for Review

When your work is complete and CI passes:

```bash
cidx pr ready
```

This command:

- Finds PR for current branch
- Marks PR as **ready for review** (no longer draft)
- Notifies team for review

**Note**: GitHub's REST API cannot convert draft→ready. CIDX uses `gh` CLI (GraphQL API) internally.

### 4. Merge PR

After approval and passing checks:

```bash
cidx pr merge --watch
```

This command:

- Validates all CI checks passed (security, code quality, tests)
- Waits for pending checks to complete (if any)
- Merges PR to main (default: squash merge)
- Watches post-merge CI workflow
- Reports workflow status

**Options**:

```bash
cidx pr merge --method squash    # Squash merge (default)
cidx pr merge --method merge     # Standard merge
cidx pr merge --method rebase    # Rebase merge
cidx pr merge --watch            # Watch post-merge workflow
cidx pr merge --skip-checks      # Bypass checks (not recommended)
cidx pr merge --dry-run          # Preview without merging
```

**Pre-merge validation**:

- All CI checks must pass (unless `--skip-checks`)
- Displays check status with visual indicators
- Fails merge if checks fail

**Post-merge**:

- PR merged to main
- CI workflow runs on main
- **No tag created** (tags are only for releases)
- **No release created** (releases are manual/grouped)

---

## Release Workflow

CIDX follows **grouped releases**: multiple PRs are merged to main, then released together as a meaningful version.

### When to Release

Create a release when:

- You have 3-5 merged features/fixes ready to publish
- A critical bug fix needs to go out
- You reach a planned milestone
- End of sprint/iteration

**Important**: Not every PR merge creates a release. Main accumulates changes between releases.

### Creating a Release

```bash
cidx release create
```

This command:

1. Analyzes conventional commits since last release
2. Calculates version bump (MAJOR.MINOR.PATCH)
3. Updates VERSION, .cz.toml and CHANGELOG.md, and commits the bump
4. Moves that commit onto a `chore/release-vX.Y.Z` branch (main stays untouched)
5. Opens the release pull request, waits for CI, and squash-merges it
6. Creates the annotated Git tag (e.g., `v1.2.0`) on the **merged** commit and pushes it
7. Triggers GitHub Release workflow automatically

The bump always travels through a PR, so it behaves identically with or without
branch protection on main. Tag pushes are not covered by branch rulesets, which
is why the tag can be pushed directly after the merge.

**If the release PR's CI fails**, nothing is tagged. The bump stays on the PR
branch: fix the failure, `cidx cpw -m "fix: ..."`, `cidx pr merge`, then
`cidx release tag prepare && cidx release tag create`.

**If a release is already in flight** (a `chore/release-*` branch exists on the
remote), `release create` refuses to start a second one and points at the open PR.

**The release workflow** (`.github/workflows/release.yml`):

- Cross-compiles the binaries for Linux, macOS and Windows, natively
- Builds the Docker image with `cidx run docker` → pushes to `ghcr.io/cidx-org/cidx:VERSION`
- Creates the GitHub Release with `softprops/action-gh-release`, changelog and binaries attached

It does **not** re-run security, code and test: the tag is cut from a `main`
commit those phases already passed on in `ci.yml`. And it publishes the release
natively, because release publishing is deliberately outside cidx's scope —
cross-compilation plus asset attachment is a handful of lines of shell run once
per release, and a preset for it would add complexity for no friction removed
(guardrail 3).

So `release.yml` is the _publication_ workflow, not an implementation of
`[pipelines.release]`. That pipeline is the end-to-end rehearsal `cidx run
release` walks locally with the guardrails on (`gh-release` drafts, `kaniko`
does not push) — see [Local Safety](../core-concepts/security.md). The two are
not meant to coincide, which is why `cidx.toml` states `workflow = "none"` on
`[pipelines.release]` so `cidx check workflow` does not compare them (issue
#233). See [`cidx check workflow`](../reference/cli.md#cidx-check-workflow).

**Options**:

```bash
cidx release create --dry-run    # Preview without creating
```

### Manual Release (Fallback)

If `cidx release create` fails (e.g., commitizen container issues):

```bash
# 1. Determine next version (check commits since last tag)
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# 2. Update version files
echo "1.2.0" > VERSION
sed -i 's/version = "1.1.0"/version = "1.2.0"/' .cz.toml

# 3. Commit the version bump on a branch and merge it through a PR
#    (direct pushes to main are rejected when a ruleset requires PRs)
cidx pr create "chore(release): bump version to v1.2.0"
cidx cpw -m "bump: version 1.1.0 → 1.2.0"
cidx pr merge

# 4. Create and push the tag on the merged commit
git tag -a v1.2.0 -m "Release 1.2.0"
git push origin v1.2.0
```

The GitHub workflow automatically detects the tag push and creates the release.

---

## Tags and Releases

Understanding the relationship between Git tags and GitHub releases:

### Tag Management Commands

CIDX provides dedicated commands for managing git tags with human review:

#### Tag Workflow

```bash
# 1. Prepare tag (determines version, generates message)
cidx release tag prepare

# 2. Edit prepared files (optional)
# - .cidx/tag-version: Target version number
# - .cidx/tag-message: Tag annotation message

# 3. Preview what will happen
cidx release tag preview

# 4. Create and push the tag
cidx release tag create
```

#### Tag Utilities

```bash
# List tags with details
cidx release tag list --verbose

# Filter by pattern
cidx release tag list --pattern "v1.*"

# Delete a tag (locally)
cidx release tag delete v1.2.3

# Delete a tag (local + remote)
cidx release tag delete v1.2.3 --remote

# Delete a protected tag (requires --force)
cidx release tag delete v1.0.0 --remote --force
```

#### Tag Configuration

Configure tag behavior in `cidx.toml`:

```toml
[tag_workflow]
prefix = "v"                    # Tag prefix (v1.2.3)
use_commitizen = true           # Auto-determine version from commits
auto_push = true                # Push tags automatically
sign_tags = false               # GPG signing
require_annotated = true        # Require annotated tags
protected_tags = ["v1.*"]       # Patterns that can't be deleted
linked_to_release = true        # Tags trigger release workflow
```

### Git Tags

**What**: Immutable Git references pointing to specific commits
**Purpose**: Mark release points in repository history
**Location**: Stored in Git repository (`.git/refs/tags/`)

```bash
# View all tags (with cidx)
cidx release tag list

# View all tags (with git)
git tag

# View tag details
git show v1.2.0

# Tags are NOT created on every merge
```

**Tag naming convention**:

- `v1.2.3` - Stable releases (semantic versioning)
- `v2.0.0-beta.1` - Pre-releases
- `v1.2.3-rc.1` - Release candidates

### GitHub Releases

**What**: GitHub feature that packages a tag with release notes and artifacts
**Purpose**: Provide downloadable releases for users
**Location**: GitHub web interface + API

**A GitHub Release includes**:

- Git tag reference (e.g., `v1.2.0`)
- Release notes (changelog)
- Binary artifacts (cidx binary, checksums)
- Docker image reference (`ghcr.io/cidx-org/cidx:1.2.0`)
- Release metadata (date, author)

### Relationship

```
Git Tag (v1.2.0)
    ↓
GitHub Release Workflow
    ↓
GitHub Release (v1.2.0)
    ├── Release Notes
    ├── Binary: cidx_linux_amd64
    ├── Binary: cidx_darwin_amd64
    └── Docker: ghcr.io/cidx-org/cidx:1.2.0
```

### Timeline Example

```
[PR #1 merge] ─→ main (commit abc123)  ← no tag, no release
[PR #2 merge] ─→ main (commit def456)  ← no tag, no release
[PR #3 merge] ─→ main (commit ghi789)  ← no tag, no release

Decision: "Let's release these features"

cidx release create
    ↓
Git tag v1.2.0 created on commit ghi789
    ↓
Tag pushed to GitHub
    ↓
Release workflow triggered
    ↓
GitHub Release v1.2.0 published
    ↓
Users can download cidx v1.2.0
```

**Key Points**:

- ✅ Tags are created **only during releases**
- ✅ No tags between releases (this is normal and correct)
- ✅ Main branch can be ahead of latest release
- ✅ One tag = One release

---

## Conventional Commits

CIDX uses conventional commits for automated version bumping:

### Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Types

| Type        | Description             | Version Bump          |
| ----------- | ----------------------- | --------------------- |
| `feat:`     | New feature             | MINOR (1.1.0 → 1.2.0) |
| `fix:`      | Bug fix                 | PATCH (1.1.0 → 1.1.1) |
| `docs:`     | Documentation only      | PATCH                 |
| `style:`    | Code style (formatting) | PATCH                 |
| `refactor:` | Code refactoring        | PATCH                 |
| `perf:`     | Performance improvement | PATCH                 |
| `test:`     | Adding/updating tests   | PATCH                 |
| `chore:`    | Maintenance tasks       | PATCH                 |
| `ci:`       | CI/CD changes           | PATCH                 |

### Breaking Changes

Add `!` after type or include `BREAKING CHANGE:` in footer:

```bash
feat!: redesign configuration format

BREAKING CHANGE: Config now uses TOML instead of YAML
```

**Version bump**: MAJOR (1.2.0 → 2.0.0)

### Examples

```bash
# Feature (MINOR bump)
git commit -m "feat: add kaniko preset for Docker builds"

# Bug fix (PATCH bump)
git commit -m "fix: resolve permission issue with commitizen container"

# Breaking change (MAJOR bump)
git commit -m "feat!: change action command syntax"

# With scope
git commit -m "feat(presets): add megalinter preset with custom rules"

# With body and footer
git commit -m "fix: handle empty commit in PR creation

Previously PR creation failed if branch had no commits.
Now automatically creates empty commit to satisfy GitHub API.

Closes #42"
```

### Why Conventional Commits?

1. **Automated versioning**: Commitizen analyzes commits to determine bump
2. **Clear changelog**: Auto-generated release notes
3. **Consistent history**: Easy to understand project evolution
4. **Tooling integration**: Works with semantic-release, commitizen, etc.

---

## Best Practices

### For Feature Development

1. **Keep branches short-lived** (1-3 days max)
   - Don't let feature branches diverge from main
   - Merge frequently to reduce conflicts

2. **Use draft PRs early**
   - Create PR on day 1 to show work-in-progress
   - Get early feedback on direction
   - CI validates changes continuously

3. **Write good commit messages**
   - Follow conventional commits format
   - Include context in body for complex changes
   - Reference issues when applicable

4. **Small, focused PRs**
   - One feature or fix per PR
   - Easier to review and merge
   - Reduces risk of conflicts

### For Reviews

1. **Review promptly**
   - PRs block trunk, review within 24h
   - Use GitHub review features (comment, approve, request changes)

2. **CI is gatekeeper**
   - All checks must pass before merge
   - Don't use `--skip-checks` in production

### For Releases

1. **Group related changes**
   - Don't release every single PR
   - Wait for 3-5 meaningful changes
   - Balance between features and stability

2. **Test before release**
   - Run `cidx validate` and local tests
   - Verify CI passed on main
   - Consider manual smoke test

3. **Write good release notes**
   - Highlight new features
   - List breaking changes prominently
   - Credit contributors

4. **Monitor release workflow**
   - Use `--watch` to monitor workflow
   - Check Docker image published correctly
   - Verify GitHub Release created

### For Hotfixes

Critical bugs need fast release:

```bash
# 1. Create hotfix branch from main
git checkout main
git pull
git checkout -b fix/critical-security-issue

# 2. Fix the bug
git commit -m "fix: patch critical security vulnerability"

# 3. Push and create PR
git push -u origin fix/critical-security-issue
cidx pr create "fix: critical security patch" --dry-run=false

# 4. Fast-track review and merge
cidx pr ready
# Get approval
cidx pr merge --watch

# 5. Immediate release
cidx release create
# This creates a PATCH version (e.g., 1.2.3 → 1.2.4)
```

---

## Workflow Summary

### Development Cycle

```
┌─────────────────────────────────────────────────────┐
│ 1. Start Work                                       │
│    cidx pr create "feat: new feature"               │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 2. Implement                                        │
│    git commit -m "feat: implement core logic"       │
│    git push (CI checks run automatically)           │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 3. Mark Ready                                       │
│    cidx pr ready                                    │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 4. Review & Merge                                   │
│    cidx pr merge --watch                            │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 5. Main Branch (no tag, no release yet)             │
│    Accumulate multiple PRs                          │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 6. Create Release (when ready)                      │
│    cidx release create                              │
│    → Git tag created                                │
│    → GitHub Release workflow triggered              │
│    → Release published automatically                │
└─────────────────────────────────────────────────────┘
```

### Key Principles

1. **Trunk-based**: One main branch, short-lived features
2. **PR-based**: All changes via pull requests
3. **CI-validated**: Every commit runs full CI checks
4. **Grouped releases**: Multiple changes per release
5. **Automated versioning**: Conventional commits drive versions
6. **Git tags = Releases**: Tags only created for releases

---

## Troubleshooting

### PR Creation Fails

**Error**: "No commits between main and branch"
**Solution**: This is fixed in current version. CIDX automatically creates empty commit.

### PR Merge Fails

**Error**: "Some checks have failed"
**Solution**: Fix failing checks before merging. Use `--skip-checks` only for emergencies.

### Release Creation Fails

**Error**: "Container exited with code 16"
**Solution**: Known issue with commitizen container permissions. Use manual release process (see above).

### Tag Already Exists

**Error**: "Tag v1.2.0 already exists"
**Solution**: Delete tag using cidx or git:

```bash
# Using cidx (recommended)
cidx release tag delete v1.2.0 --remote

# Using git directly
git tag -d v1.2.0
git push origin :refs/tags/v1.2.0
```

### Cannot Delete Protected Tag

**Error**: "tag v1.0.0 is protected and cannot be deleted"
**Solution**: Use `--force` flag to override protection:

```bash
cidx release tag delete v1.0.0 --remote --force
```

### No Prepared Version Found

**Error**: "no prepared version found"
**Solution**: Run `tag prepare` before `tag create`:

```bash
cidx release tag prepare
# Review/edit .cidx/tag-version and .cidx/tag-message
cidx release tag create
```

---

## Nightly Builds

CIDX automatically builds and publishes nightly versions from the main branch.

### What Are Nightly Builds?

Nightly builds are development versions published after every merge to main:

- **Docker image**: `ghcr.io/cidx-org/cidx:nightly`
- **Binary artifact**: Available in GitHub Actions for 7 days
- **Version format**: `X.Y.Z-nightly.YYYYMMDD.SHORTSHA` (e.g., `1.2.0-nightly.20251206.abc1234`)

### When to Use Nightly

Use nightly builds when you want to:

- Test the latest features before a release
- Validate fixes merged to main
- Run CI with cutting-edge cidx version

```bash
# Pull nightly Docker image
docker pull ghcr.io/cidx-org/cidx:nightly

# Use in your CI
docker run ghcr.io/cidx-org/cidx:nightly cidx validate
```

### Nightly vs Release

| Aspect         | Nightly                  | Release                      |
| -------------- | ------------------------ | ---------------------------- |
| Trigger        | Every push to main       | Manual `cidx release create` |
| Docker tag     | `:nightly`               | `:vX.Y.Z` and `:latest`      |
| Stability      | Development              | Production-ready             |
| GitHub Release | No                       | Yes                          |
| Version        | `X.Y.Z-nightly.DATE.SHA` | `X.Y.Z`                      |

**Important**: Nightly builds are **not** production-ready. Use tagged releases for production.

---

## Related Documentation

- [Architecture: Git Operations](../architecture-git-operations.md) - Why we use native git vs go-git
- [CI Integration Guide](ci-integration.md) - Setting up CI for your project
- [Creating Presets](creating-presets.md) - Adding new tool presets

---

**Last Updated**: 2025-12-06
**CIDX Version**: 1.2.0+
