# Environments & Local Safety Modes

How CIDX works out where it is running, and what it refuses to do when the answer is "someone's laptop". The supply-chain policy that governs the images it runs is a separate concern, starting at [Supply-Chain Policy](supply-chain-policy.md).

CIDX automatically detects its execution environment and adapts container behavior to ensure safe testing locally while enabling full automation in CI/CD.

## Core Philosophy

**"Test locally, deploy in CI"** - Any phase should be runnable from any environment, but with appropriate safety guards.

## Environment Detection

CIDX automatically detects:

- **Local**: Developer workstation
- **GitHub Actions**: `GITHUB_ACTIONS=true`
- **GitLab CI**: `GITLAB_CI=true`
- **Jenkins**: `JENKINS_HOME` or `JENKINS_URL`
- **CircleCI**: `CIRCLECI=true`

Detection also identifies:

- Pull/Merge Requests
- Tag-based builds
- Branch names

## Local Behaviors

Presets can define how they behave in local environments:

### `local_behavior = "production"`

Full execution (use with caution!)

- No restrictions
- Same behavior as CI

### `local_behavior = "draft"` ✅ Recommended for releases

Creates drafts only (GitHub releases)

- Automatically adds `--draft` flag
- Safe for testing release process locally
- **Example**: `gh-release`, `goreleaser`

### `local_behavior = "dry-run"` ✅ Recommended for Docker

Simulation only

- Shows what would execute, including the flags CI will use
- No actual execution
- **Example**: `docker-buildx`, `kaniko`

> **`no-push` was removed in v3.0.0** (issue #353). It set the same dry-run as
> `dry-run` did, so it never built anything either — while this page promised it
> "validates Dockerfile and build process", the one thing it could not do. What
> it added on top was cosmetic: a `DOCKER_PUSH=false` on a container that never
> starts, and a `--push` stripped from a command that is only printed. A config
> still carrying it is refused by name, and the replacement is `dry-run`.
>
> The `docker` phase is therefore exercised for real only in CI. That is a known
> gap, not a claim withdrawn quietly — it is what issue #338 describes for
> `goreleaser` and `docker-buildx` alike.

### `local_behavior = "disabled"`

Completely disabled locally

- Refuses to run outside CI
- For extremely sensitive operations

## Preset Configuration

```toml
[presets.gh-release]
name = "gh-release"
phase = "release"
# ...
require_ci = false           # Allow local execution
local_behavior = "draft"     # Create drafts only in local

[presets.docker-buildx]
name = "docker-buildx"
phase = "docker"
# ...
require_ci = false
local_behavior = "dry-run"   # Print what CI would run, build nothing
```

## Contextual Pipelines

Different pipelines for different lifecycle stages:

```toml
# Pull Request validation (no artifacts)
[pipelines.pr]
phases = ["security", "code", "test"]

# Main branch (build artifacts)
[pipelines.main]
phases = ["security", "code", "test", "build"]

# Full release (tags only, all phases)
[pipelines.release]
phases = ["security", "code", "test", "build", "docker", "release"]
```

## Usage Examples

### Local Development

```bash
# Quick code check
cidx run quick

# Full local validation (safe, no publish)
cidx run pr

# Test release locally (creates draft)
cidx run release
# → Docker builds without push
# → GitHub release created as draft
```

### CI/CD

```bash
# Pull Request
cidx run pr

# Main branch commit
cidx run main

# Tag-based release
cidx run release
# → Docker builds and pushes to registry
# → GitHub release published
```

## GitHub Actions Integration

```yaml
# Pull Request
- run: cidx run pr

# Main branch
- run: cidx run main
  if: github.ref == 'refs/heads/main'

# Release (tags only)
- run: cidx run release
  if: startsWith(github.ref, 'refs/tags/v')
```

## Benefits

1. **Safe Testing**: Test release processes locally without publishing
2. **Consistent Behavior**: Same commands work everywhere
3. **Auto-Detection**: No manual environment flags needed
4. **Flexible**: Override behaviors when necessary
5. **Secure**: Prevents accidental production publishes from local

## Future Enhancements

- Override flags: `--force-production`, `--force-dry-run`
- Pipeline-level `require_ci`
- Custom local behaviors per environment
- Validation warnings for risky operations
