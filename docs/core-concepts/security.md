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

| Delay                                           | Duration      | Addressed by       |
| ----------------------------------------------- | ------------- | ------------------ |
| CVE published → scanner knows it                | hours         | database refresh   |
| Malicious code published → someone discovers it | days to weeks | the cooldown below |

### The three rules

**1. Pin by digest.** Every catalogue image is written `image:tag@sha256:...`. The tag stays readable; the digest makes the reference immutable. A tag alone is mutable — `commitizen:4.15.1` can point at different content tomorrow with nothing in `presets.toml` changing. This is the quietest vector, and no version-based rule catches it.

**2. Wait 14 days.** A newly published version is not promoted until it has been publicly available for 14 days, comfortably past the usual detection window for a compromise (24–72h). The monitor runs weekly, so this is roughly two cycles.

Why not "always stay one version behind"? An attacker publishing twice in a row defeats it, and on a project that ships twice a year it would strand the catalogue on months-old CVEs. Age is measurable and independent of upstream's release cadence; lagging a version or two falls out of it naturally when upstream ships often.

**3. Waive the wait for a real fix.** When a new version fixes a vulnerability that actually affects us, it is promoted immediately — deliberately running a known-vulnerable image to guard against a hypothetical one is the worse trade. The waiver is stated in the promotion PR: which CVE, affecting which image, fixed by which version.

### How the rules are applied

`cidx preset scan-targets` decides, per image, what `container-monitor.yml` scans and which candidates are old enough to consider; `cidx preset scan-verdicts` then decides which of them the scan results allow. The workflow only reads those verdicts — the policy lives in code, where it is testable, rather than in shell scattered across a YAML file.

**Where the age comes from.** The cooldown is measured against the date the registry reports for the candidate tag, taken from the same call that finds the tag, so it costs no extra request:

| Registry   | Date used                 | Meaning                                               |
| ---------- | ------------------------- | ----------------------------------------------------- |
| Docker Hub | `last_updated` on the tag | when that tag last received content                   |
| Quay.io    | `start_ts` on the tag     | when that tag started pointing at its current content  |
| gcr.io     | `timeUploadedMs`          | when the registry received that tag's manifest         |
| ghcr.io    | none                      | tags are listed, dated nowhere we can read (see below) |
| dhi.io     | none                      | idem                                                   |

Every one of those dates restarts if a tag is republished with new content, which is what the cooldown wants: new content, new wait.

The OCI distribution API itself carries no publication date. The nearest substitute is the `created` field of the image config blob, and it is deliberately **not** used: that is a _build_ date, which can precede publication by an arbitrary amount. A date that is too old would silently shorten the cooldown, which is worse than having none.

**Fail-closed.** A candidate whose publication date cannot be determined is not promoted — the same posture as rule 1's unresolvable digest. It is reported in the workflow summary with the reason, not silently discarded, and the current pinned image keeps being scanned in the meantime.

### Finding versions on the registries that only list tags

`GET /v2/<repo>/tags/list` works on every OCI registry the catalogue pulls from, behind the same Bearer challenge as the manifest lookup, and that is how `cidx preset scan-targets` reaches gcr.io, ghcr.io and dhi.io — 9 of the 21 catalogue images, every Docker Hardened Image among them, which had no update detection at all before #245.

The listing is an unordered set of names, so the newest version is worked out from the names themselves. A version qualifies only if it has the **same shape** as the tag the catalogue pins: the same `v` prefix, the same variant suffix, the same number of components. `dhi.io/golang:1.23-alpine3.21-dev` is therefore never offered a plain `1.24` — a different base image — and an image pinned `0.68` is offered `0.71` rather than `0.71.2`. Versions compare as numbers: `1.24` is newer than `1.9`, which no lexical ordering would say.

**A newer version is not always a candidate.** ghcr.io and dhi.io date nothing:

- ghcr.io's dates live in the GitHub Packages API, which needs a `read:packages` token and answers 403 for a package owned by another organisation.
- dhi.io has no repository on `hub.docker.com` to ask, and its registry response carries names only.

Reporting versions found there as candidates would be worse than reporting nothing: the cooldown is fail-closed, so each one would be held in every weekly run from now until someone acted on it by hand — noise that never resolves. They are reported in a state of their own instead, `newer_version` with a reason saying the registry publishes no date, and the workflow summary lists them under **Newer version, not promotable automatically**. Pinning one is a deliberate act with a human behind it.

### A pinned image that vanished

Rule 1 makes a reference immutable; it does not make it eternal. Two catalogue images — `dhi.io/alpine-base:3.21` and `dhi.io/docker:27-cli` — were deleted upstream and answered 404, and nothing noticed until the presets using them failed to start (#244).

`cidx preset scan-targets` now resolves the exact reference each catalogue image is pinned to, digest included, and marks it `missing` when the registry says it does not exist. `container-monitor.yml` annotates the run with an error and fails its summary job, so the weekly run goes red. A 401 from a registry we hold no credentials for is reported as an unverified image, never as a deleted one — the loudest signal the command has must not cry wolf.

**How rule 3 knows what affects us.** `known-vulnerabilities.toml` already records the HIGH/CRITICAL findings accepted against the images the catalogue runs today — that is the list of vulnerabilities demonstrably affecting us, produced by the security audit. A candidate replacing an image with entries in that file is promoted without waiting, and the promotion PR names them. No second scan is run to obtain this: the current image's vulnerabilities are on file, and the monitor already scans the candidate.

A candidate that has served the full 14 days claims no waiver, even when the running image is vulnerable — a waiver line in the PR means the cooldown was actually bypassed, or the record stops being worth reading.

### The scan gate is differential

The cooldown decides whether a version is old enough to consider. What the scanners find on it decides whether it may actually replace the running image — and for a long time it decided nothing at all. Every scan step wrapped `docker run` in an `if … then … else …`, so a vulnerable image exited 0 and the job succeeded; the promote job read `needs.trivy-scan.result`, which was `success` by construction. The promotion PR claimed a check that had never run (#247).

Making the job fail on any HIGH/CRITICAL is not the fix. Several catalogue images are knowingly vulnerable — that is precisely what `known-vulnerabilities.toml` records — so a fail-hard gate would leave the monitor permanently red, and a gate that is always red is ignored exactly like one that is always green.

The verdict is therefore **per candidate** and **differential**:

```
held  ⟺  the scanners report a HIGH/CRITICAL finding on the candidate
         that is accepted neither for the image we run today
         nor for the candidate's own reference
```

A candidate carrying the same vulnerabilities as the running image is not a regression: those findings are already on file, they subtract out, and the update ships. What blocks a promotion is a finding that is **new**. `known-vulnerabilities.toml` is what makes the comparison possible without a second scan — `security-audit.yml` fails daily on any HIGH/CRITICAL that is not on file, so that record _is_ the status of the pinned image.

**Two cumulative gates, in this order.** The cooldown runs first, in `cidx preset scan-targets`: only a candidate it clears becomes the image the monitor scans. The scan gate then judges what came back, in `cidx preset scan-verdicts`. Both must pass, and rule 3's waiver applies to the cooldown alone — a candidate promoted early because it fixes a CVE affecting us is still held if it brings a different one along.

**Fail-closed, again.** A candidate that neither scanner produced a readable result for is held. An empty scanner output — what a failed pull leaves behind — parses as no JSON at all rather than as a clean image.

One scanner missing is not fatal: the other's findings are real evidence, and holding every promotion because a registry login flaked would rebuild the stuck gate this replaced. What the verdict must not do is imply a scan that never happened, so it names the scanners it actually read (`scanned by Trivy and Grype`, or just one of them). For the same reason the promote job runs even when a scan job failed — a lost matrix leg holds its own candidate, not everyone else's.

**A held candidate is information, not a failure.** It is annotated on the run and listed in the summary under *Held by the scan gate*, and the monitor stays green. Red is reserved for what is actually broken — a catalogue image deleted upstream (#245) — so that signal keeps meaning something. The candidate returns next week, with the current pinned image still scanned and still running in the meantime.

Both scanners' JSON is parsed, so a finding only one of them knows about still holds the candidate; running two scanners buys nothing otherwise. Severity filtering happens in code because Grype reports every severity whatever `--fail-on` says.

### What this costs

Security fixes reach the catalogue up to two weeks later than upstream publishes them, unless rule 3 applies. That is the deliberate price of rule 2. If that lag ever hurts more than it helps, the honest fix is to shorten the window — not to quietly bypass it.

## Future Enhancements

- Override flags: `--force-production`, `--force-dry-run`
- Pipeline-level `require_ci`
- Custom local behaviors per environment
- Validation warnings for risky operations
