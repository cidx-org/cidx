# Environment Detection & Security

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

### `local_behavior = "no-push"` ✅ Recommended for Docker

Build without push

- Docker builds locally but doesn't push to registry
- Validates Dockerfile and build process
- **Example**: `docker-buildx`, `kaniko`

### `local_behavior = "dry-run"`

Simulation only

- Shows what would execute
- No actual execution

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
local_behavior = "no-push"   # Build without push in local
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

## Image Supply-Chain Policy

This governs how the **built-in preset catalogue** is pinned and updated. It is not imposed on projects using CIDX — you pin whatever you want in your own `cidx.toml`.

### Why scanning is not enough

`container-monitor.yml` scans every candidate image with Trivy and Grype, and both refresh their databases on each run. That covers known vulnerabilities well. It does not cover a compromised image, for two reasons:

- **A CVE exists only once someone has found the flaw.** A deliberately backdoored image has no CVE until the world notices. In the xz-utils case the backdoor landed in February 2024 and was found on 29 March — for six weeks a perfectly current scanner reported green, because there was nothing yet to know.
- **Neither scanner does behavioural analysis.** They match an inventory of installed packages against CVE lists. A modified entrypoint, an added `curl | sh`, a binary swapped at an unchanged version — none of it registers, ever.

So there are two delays, and database freshness only addresses the first:

| Delay | Duration | Addressed by |
| ----- | -------- | ------------ |
| CVE published → scanner knows it | hours | database refresh |
| Malicious code published → someone discovers it | days to weeks | the cooldown below |

### The three rules

**1. Pin by digest.** Every catalogue image is written `image:tag@sha256:...`. The tag stays readable; the digest makes the reference immutable. A tag alone is mutable — `commitizen:4.15.1` can point at different content tomorrow with nothing in `presets.toml` changing. This is the quietest vector, and no version-based rule catches it.

**2. Wait 14 days.** A newly published version is not promoted until it has been publicly available for 14 days, comfortably past the usual detection window for a compromise (24–72h). The monitor runs weekly, so this is roughly two cycles.

Why not "always stay one version behind"? An attacker publishing twice in a row defeats it, and on a project that ships twice a year it would strand the catalogue on months-old CVEs. Age is measurable and independent of upstream's release cadence; lagging a version or two falls out of it naturally when upstream ships often.

**3. Waive the wait for a real fix.** When a new version fixes a vulnerability that actually affects us, it is promoted immediately — deliberately running a known-vulnerable image to guard against a hypothetical one is the worse trade. The waiver is stated in the promotion PR: which CVE, affecting which image, fixed by which version.

### What this costs

Security fixes reach the catalogue up to two weeks later than upstream publishes them, unless rule 3 applies. That is the deliberate price of rule 2. If that lag ever hurts more than it helps, the honest fix is to shorten the window — not to quietly bypass it.

## Future Enhancements

- Override flags: `--force-production`, `--force-dry-run`
- Pipeline-level `require_ci`
- Custom local behaviors per environment
- Validation warnings for risky operations
