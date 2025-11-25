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
cidx action pr create "feat: add new security scanner preset"

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
cidx action pr create "feat: your feature title"
```

This command:

- Creates feature branch from main
- Creates initial empty commit (allows PR creation)
- Pushes branch to remote
- Creates **draft pull request** on GitHub
- Links to issue if provided with `--issue` flag

**Options**:

```bash
cidx action pr create "feat: new feature" --issue 42  # Link to issue #42
cidx action pr create "fix: bug" --dry-run            # Preview without creating
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

### 3. Mark PR Ready for Review

When your work is complete and CI passes:

```bash
cidx action pr ready
```

This command:

- Finds PR for current branch
- Marks PR as **ready for review** (no longer draft)
- Notifies team for review

**Note**: GitHub's REST API cannot convert draft→ready. CIDX uses `gh` CLI (GraphQL API) internally.

### 4. Merge PR

After approval and passing checks:

```bash
cidx action pr merge --watch
```

This command:

- Validates all CI checks passed (security, code quality, tests)
- Waits for pending checks to complete (if any)
- Merges PR to main (default: squash merge)
- Watches post-merge CI workflow
- Reports workflow status

**Options**:

```bash
cidx action pr merge --method squash    # Squash merge (default)
cidx action pr merge --method merge     # Standard merge
cidx action pr merge --method rebase    # Rebase merge
cidx action pr merge --watch            # Watch post-merge workflow
cidx action pr merge --skip-checks      # Bypass checks (not recommended)
cidx action pr merge --dry-run          # Preview without merging
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
cidx action release create
```

This command:

1. Analyzes conventional commits since last release
2. Calculates version bump (MAJOR.MINOR.PATCH)
3. Updates VERSION file and .cz.toml
4. Creates annotated Git tag (e.g., `v1.2.0`)
5. Pushes tag to remote
6. Triggers GitHub Release workflow automatically

**The release workflow** (`.github/workflows/release.yml`):

- Runs full CI checks (security, code quality, tests, build)
- Builds Docker image → pushes to `ghcr.io/arcker/cidx:VERSION`
- Creates GitHub Release with changelog and binaries
- Publishes release artifacts

**Options**:

```bash
cidx action release create --dry-run    # Preview without creating
```

### Manual Release (Fallback)

If `cidx action release create` fails (e.g., commitizen container issues):

```bash
# 1. Determine next version (check commits since last tag)
git log $(git describe --tags --abbrev=0)..HEAD --oneline

# 2. Update version files
echo "1.2.0" > VERSION
sed -i 's/version = "1.1.0"/version = "1.2.0"/' .cz.toml

# 3. Commit version bump
git add VERSION .cz.toml
git commit --no-verify -m "bump: version 1.1.0 → 1.2.0"

# 4. Create and push tag
git tag -a v1.2.0 -m "Release 1.2.0"
git push && git push --tags
```

The GitHub workflow automatically detects the tag push and creates the release.

---

## Tags and Releases

Understanding the relationship between Git tags and GitHub releases:

### Git Tags

**What**: Immutable Git references pointing to specific commits
**Purpose**: Mark release points in repository history
**Location**: Stored in Git repository (`.git/refs/tags/`)

```bash
# View all tags
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
- Docker image reference (`ghcr.io/arcker/cidx:1.2.0`)
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
    └── Docker: ghcr.io/arcker/cidx:1.2.0
```

### Timeline Example

```
[PR #1 merge] ─→ main (commit abc123)  ← no tag, no release
[PR #2 merge] ─→ main (commit def456)  ← no tag, no release
[PR #3 merge] ─→ main (commit ghi789)  ← no tag, no release

Decision: "Let's release these features"

cidx action release create
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
cidx action pr create "fix: critical security patch" --dry-run=false

# 4. Fast-track review and merge
cidx action pr ready
# Get approval
cidx action pr merge --watch

# 5. Immediate release
cidx action release create
# This creates a PATCH version (e.g., 1.2.3 → 1.2.4)
```

---

## Workflow Summary

### Development Cycle

```
┌─────────────────────────────────────────────────────┐
│ 1. Start Work                                       │
│    cidx action pr create "feat: new feature"        │
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
│    cidx action pr ready                             │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 4. Review & Merge                                   │
│    cidx action pr merge --watch                     │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 5. Main Branch (no tag, no release yet)            │
│    Accumulate multiple PRs                          │
└──────────────────┬──────────────────────────────────┘
                   ↓
┌─────────────────────────────────────────────────────┐
│ 6. Create Release (when ready)                      │
│    cidx action release create                       │
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
**Solution**: Delete tag locally and remotely if it was created incorrectly:

```bash
git tag -d v1.2.0
git push origin :refs/tags/v1.2.0
```

---

## Related Documentation

- [Architecture: Git Operations](../architecture-git-operations.md) - Why we use native git vs go-git
- [CI Integration Guide](ci-integration.md) - Setting up CI for your project
- [Creating Presets](creating-presets.md) - Adding new tool presets

---

**Last Updated**: 2025-01-25
**CIDX Version**: 1.1.0+
