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

## Supply-Chain Policy

This governs how the third-party artefacts CIDX itself depends on are pinned and updated. There are two classes, and the same three rules apply to both:

- **The built-in preset catalogue** — the container images CIDX runs. Everything up to [What this costs](#what-this-costs).
- **This repository's own CI** — the GitHub Actions its workflows call, and its Go modules. See [The same rules, applied to GitHub Actions](#the-same-rules-applied-to-github-actions).

Neither is imposed on projects using CIDX. You pin whatever you want in your own `cidx.toml`, and the workflows `cidx generate github` writes for you are yours to harden or not — per guardrail 5, CIDX is an execution engine, not a governance framework.

The reverse does bind the generators, though: what CIDX writes for someone else has to run on their runners. A generated workflow therefore never references a Docker Hardened Image, however clean it measures — `dhi.io` answers `401` to anyone without an entitlement, and the catalogue's Go image is only usable here because this repository's own CI holds credentials for it (#288). `cidx generate gitlab` bootstraps on the Docker Official `golang:<version>-alpine`, pinned by digest, from the same version constant the GitHub generator hands to `actions/setup-go`.

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

### At equal tool and version, take the smaller image

The three rules govern how a reference is pinned and when it moves. They say nothing about which reference to pick in the first place, and that choice turns out to dominate the numbers.

`golangci-lint` was pinned to the publisher's default image, built on Debian. Moving to `-alpine` — same publisher, same build, same version of the same tool — took it from **604 HIGH / 23 CRITICAL to 98 HIGH / 0 CRITICAL** (#280). 515 of those findings were never in golangci-lint at all: they were in a base image the linter does not use, shipped along with it.

So: **at equal tool and equal version, prefer the smallest variant the publisher offers** — `alpine`, `slim`, `distroless`, in that order of preference where they exist — and check at the moment of choosing, not later. A finding that is not in the image cannot be scanned, triaged, excepted, or re-argued in ninety days; the cheapest exception is the one never written.

Two caveats, both real:

- **The variant has to actually work.** A distroless image with no shell breaks any preset whose command is a pipeline, and an alpine image is musl, not glibc — a tool that ships a glibc binary will not run in one. The variant is a candidate, not an automatic winner; `cidx run <preset> --dry-run` and one real run are what settle it.
- **The variant line is a commitment.** Moving between families later is a repin by hand, never a promotion (see [A variant line that froze](#a-variant-line-that-froze)), so picking `-alpine3.21` means watching whether that family is still published.

### Applying it to the rest of the catalogue

Every catalogue image was scanned against every smaller variant its publisher offers at the same version (#286). One moved, and the reasons the others did not are the more useful half of the result.

**What moved.** The Rust pack went to `rust:1.97.0-slim`: **429 HIGH / 60 CRITICAL → 78 / 5**, and 596 MB → 323 MB. Same publisher, same toolchain, the same glibc — Debian minus the packages a compiler never reads.

The version does not move, so rule 2 has nothing to hold: 1.97.0 is the release the catalogue already runs, and `-slim` has carried it since the day it shipped. What is 13 days old is that tag's _content_ — the official images rebuild every variant when their base does, and `-slim` was last rebuilt one day later than the default. Rule 3 covers the remaining day and says so out loud: the image being left behind carries 60 CRITICAL findings that affect us today, against a hypothetical compromise in a rebuild of a release the catalogue already trusts.

**Why not `-alpine`, which measures 0 / 0.** Because alpine is musl, and these presets _compile_. Every artefact `cargo-build` produces would silently change libc: the binary would stop running anywhere glibc is expected, and nothing in the config would say so. The catalogue has already been bitten from the other side — `probatum` carries a description explaining that a glibc binary will not start on its musl image (#195) — and flipping the producer instead of the consumer is the same trap with the arrow reversed. The rule reads _at equal tool and version_; an image that changes what the tool emits is not equal. **For anything that compiles, or whose output is consumed elsewhere, `slim` is the floor.** A self-contained linter or scanner that emits nothing linkable can take alpine, as `golangci-lint` did.

**Where the smaller variant lost.** A variant is a candidate, not a winner, and three of them measured worse or equal:

| Image            | Default | Smaller variant                           | Verdict                                                              |
| ---------------- | ------- | ----------------------------------------- | -------------------------------------------------------------------- |
| `ruff:0.8.2`     | 0 / 0   | `-alpine` 21 / 2, `-bookworm-slim` 38 / 7 | Default is distroless and already the smallest — kept                |
| `prettier:3.9.4` | 0 / 0   | `-alpine` 0 / 0                           | Nothing to gain — kept                                               |
| `kaniko:v1.28.0` | 7 / 0   | `-slim` 3 / 0                             | `-slim` drops the ACR/ECR/GCR credential helpers — not the same tool |

`ruff` is the case the rule exists to catch in both directions: the publisher's default is a scratch image, and reaching for `-alpine` out of habit would have _added_ 21 findings. Measure, then choose.

The remaining images publish no smaller variant at their pinned version (`commitizen`, `commitlint`, `gitleaks`, `black`, `goreleaser`, `gh`, `gosec`, `shellcheck`, `probatum`, the Ansible dev-tools image), or are Docker Hardened Images that are already the minimal build (`dhi.io/*`). `dhi.io/golang:1.23-alpine3.21-dev` carried 340 HIGH / 23 CRITICAL, the catalogue's second-worst, and no smaller variant would have helped: it was the frozen variant line of [the section below](#a-variant-line-that-froze), and getting out of it took a base-version decision, not a variant choice.

### When no variant is the answer: `cargo-audit`

`cargo-audit` was the one Rust preset the sweep above left on the full image. `rust:<version>-slim` ships no HTTP client — the official Dockerfile installs `wget` to fetch rustup and `apt-get remove`s it in the same layer — and the preset downloads the RustSec release binary rather than `cargo install`ing it, which #161 measured in minutes and #188 fixed for non-root. On `-slim` it failed with `sh: curl: not found`.

That the rule had no move to offer was the tell. The question was never which variant of the Rust image to take: auditing a `Cargo.lock` reads a text file against an advisory list — no compiler, no crates, no toolchain — and one preset was holding the catalogue's worst image, **179 HIGH/CRITICAL findings against 415 for the other twenty put together**, in order to run a binary it downloads anyway (#287).

So it left the family rather than the variant: `buildpack-deps:trixie-curl`, **179 → 41**, and the catalogue 594 → 456. That is the Docker Official base `rust:` is itself built on, minus the toolchain, carrying exactly what the preset's one line needs — `sh`, `curl`, `tar` and CA certificates. The command is unchanged but for one flag. **The rule generalises: before comparing variants, ask whether the tool needs that image at all.** A preset that fetches its own binary is coupled to a libc and a shell, not to a toolchain, and the second question is much cheaper to answer than the first.

**Why not alpine, which measures 0 / 0 at 4 MB.** RustSec publishes a musl asset for `x86_64` only; `cargo-audit-aarch64-unknown-linux-musl` is a 404. A musl base would buy the last 41 findings by breaking every aarch64 user — [#195](#a-variant-line-that-froze) with the arrow reversed, and the second time this catalogue has refused alpine over libc. glibc keeps `$(uname -m)-unknown-linux-gnu` resolving on both architectures, exactly as the preset already did; the run was verified under `linux/arm64` as well as `linux/amd64`, detecting RUSTSEC-2021-0139 on each.

**What it cost: the yanked-crate check.** cargo-audit shells out to `cargo -V` to learn which crates.io index protocol to use, so with no toolchain in the image it reports `couldn't update crates.io index: registry: No such file or directory`. The preset passes `--no-yanked` rather than print that on every run — a yank is not an advisory, the RustSec scan the preset exists for is untouched, and a project that wants the check back overrides `command` in its own `cidx.toml`. What the preset does **not** need is `git`: the release binary fetches the advisory database itself, measured on an image carrying no `git` at all.

**What it gives up in exchange.** `buildpack-deps:trixie-curl` carries no version in its tag, so `cidx preset scan-targets` offers it no candidate — the same position `koalaman/shellcheck:stable` is already in. The digest keeps being scanned every week; what stops is automatic promotion, because a Debian suite is not a version to compare. Moving `trixie` on is a repin by hand, like the variant lines above.

That was written as a consequence accepted in advance, and for six weeks it was not what the code did: both images were being offered a candidate, and `trixie-curl` was offered an Ubuntu development branch (#328). Both are now reported as [a tag that carries no version](#a-tag-that-carries-no-version), which is what this paragraph had assumed all along.

### How the rules are applied

`cidx preset scan-targets` decides, per image, what `container-monitor.yml` scans and which candidates are old enough to consider; `cidx preset scan-verdicts` then decides which of them the scan results allow. The workflow only reads those verdicts — the policy lives in code, where it is testable, rather than in shell scattered across a YAML file.

**Where the age comes from.** The cooldown is measured against the date the registry reports for the candidate tag, taken from the same call that finds the tag, so it costs no extra request:

| Registry   | Date used                 | Meaning                                                |
| ---------- | ------------------------- | ------------------------------------------------------ |
| Docker Hub | `last_updated` on the tag | when that tag last received content                    |
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

**That rule now governs every registry, and did not always.** Docker Hub and Quay.io had been reached first and kept their own selection: a bare semver regex over a listing ordered by push date, first match wins, with the variant family standing in as a hardcoded list of seven suffixes — `-alpine`, `-slim`, `-bullseye`, `-bookworm`, `-buster`, `-jammy`, `-focal`. Anything else was read as no variant at all. So `buildpack-deps:trixie-curl` — Debian 13, and the base [`cargo-audit` runs on](#when-no-variant-is-the-answer-cargo-audit) — was offered `buildpack-deps:26.10`, an **Ubuntu development branch**, past the cooldown and ready to promote (#328). Three things had to be wrong at once, and all three were: `-curl` was not on the list, `trixie` is not a number so the pin looked unversioned, and `buildpack-deps` publishes Debian codenames and Ubuntu release numbers in one namespace, where the newest number is not the newest version of anything in particular.

The listing shape is per registry; the choice made from it is not. Both paths now return the same `(names, dates)` and hand it to the same comparison — which is what the rule above had always claimed, and what its tests had only ever exercised on one of the two routes.

**A newer version is not always a candidate.** ghcr.io and dhi.io date nothing:

- ghcr.io's dates live in the GitHub Packages API, which needs a `read:packages` token and answers 403 for a package owned by another organisation.
- dhi.io has no repository on `hub.docker.com` to ask, and its registry response carries names only.

Reporting versions found there as candidates would be worse than reporting nothing: the cooldown is fail-closed, so each one would be held in every weekly run from now until someone acted on it by hand — noise that never resolves. They are reported in a state of their own instead, `newer_version` with a reason saying the registry publishes no date, and the workflow summary lists them under **Newer version, not promotable automatically**. Pinning one is a deliberate act with a human behind it.

### A release that has not happened yet

A pre-release usually says so in its name — `-rc1`, `-beta`, `-nightly` — and the family rule above already refuses every one of them without knowing what any of those words mean: a candidate has to carry the pinned tag's suffix verbatim, so a marker the pin does not have cannot get through. A list of pre-release words would be unreachable code.

The channel that gets through is the one carrying no marker at all. A calendar-versioned distribution names a release by the month it is **due** and publishes images for it throughout its development: on 2026-07-30, `buildpack-deps:26.10` was "Ubuntu Stonking Stingray (development branch)" and would not be a release until October. Nothing in the name says so. The tag is pushed weekly like any other, so the cooldown ages it exactly like a release — and to an image pinned `24.04-curl` it is a perfectly well-formed successor: same variant, same precision, larger number.

The calendar is the one thing that answers, and it answers without a request: **a candidate whose version reads as a year and month later than the current month is not offered.** `26.10` is refused in August 2026, `26.04` is not, and `26.10` becomes offerable of its own accord in October — the rule reads a date rather than keeping a list of development branches, so nothing has to be edited when one ships.

It applies only where the **pinned** tag is itself calendar-versioned, which is what proves the repository numbers its releases that way: two components, a two-digit month in 01–12. Without that guard a tool sitting at `26.1` would see its own `26.10` read as October and refused; with it, `v2.95`, `0.71` and `3.24` are compared as the plain versions they are. This is the narrower half of the pair — the wider half is that a Debian suite is not offered an Ubuntu number at all — and it is the half that survives a repin onto `buildpack-deps:26.04-curl`, where the family rule would have nothing left to say.

### A tag that carries no version

`buildpack-deps:trixie-curl` and `koalaman/shellcheck:stable` are names, not versions. No tag a registry lists can be shown to be newer than a name, so the whole promotion path — which compares versions end to end — has nothing to say about these two images, and never will. Their updates arrive as **rebuilds of the same tag** under a new digest, and nothing here sees that: the cooldown, the family rule and the candidate all read tag names.

That is a real blind spot, and it is stated rather than papered over. `cidx preset scan-targets` reports `unversioned_tag` with the reason, and the workflow summary lists it under **Tag carries no version**, deliberately away from **Current (no updates)** — being unwatchable and being current are different facts, and this repository has twice been caught by the second hiding the first ([the deleted images](#a-pinned-image-that-vanished), [the frozen variant lines](#a-variant-line-that-froze)). Both images keep being scanned every week at the digest they are pinned to; what they never get is a candidate.

It is not annotated as a warning. This is a standing property of the pin, not an event: it would fire on every run from now until the pin changes, and a weekly alarm that cannot resolve is one nobody reads. Detecting the rebuild itself — comparing the digest a tag resolves to against the digest the catalogue pins — is a different mechanism from anything the promotion path does today, and it is the open half of the rebuild-versus-new-version question raised when the cooldown was designed.

### A pinned image that vanished

Rule 1 makes a reference immutable; it does not make it eternal. Two catalogue images — `dhi.io/alpine-base:3.21` and `dhi.io/docker:27-cli` — were deleted upstream and answered 404, and nothing noticed until the presets using them failed to start (#244).

`cidx preset scan-targets` now resolves the exact reference each catalogue image is pinned to, digest included, and marks it `missing` when the registry says it does not exist. `container-monitor.yml` annotates the run with an error and fails its summary job, so the weekly run goes red. A 401 from a registry we hold no credentials for is reported as an unverified image, never as a deleted one — the loudest signal the command has must not cry wolf.

### A variant line that froze

A reference can also stop receiving fixes without ever answering 404. `dhi.io/golang:1.23-alpine3.21-dev` and `dhi.io/python:3.13-alpine3.21` still pulled, and inside their own variant family they were genuinely up to date — because DHI publishes no `alpine3.21` tag at all any more, having moved to `alpine3.23` and `alpine3.24`. No successor will ever appear in the pinned family, so the family comparison correctly offers nothing and the catalogue silently sat on an abandoned line (#252). Same rot as the deleted images above, one step quieter.

`cidx preset scan-targets` reads the version the variant suffix itself carries — `-alpine3.21-dev` is version 3.21 of the `-alpine…-dev` line — and reports `frozen_variant` when the repository lists **no** tag in the pinned family while publishing a newer one. A family still published, even sitting at its own head, is alive and says nothing; the check costs no extra request, since it reads the listing update detection already fetched.

It is deliberately not a candidate. Moving from `alpine3.21` to `alpine3.24` changes the base image, which is a decision, not a version bump. The workflow summary lists it under **Frozen variant line** with a warning annotation, and repinning stays a human act.

**Getting out of one.** Both lines were repinned by hand: `dhi.io/golang:1.23-alpine3.21-dev` → `1.26.5-alpine-dev` (340 HIGH / 23 CRITICAL → 0 / 0) and `dhi.io/python:3.13-alpine3.21` → `3.13.14-alpine-dev` (34 / 3 → 0 / 0).

The base did **not** go to the `-alpine3.24…` family the report names. `frozen_variant` answers "what replaced the dead family", which is the honest reading of the tag listing; it is not a recommendation, and taking it as one would pin the catalogue to Alpine 3.24 until that freezes too. The unversioned line — `-alpine-dev`, `-alpine` — names the family rather than one base of it: DHI republishes it against each new Alpine, so it is the one shape that cannot go stale the way 3.21 did. It also cannot be _reported_ frozen, because a suffix carrying no version supersedes nothing, and that is correct rather than a blind spot. What the tag stops saying is which Alpine is underneath; the digest still says exactly what the content is, which is what rule 1 asks of it.

The language version moved with it, and that was the point of the exercise: `1.23` was three Go releases behind the `go 1.26` this repository compiles with, far enough that `govulncheck` could not analyse its own project from that image. DHI keeps one patch per minor, so `1.26.5` and `3.13.14` were the only tags on offer in their lines — there is no slightly older, equally clean version to prefer here, as there was in #277 and #280.

Python moved off the non-dev line at the same time. The minimal build ships no shell, and all six Python presets are a `sh -c 'pip install <tool> && <tool>'`: none of them had been able to start since the day they were written. "The variant has to actually work" is the rule that decided it, settled by a real run rather than by the finding count, which was 0 / 0 either way.

**On the cooldown.** dhi.io publishes no date for its tags, so rule 2 cannot be measured against these images at all — which is precisely why the promotion is a human act and not something `container-monitor.yml` performs. Rule 3's waiver is stated in its place, as the rule requires: the images being left behind carry 374 HIGH / 26 CRITICAL findings between them, on file in `known-vulnerabilities.toml` and demonstrably affecting us today, against a hypothetical compromise in a tag whose publication date nobody can read.

**How rule 3 knows what affects us.** `known-vulnerabilities.toml` already records the HIGH/CRITICAL findings accepted against the images the catalogue runs today — that is the list of vulnerabilities demonstrably affecting us, produced by the security audit. A candidate replacing an image with entries in that file is promoted without waiting, and the promotion PR names them. No second scan is run to obtain this: the current image's vulnerabilities are on file, and the monitor already scans the candidate.

A candidate that has served the full 14 days claims no waiver, even when the running image is vulnerable — a waiver line in the PR means the cooldown was actually bypassed, or the record stops being worth reading.

That record only works while it points at what we actually run. Entries were keyed `repo:tag`, so every promotion left the ones recorded against the replaced version behind: they stopped matching, which is correct, and then nothing said so — 138 of the file's 155 entries were keyed to tags the catalogue had passed, and rule 3's waiver had gone quiet with them (#248). Keying by repository ([below](#an-exception-dies-with-its-cve-not-with-a-tag)) removes the case a promotion creates; `cidx security vuln list --stale` lists what is left, the entries whose repository the catalogue no longer runs at all.

**The catalogue, and only the catalogue.** These rules govern the built-in preset catalogue. `cidx preset scan-targets` therefore reads `pkg/presets/presets.toml` rather than the resolved preset registry, which also carries whatever the user and the project declared in their own `presets.toml` — images the policy does not govern and the promotion job could not update anyway (guardrail 1, #248).

### A base that stopped being supported

The two sections above are about a _reference_ going quiet: one deleted, one whose variant family stopped being published. There is a third, and it is wider than either, because it is a property of the **base** rather than of the tag.

An image whose distribution has reached end of support receives no further security updates. Its packages are frozen at whatever they were on the day support ended, so the findings the scanners report on it are permanent — not "not fixed yet", but never. The tag can be current, the digest can resolve, the cooldown can promote it on schedule, and none of that changes the answer. Every question the [triage](#judging-a-finding) asks is downstream of this one: "is a fix available?" has no meaning for a base nobody is fixing.

The catalogue is in this state today, and nothing said so. `ghcr.io/probatum-org/probatum:0.2.1` is built on Alpine 3.20, whose support ended on **2026-04-01**. It pulls, it scans, it reports zero accepted findings — and it will not improve again.

**Where the base comes from.** Trivy already reports it, in `Metadata.OS` — `{"Family": "alpine", "Name": "3.20.10"}` — on every image it scans, and `security-audit.yml` scans every catalogue image daily. The field was being discarded. Nothing new is pulled, scanned or authenticated to obtain this.

**Where the dates come from.** [endoflife.date](https://endoflife.date), one request per distinct OS family — three for a catalogue of twenty-two images. The families the catalogue runs are mapped explicitly:

| Trivy family | endoflife.date product | Images |
| ------------ | ---------------------- | ------ |
| `alpine`     | `alpine-linux`         | 13     |
| `debian`     | `debian`               | 4      |
| `fedora`     | `fedora`               | 1      |

Only Alpine needs translating: endoflife.date files it under `alpine-linux`, and `/api/alpine.json` is a redirect rather than a document. The other two match their own names — and the map holds them anyway, rather than falling back to the family name, because **an identity default is what turns a fail-closed check into a wrong answer.** With one, every unmapped family would look like a valid product and produce a 404 that is indistinguishable from an outage.

A version is matched to a release line component-wise: Alpine `3.20.10` belongs to `3.20`, Debian `13.6` to `13`, Fedora `44` to `44`. The dot boundary is the whole of that rule — a plain string prefix files `3.20.10` under `3.2`, a line that ended in 2017, and the report would call a supported base long dead.

**Fail-closed, once more.** A family this code does not map, a version no published line covers, or a line endoflife.date announces no date for, all report `unknown_base` and annotate as an error. None of them reads as "supported". This is the same posture as an unresolvable digest (rule 1), an undatable candidate (rule 2) and an unreadable scan (the scan gate): the check refuses rather than assumes, and an unrecognised family is a gap in _our_ mapping, one line away from being closed. Treating it as "nothing to worry about" is how a blind spot becomes permanent.

An image with no distribution underneath it at all — kaniko, ruff and shellcheck are scratch or static builds, and Trivy reports no `Metadata.OS` for them — is a separate answer, `no_base`. A base that does not exist cannot stop being supported, and filing it with the ones nothing could resolve would manufacture three permanent false alarms.

**The threshold is 90 days.** The two ends of the range are the argument. An end of support two years out is a fact about the calendar: nothing is decided by knowing it, and printing it daily is how a section teaches its reader to skip it. Two months out is too late to be comfortable — getting off an abandoned base is a repin by hand, and the escapes in [the section above](#a-variant-line-that-froze) moved a base version, a variant line and a language version at once. A quarter is the smallest window that leaves room to schedule that work rather than rush it, and it survives the slowest loop that could act on it: `container-monitor.yml` runs weekly, so it is roughly thirteen chances to notice, and the audit that actually reports it runs daily.

**An outage never fails anything.** endoflife.date is a third party, and this check sits on top of a scan that already happened. When it does not answer, every base is reported `unchecked`, with the reason, once rather than per image — and nothing downstream changes its verdict. A scan, a monitor run and an audit all complete exactly as they did before. The one thing that must never happen is the outage reading as good news, so `unchecked` is stated rather than silently omitted.

**What it does not say.** It is not a recommendation, for the same reason `frozen_variant` is not: which base to move to, and whether the tool still works on it, are decisions with a human behind them. It says nothing about _why_ a base is old — an image can sit on a supported base and still be abandoned, and a fresh base is not evidence that anything else about the image is maintained. And it is not a gate. A base past its date annotates as an error and reports; it does not fail the audit, because red there means "an unhandled vulnerability", and a repin decision is the same class as the frozen variant line that warns rather than breaks. The audit that cries wolf is the one nobody reads.

**Where it lives.** `cidx security baseline`, which already reads exactly these scan results for exactly these images, and `security-audit.yml`, which runs it daily and turns the verdicts into run annotations. Deliberately _not_ `cidx preset scan-targets`, where `missing` and `frozen_variant` live: the base is only knowable from a scan result, and that command runs before anything has been scanned. The monitor's population is also the wrong one — it scans candidates, whose base is provisional until they are promoted.

`SECURITY-BASELINE.md` therefore records the **base** and not the date. The base is a fact about what we ship and changes only when we change it, so it belongs in a committed file whose diff is the point. The date support ends is relative to the day it is read and comes from a third party: it would move those lines without anything changing about the catalogue, which is the same reason that file carries no generation date. The countdown is printed, not committed.

### What we are actually defending against

Every judgement below rests on this. Without it, no finding can be called irrelevant, and the only available answer to any CVE is to patch it — which is how a catalogue ends up churning on findings nobody could have exploited.

A CIDX container:

- **lives for seconds**, the length of one phase, then is gone;
- **exposes no service** — nothing listens, nothing routes to it, no attacker sends it anything;
- **reads the repository's own code**, not input from unknown users;
- **persists nothing** — no database, no state that survives the run;
- **holds no durable secret.** The exception is the CI token, in scope only for the presets that publish (`gh-release`, `twine`, `kaniko`, the registry ones).

So the realistic threat is a **compromised image**: an artefact that runs code we did not expect, on our source, with our token in reach. That is the threat rules 1–3 address, and the one no CVE scanner detects — a backdoor has no CVE until someone finds it.

A vulnerability in a package sitting on the disk of a linter is a different thing entirely. `libcrypto` inside `golangci-lint` is a TLS implementation that nothing calls; a parser flaw in it is unreachable because no attacker-controlled bytes ever arrive. This is not an argument for ignoring findings — it is the difference between the ones that matter here and the ones that would matter in a public-facing server.

### Judging a finding

Four questions, in this order. The first that answers decides.

**1. Is it being exploited?** Presence in [CISA KEV](https://www.cisa.gov/known-exploited-vulnerabilities-catalog) means somebody is using it right now — act immediately, whatever the severity says. A high [EPSS](https://www.first.org/epss/) score (the probability of exploitation in the next 30 days) means examine it now rather than at the next cycle. Severity alone answers neither question: it describes what a flaw could do, not whether anyone does it. Measured on this catalogue: **zero KEV, highest EPSS 0.10, 95% below 0.02** (#238).

**2. Is a fix available?** This splits the work in two, and the two halves need opposite treatment:

- **Fix exists** → nothing to decide. The finding disappears when the publisher republishes, so the question is the image's age, not the vulnerability. **Never write an exception for one of these** — it would record a decision where there is only a wait.
- **No fix at any version** → the only case where an exception is the right instrument.

The split is real and it is per-image, not per-severity: `commitlint` is 100% fixable, `gitleaks` 98%, `ansible-dev-tools` 93% — pure image-freshness. `rust:1.97.0-slim` is 20% fixable — genuinely exception territory.

The scanners say so directly, in a field each: Trivy's `FixedVersion` and Grype's `fix.versions`. `cidx security vuln add` reads them and **refuses** to record an exception for a vulnerability that has a fix, naming the version and pointing at the repin. There is no `--force`, because the correct action is not one that command performs. With no scan result for the image it says so and writes the entry: refusing on missing evidence would make the command unusable away from the audit's artifacts, and nothing is being promoted or deleted — the entry is argued again at its expiry. `cidx security vuln prune` names the entries already on file whose CVE turns out to have a fix, under **Fixed upstream**, and removes none of them: until the repin happens the entry still waives a finding the audit would otherwise fail on.

**Where that answer comes from for an entry still on a running repository.** From what the scanners recorded as suppressed, not from what their reports show (#312). `security-audit.yml` builds each image's ignore file from the entries accepted on that image's repository, so a live entry removes its own CVE from its own scan results by construction — asking `Vulnerabilities` or `matches` whether it has since been fixed can only ever return silence. Both scanners _move_ a suppressed finding rather than dropping it, fix version included: Grype into `ignoredMatches`, Trivy into `ExperimentalModifiedFindings` under `--show-suppressed`. Reading both is what closed the gap #312 left open: until #311 added the flag the answer was Grype's alone, and a fix only Trivy reported left no trace. An entry absent from **Fixed upstream** still means nobody said, never that no fix exists — the remaining blind spot is #315's, where a fix is recorded under an alias of the identifier the entry is keyed by.

**3. Is it reachable here?** Read the CVSS vector against the threat model above, not in the abstract. `AV:N` means the attack path crosses a network _if something feeds it hostile input_; in a linter that opens no socket, it means "would be network-reachable in a server", not "is reachable in this container".

Two classes are unreachable by construction and are exempt as classes, not case by case:

- **Go standard library compiled into a CLI binary.** These vanish when the publisher recompiles, no action exists at our end, and the paths flagged (`net/http`, `crypto/*`) are unreachable in a tool that opens no listener.
- **`linux-libc-dev` and kernel headers.** The kernel is the host's; it is not in the container. The scanner flags the headers package because it carries a version string.

Together these were ~20% of findings and ~0% of risk (#238).

**What exactly is excluded.** An exemption that is too broad hides findings that matter, so both tests are narrow and read off the scan results rather than off a list of CVEs:

| Class          | Test                                                                                 | What it does **not** catch                                                         |
| -------------- | ------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| Go stdlib      | package named exactly `stdlib`, in a Go binary (Trivy `gobinary`, Grype `go-module`) | every other Go module in the same binary                                           |
| Kernel headers | package named exactly `linux-libc-dev` or `linux-headers`                            | `util-linux`, `linux-pam`, and anything else whose name merely begins with `linux` |

The second column is the point. The runc and containerd findings on the Ansible image are `gobinary` too — an exemption keyed on the ecosystem alone would have swallowed a container escape. And a finding the two scanners disagree about is **not** exempt: Trivy filing a CVE against `linux-libc-dev` while Grype files the same CVE against `openssl` is a reason to look, not a reason to skip. A finding is exempt only when every instance of it on that image is exempt under the same class; a fix, by contrast, counts as soon as one scanner reports it, since a fix does not stop existing because the other missed it.

These findings leave the triage queue; they do not leave the record. They are counted and named in `SECURITY-BASELINE.md`, where they read as what they are — a signal about the images' age.

**KEV and EPSS are carried, not thresholded.** Grype reports EPSS on every match and `kev` only for a vulnerability CISA lists; Trivy reports neither. Both travel through to the baseline, which prints the highest EPSS seen and names any KEV entry. Neither gates anything: they answer question 1, and question 1 is a judgement.

**4. What does it cost to remove?** Only now. In increasing order: accept and document → wait for the rebuild → bump or replace the image → **disable the preset**. The last rung has to exist explicitly: without it, a preset whose image is genuinely dangerous gets kept because nothing else was on the menu.

Paranoia is not a policy. Patching every finding on sight costs review attention that the real threat — a compromised image — then does not get.

### An exception dies with its CVE, not with a tag

An exception is written with a date on it, so the acceptance gets argued again rather than inherited. It records a judgement about **a CVE in an image, in our usage** — and that judgement survives a version bump, because none of what it rests on changed.

Keying it to `repo:tag` was the original design and it was wrong. When the image was promoted, every entry stopped matching anything and stayed in the file for ever, because nothing asked whether it still had an object. All 155 entries reached that state, the most recent expiry dated 2026-03-02. Worse, the record silently lost track of what it covered: twelve kernel CVEs filed against `golangci-lint:v2.6.2` are carried today by the Rust images, and nothing connected the two (#238).

So an exception is keyed by **repository and CVE**; the tag it was first seen on is context, not identity. It dies when its CVE is no longer carried by any catalogue image, or when its expiry falls — never because a tag moved.

**"Or when its expiry falls" was not true until #303.** `cidx security vuln ignore` wrote every entry into the scanners' ignore file with no date check at all, so an acceptance five months past its date removed its finding from the audit's own results exactly as a live one did — and removed it _before_ the JSON was written, so nothing downstream could see it. All eighteen entries on file were in that state. The `expires` field was decorative, and it was the whole of the mechanism meant to force the acceptance to be argued again rather than inherited.

The date is now the test the ignore file is built from. Two edges of it are decisions rather than consequences:

- **The named day is included.** An entry expiring on 2026-03-02 waives its finding all through that day and stops on 2026-03-03 — "expires on" reads as a deadline, not as a cut-off. It is also the boundary the Security tab already used to call an entry expired, so the day an acceptance stops filtering is the day the tab says it lapsed, rather than the day before it. One definition rather than two a day apart: `Waives` and `ExpiredExceptions` share a parse, a UTC day and a test walking the week around the boundary.
- **A date that is missing or unreadable waives nothing.** Fail-closed, the same posture as an unresolvable digest (rule 1), an undatable candidate (rule 2) and an unreadable scan (the scan gate). An acceptance is a _dated_ judgement; with no readable date, nothing says it is still one somebody has taken. It is deliberately **not** reported as expired — an entry with no date never began rather than having lapsed — so `cidx security vuln check` names the malformed date as the warning it is, while the finding it stops hiding reaches the audit like any other finding. `cidx security vuln add` always writes a date, so this is a hand edit or nothing.

**Nothing is decided on anybody's behalf.** The eighteen lapsed entries stay in the file exactly as they were. Renewing one, re-arguing it or deleting it is an acceptance decision, and it belongs to whoever has to live with it — the same line `vuln prune` holds. What changed is only that they stop waiving until somebody takes it.

That is what an entry looks like now, and `first_seen` is the whole of what remains of the tag:

```toml
[[vulnerabilities]]
  cve = "CVE-2013-7445"
  repository = "rust"
  first_seen = "golangci/golangci-lint:v2.6.2"
  severity = "HIGH"
  status = "third-party"
  added = "2025-12-02"
  expires = "2026-03-02"
```

Two consequences fall out of the key, and both are the point. An exception written for `rust` covers every tag of it the catalogue runs — it covered `rust:1.97.0` **and** `rust:1.97.0-slim` while both were in, and all thirteen survived `cargo-audit` leaving the full image (#287) without one of them being re-filed, because the judgement was never about a tag. And a candidate the monitor proposes is a newer tag of a repository the catalogue already runs, so the scan gate honours the entries written for it without their being re-filed first; under the old key it did not, and promotions were held on findings reviewed months earlier.

**Migrating the 33 entries that were on file.** All of them were recorded against `repo:tag`, and all of them named a tag the catalogue had passed — yet their 19 distinct CVEs were every one of them still carried by a live image. A repository derived from the old key would have been wrong for most: twelve kernel CVEs filed against `golangci-lint:v2.6.2` are carried by the Rust images today, because the linter moved to Alpine and stopped shipping `linux-libc-dev` at all. So the old key is not converted, it is _classified_: a whole `repo:tag` equals no repository, so the entry falls through to the CVE test and lands as carry-over on whichever repository the findings say carries it. `vuln prune -x` re-files it there. Thirteen entries went to `rust`, twenty to `ghcr.io/ansible/community-ansible-dev-tools`, where they collapsed onto six keys — 33 entries became 19, and none of the justifications was lost.

The file is written by hand rather than by the TOML encoder, which re-indented every key it touched and turned a purge of 101 entries into 538 insertions and 1552 deletions (#289). Entries are sorted by repository then CVE, so the same content produces the same bytes and a removal reads as a removal.

The tempting rule is "the tag changed, so delete it". It is wrong, and expensively so. When an image is promoted, an accepted CVE does one of two things:

- **It went away with the image.** The exception has no object left. Delete it.
- **It followed the promotion into the new image.** Deleting it loses the justification somebody wrote and the review it came from, and the next audit goes red on a finding that was settled months ago. It has to be **re-filed** against the new reference, not purged.

Tags cannot tell those apart. The findings can, so the criterion is the CVE: **is it still present in an image the catalogue runs?** `cidx security vuln prune` answers that from the scanner results the audit and the monitor already produce — nothing is rescanned, because a command that pulled twenty images to answer a bookkeeping question would be run once and never again. Point `--results` at the JSON either workflow uploaded.

Every entry lands in one of four states:

| State          | Meaning                                                            | What happens to it            |
| -------------- | ------------------------------------------------------------------ | ----------------------------- |
| **live**       | covers a repository the catalogue runs, and its CVE is still on it | nothing, it is doing its job  |
| **carry-over** | its CVE is carried by another catalogue repository                 | re-filed onto that repository |
| **obsolete**   | no catalogue image carries its CVE any more                        | removable                     |
| **unknown**    | the evidence does not settle whether the CVE is still there        | reported, never deleted       |

**Where "is it still there?" is answered from.** Not from the findings the reports show, because those cannot answer it: `security-audit.yml` builds each image's ignore file from these very entries, so a CVE accepted on a running repository is removed from that repository's own results by construction. Its absence there means nothing.

For a long time the lifecycle worked around that by not asking at all — the repository match returned `live` before the findings were consulted. The consequence was not intended and it emptied the lifecycle of most of its content: a repin never changes the repository, so **an exception whose CVE had genuinely disappeared could never be retired by any mechanical path**, and the only thing that ever removed one from the file was its repository leaving the catalogue (#311). Proven by stripping the six ansible CVEs from a copy of the scan results: `vuln prune` still reported all six `live`.

What answers it is the record each scanner keeps of what its ignore file removed. Grype moves a suppressed match to `ignoredMatches`; Trivy does the same under `--show-suppressed`, which the audit passes on the JSON scan for exactly this reason — `Vulnerabilities` is unchanged, so every other reader of the artifact sees what it always saw. **What an image carries is the two lists read together**, and a CVE in neither is a CVE nothing carries. `container-monitor.yml` builds no ignore file at all, so its results show everything and suppress nothing; the same reading is correct there without knowing which workflow produced the artifacts.

No second scan was needed for this, and that was the point of looking: the evidence was already being computed and thrown away.

**Fail-closed on the evidence itself.** Trivy's report says nothing about whether `--show-suppressed` was passed — the field is simply omitted when a scan suppressed nothing — so a report produced without it is indistinguishable from one that hid nothing. Run against the audit's artifacts from the day before the flag landed, the new reading called four ansible entries obsolete; every one of them is still carried, and `prune -x` would have deleted the exceptions waiving them. So an absence only counts when something, somewhere in the results, was recorded as suppressed. The flag is per workflow run, so one sighting settles it for the directory; with none, every absence reads `unknown` and the report says which evidence is missing and where to get it.

**`live` is not the same as "nothing to look at".** An acceptance is never the right answer to a CVE that is fixed upstream, and a fix can land long after the entry was written — so a live entry with one is an entry that should not exist: a repin candidate, not a renewal. It stays live, it stays on file, and it is listed under **Fixed upstream** as well, read from the same suppressed record ([above](#judging-a-finding)). Nothing about the four states changes; the report just stops being silent about the one case where it matters most (#312).

**Fail-closed, once more.** A CVE cannot be shown absent from an image nobody scanned, so a missing result makes the verdict `unknown`, not `obsolete` — the same posture as an unresolvable digest (rule 1), an undatable candidate (rule 2) and an unreadable scan (the scan gate). A repository counts as scanned only when _every_ image the catalogue runs from it produced a result: with two tags of `rust` in the catalogue, one answering is not an answer.

**And it only reports.** `vuln prune` prints; `vuln prune -x` writes. What it writes is mechanical either way — an obsolete entry removed, because nothing carries its CVE and it therefore waives nothing; a carry-over entry re-filed, because it waives exactly what it always did, against the repository that turned out to carry it. Neither is an acceptance. Unknown entries are left alone: that is a question nobody has answered. The convention is `repo branch cleanup`'s: the default run is the one that changes nothing. Deciding to stop waiving a CVE — or to accept one in the first place — belongs to whoever has to live with it. The tool prepares the material and names what it found; the human decides.

### What the catalogue actually ships

`known-vulnerabilities.toml` is a working record: written for the audit rather than for a reader, and saying nothing at all about the findings nobody has accepted. Nobody installing CIDX could tell from it which vulnerabilities the images they are about to run actually carry.

`cidx security baseline` writes that down, in `SECURITY-BASELINE.md` at the root of the repository: every image the built-in catalogue runs, pinned by digest, with the presets using it, and every HIGH/CRITICAL finding accepted on it — with the reason and the expiry date. Only the built-in catalogue: a preset your own `presets.toml` declares is yours, and CIDX makes no claim about it (guardrail 1).

Three properties, each deliberate:

- **It is committed.** The diff is the point. A release that changes what the catalogue carries shows it as a changed line, and "what we ship by default" stops being something you have to reconstruct.
- **It carries no generation date.** A timestamp would change the file on every run and make every diff meaningless. The same inputs produce byte-identical output, and a test pins that — map iteration order has flapped this repository's output twice already (#230, #233).
- **It states what is carried and what is accepted, separately.** Those are different numbers, and publishing only the second is how the file came to read "0 accepted findings" while the catalogue carried 596 (#238). Accepted-but-unlisted would be a fiction; carried-but-unstated is the omission that made the fiction possible.

Carried comes from the scan results (`--results`, the same artifacts `vuln prune` reads) and is counted **per image**: the same CVE on five of them is five things to look at and five repins. It is published split the way [Judging a finding](#judging-a-finding) splits it, because 465 findings reads as a catastrophe and the number that actually needs a human is the last line:

| Population                | Count |
| ------------------------- | ----- |
| Go stdlib in a CLI binary | 40    |
| Kernel headers            | 69    |
| Fixed upstream            | 206   |
| **Needing triage**        | 150   |

**Accepted is a subset of carried, and counting it took #311's evidence.** The audit generates each image's ignore file out of the entries accepted on that image's repository, so an accepted finding is deleted from that image's own results by construction — and a file counting only what the reports show published what the catalogue carries _minus the part it had already argued about_. On the same artifacts that reads 447 where it should read 465. Both scanners keep the suppressed half now (Grype in `ignoredMatches`, Trivy under `--show-suppressed`), so the accepted identifiers are read back out of it and counted where they belong. Only the accepted ones: Grype's own default rule drops indirect `linux-libc-dev` matches, 188 of them on the Rust image alone, and adding those would move this number by a scanner's defaults rather than by anything the catalogue ships. On results recording no suppression at all the file says so and reads as a floor — the same posture as `unknown` above, applied to a count.

**A file nothing regenerates goes stale, and this one had.** It was written by a hand-run command and committed when someone remembered, so it sat months without the `Base` column, listing images the catalogue had repinned past, and releases attached it as an asset all the same (#310). Two halves, two answers. The half this repository decides — the shape, the images, the presets on them, the acceptances published against them — is guarded by `TestSecurityBaselineIsCurrent`, which regenerates it offline and fails any change to the catalogue the committed file was not regenerated for; that is what the determinism above was built to make possible. The other half is measured by scanner databases that move without any commit here, so no test can hold it: a gate needing the audit's artifacts would be red whenever a CVE was published and green whenever the download flaked, which is what a decorative check is made of (#247). The daily audit reports that drift instead, in its summary, next to the artifacts that fix it.

The class exemptions are labelled before fixability, which inverts the order the questions are asked in, deliberately. The questions are ordered by what to _do_ — a fix exists, so wait, and stop asking. The table labels findings by _why they are out of the queue_, and the class is the stabler answer: a Go stdlib finding is unreachable whether or not the Go team has shipped its fix yet, and most of them have one, so labelling by fix first would report zero stdlib findings on a catalogue full of them. **Needing triage** is the same number either way, which is the number that matters.

An image with no scan result is printed `not scanned`, never `0` — an absent number is not a zero, and that confusion is the one this file exists to remove.

An entry past its expiry date is printed with that date and nothing else — the table states facts, and `cidx security vuln check` is what judges them.

### Where to look: the Security tab

Everything above produces evidence, and none of it was anywhere in particular. The daily audit's scan results lived in artifacts deleted after a day; the acceptances in `known-vulnerabilities.toml`; the totals in `SECURITY-BASELINE.md`; the reasoning in issue threads. Four places, and no one of them answers "where does the catalogue stand" without the other three (#301).

`security-audit.yml` now publishes to GitHub code scanning, so the Security tab is that place: **[github.com/cidx-org/cidx/security/code-scanning](https://github.com/cidx-org/cidx/security/code-scanning)**. It keeps findings across runs, dates them, and closes them by itself when a repin removes one — which is the part no artifact and no generated file was ever going to do.

**`known-vulnerabilities.toml` stays the source of truth. The tab is a view of it.** Nothing is accepted, waived or decided there; an alert is dismissed by editing the file and letting the next audit re-publish. That direction is the whole design, and it is why the alerts point where they do: a finding's alert is anchored to the line of `pkg/presets/presets.toml` that pins the image, and an expired acceptance's alert to its entry in `known-vulnerabilities.toml`. The Security tab links straight to the line to change.

**What is published is what needs a human — 128 alerts, not 465.** The catalogue carries 465 HIGH/CRITICAL findings and the tab exists to make the state readable in ten seconds, so publishing the raw scan would defeat the purpose it was built for. The populations [Judging a finding](#judging-a-finding) already answers are left out, and the same code answers them, so the tab and `SECURITY-BASELINE.md` cannot come to disagree about what a finding _is_. They do count different things on purpose: the baseline says what the images carry, acceptances included, while the tab shows what still needs a decision, so a finding an entry already waives is not an alert — it is [that entry's own row](#what-the-catalogue-actually-ships), and its alert is the expiry below.

| Population                | Published | Why                                                                                                         |
| ------------------------- | --------- | ----------------------------------------------------------------------------------------------------------- |
| Needing triage            | yes       | No fix at any version, not exempt. The only population an exception is the right instrument for             |
| Fixed upstream            | no        | The image's age, not a decision. `container-monitor.yml` is the loop that chases it                         |
| Go stdlib in a CLI binary | no        | Unreachable by construction                                                                                 |
| Kernel headers            | no        | Unreachable by construction                                                                                 |
| Expired exceptions        | yes       | An acceptance nobody has stood behind since its date, and it supersedes the finding's own alert (see below) |

That last row is the one the design turns on. It was added because nothing else could surface a lapsed acceptance: `cidx security vuln ignore` wrote every entry into the scanners' ignore file whatever its expiry date, so the finding it waived was suppressed before the JSON was written and **could not** appear as an alert. The only trace was an annotation on a workflow run nobody opens. So the view reads the file directly and says what the file says about itself. Re-dating an entry or deleting it closes the alert; that is the same act the record has always asked for, now with somewhere to see that it is pending.

**Since the date is honoured, the two can collide.** The finding a lapsed entry stopped filtering is back in the scan results, so it would raise a triage alert of its own — same repository, same CVE, two alerts. That is one decision asked about twice: one of them gets dismissed to clear the list, and which one is a coin toss. The expired alert supersedes the triage one, because it says strictly more — it carries the judgement somebody already wrote and the date it lapsed, and it points at the entry in `known-vulnerabilities.toml` rather than at the image pin, where the `cidx security vuln add` line the triage alert offers would be the wrong instruction for a CVE that already has an entry. Keeping that one also leaves the alerts a reader has been looking at since #301 exactly where they are, instead of closing eighteen and opening near-identical replacements — the churn #313 removed from repins, refused here for the same reason. A lapsed acceptance whose finding is exempt by class, fixed upstream or no longer carried has no triage alert to supersede and stands alone, as it always did.

Neither scanner's native SARIF is used, for that reason and not for a stylistic one: Trivy has no "only unfixed" switch, and neither tool knows what `stdlib` in a Go binary or `linux-libc-dev` means in a container that listens on nothing. `cidx security sarif` renders the JSON the audit already uploaded, reusing the triage in `pkg/presets/findings.go`. The scanners are not run a second time.

**One analysis for the catalogue, not one per image.** Code scanning tells uploads apart by `category`, and without distinct ones two uploads overwrite each other — so the alternative was 21 categories, one per image. It was rejected on what the reader gets: the alert list offers no way to filter by category, so 21 analyses would show as one undifferentiated list, while every one of them would have to be re-uploaded on each run to stay current. SARIF also caps a file at 20 runs, which rules out one run per image outright. What actually tells images apart in the UI is the alert: its rule identifier is `<repository>/<CVE>`, and its location is the line pinning that image. Both work inside a single run.

**The identifier and the fingerprint are both keyed by repository**, exactly as an exception is, and for the same reason — the judgement survives a repin. GitHub matches an alert across runs on the two of them together, so a pinned reference in either is enough to break it: the fingerprint carried the tag _and_ the digest until #313, which meant the first repin after this shipped would have closed every alert on that image and opened a near-identical one for each, including for CVEs that never went away. A burst of "fixed" alerts that fixed nothing is the exact opposite of the signal the tab was added for. The reference travels in the alert's message, where it is informative, and in its location — the line pinning that image, which a repin rewrites in place, so the link follows the promotion while the identity survives it.

What that costs is one alert per repository and CVE, whatever the repository is pinned to: two references of one repository, pinned at the same time, would publish two alerts sharing an identity and only one would be shown. The catalogue pins one reference per repository today, and `TestCataloguePinsOneReferencePerRepository` is what stops that being an assumption. Pinning two at once — as `rust:1.97.0` and `rust:1.97.0-slim` were until #287 — needs a disambiguator that itself survives a repin, which is a decision to take rather than a line to change.

**Both scanners, each finding once.** Of the 150 findings needing triage, 39 are seen by both scanners, 102 by Grype alone and 9 by Trivy alone. Publishing a run per scanner would show 189 alerts for 150 problems, a third of them twice; publishing Trivy alone — the tempting simplification, since it is the one with native SARIF — would hide two thirds of what needs attention. They are merged on the identifier instead, and each alert names the scanners that reported it. That is the same posture as [the scan gate](#the-scan-gate-is-differential), which already holds a candidate on a finding only one scanner knows about: running two scanners buys nothing if only one is believed.

**Permissions.** Uploading a SARIF needs `security-events: write`, and the workflow carries `contents: read` globally (#207). The write is granted to the `report` job alone: the scan jobs pull images and run third-party scanners on them, and none of that has any business being able to write this repository's security findings.

**What a branch run means.** Code scanning attaches an analysis to the ref it ran on, so triggering the audit from a branch — `cidx workflow run security-audit.yml --ref <branch>` — files the alerts against that branch, and the alert list defaults to the default branch. They arrive on `main` when the scheduled run next executes there, on the same 21 images.

### Where to look for what: the status page

The Security tab answers "which finding, on which image, since when". It does not answer the question that comes before it — how much is there, how much of it needs a human, what has already lapsed — and it cannot: it is a list of 169 alerts, and a list is not a count. The end-of-support signal above is one fact about one image. So there were two views of the detail and none of the whole (#308).

`cidx security summary` renders that whole, and `security-audit.yml` rewrites it into a tracking issue on every run: **[the vulnerability status page](https://github.com/cidx-org/cidx/issues?q=label%3Asecurity-status)**. Three places, and each answers one thing:

| Question                                   | Where                                      | What it is                                            |
| ------------------------------------------ | ------------------------------------------ | ----------------------------------------------------- |
| Where does the catalogue stand             | the status issue (label `security-status`) | the synthesis, rewritten by every audit               |
| Which finding, on which image, since when  | the Security tab                           | the detail, one alert per finding, dated              |
| Does this acceptance still stand           | `known-vulnerabilities.toml`               | the source of truth — everything else is a view of it |
| What the catalogue shipped on any past day | `SECURITY-BASELINE.md`, in `git log`       | the committed record, whose diff is the history       |

**It recalculates nothing.** The partition comes from the same `Summarise` the baseline and the SARIF renderer read, the expiry test from the same `ExpiredExceptions` the alerts read, the base verdicts from the same `ClassifyBase` the run annotations read. That is the whole design: a page disagreeing with the tab about what the catalogue carries would be worse than no page, and the only way two views cannot disagree is for them to be two renderings of one computation.

**One exception it does not restate: the cooldown.** "Which candidate is rule 2 still holding" is answered by `cidx preset scan-targets`, which reads the registries, from `container-monitor.yml`, which holds the credentials for the hardened ones. Recomputing it in the audit would mean a second answer from a job that would get 401s where the first got results — two sources for one fact, differing exactly where a reader would notice. The page names the monitor and links to its runs instead.

**One issue, rewritten, found by its label.** A daily audit filing one issue per run would produce 365 a year, which is the noise this page exists to replace. The label is the key rather than the title, because a title is editable and carries no counts for that reason; no match means the issue does not exist yet and it is created, and more than one match fails the step naming them, because rewriting the body of an issue somebody else's label happened to match is the one failure that would go unnoticed.

**No comment is ever posted.** The body is overwritten and yesterday's numbers are gone, deliberately. The history already exists twice: `git log SECURITY-BASELINE.md` is what the catalogue carried on any past day, `git log known-vulnerabilities.toml` is which acceptances stood, and code scanning dates every finding and closes it by itself. A comment per run would duplicate both. A comment on a threshold — "the triage queue crossed 200" — would need yesterday's numbers, which nothing in the audit keeps: that state would have to be invented before it could be compared, and it would be a worse copy of a diff that is already committed.

**It is markdown, and it also carries JSON.** The body is read by a human and, increasingly, by an agent asked where the catalogue stands. Prose is what the human wants and what a script has to guess at — the wording will change, and a consumer keyed on a sentence breaks the first time it does. So the counts are repeated once, at the end, as a flat JSON object in a collapsed block: sixteen keys that do not move, no findings (they have an API of their own), and one test asserting the block and the page are rendered from the same value.

**Permissions.** Rewriting an issue needs `issues: write`, granted to the `report` job alone, next to the `security-events: write` #301 already granted it. The workflow now also carries `permissions: contents: read` at the top level, which it had been documented as carrying and did not: without that floor the four scan jobs inherit the repository default, and a job-level grant restricts nothing.

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

**A held candidate is information, not a failure.** It is annotated on the run and listed in the summary under _Held by the scan gate_, and the monitor stays green. Red is reserved for what is actually broken — a catalogue image deleted upstream (#245) — so that signal keeps meaning something. The candidate returns next week, with the current pinned image still scanned and still running in the meantime.

Both scanners' JSON is parsed, so a finding only one of them knows about still holds the candidate; running two scanners buys nothing otherwise. Severity filtering happens in code because Grype reports every severity whatever `--fail-on` says.

### What this costs

Security fixes reach the catalogue up to two weeks later than upstream publishes them, unless rule 3 applies. That is the deliberate price of rule 2. If that lag ever hurts more than it helps, the honest fix is to shorten the window — not to quietly bypass it.

### The same rules, applied to GitHub Actions

A third-party action is the same class of risk as a third-party image, and for a while it got none of the same treatment (#249). It executes on our runners with the repository token in scope — `actions/checkout` handles that token directly — and `actions/checkout@v6` is a **moving reference**: it resolves to whatever the maintainers last pushed to that tag, which is exactly the mutability rule 1 removed from images.

**Rule 1 — pin by SHA.** Every `uses:` in `.github/workflows/` is written `org/action@<full-commit-sha> # vX.Y.Z`. The 40-character commit SHA is the digest equivalent: immutable, and unlike a tag it cannot be repointed. The trailing comment keeps the reference readable and is not decoration — Dependabot parses it, and keeps proposing updates in the same form. A short SHA is not enough; the full one is what GitHub's own hardening guidance asks for.

Resolving the SHA means dereferencing the tag, because some publishers use annotated tags whose ref points at a tag object rather than a commit:

```bash
gh api repos/OWNER/REPO/git/ref/tags/TAG --jq '.object.type + " " + .object.sha'
# if the type is "tag", the commit is one hop further:
gh api repos/OWNER/REPO/git/tags/SHA --jq '.object.sha'
```

The comment must carry the **precise** version the SHA is, not the major tag that happened to point at it — `# v6.1.0`, never `# v6`. A comment saying `v6` would be true forever and therefore say nothing.

**Rule 2 — wait 14 days.** Pinning by SHA freezes the reference but says nothing about when to move it, and Dependabot's default is to open the bump the day a version lands. `.github/dependabot.yml` sets the wait in the tool rather than in whoever reviews the pull request:

```yaml
cooldown:
  default-days: 14
```

`github-actions` supports `default-days` only; the `semver-major-days` / `semver-minor-days` / `semver-patch-days` keys exist for SemVer ecosystems and are deliberately unused for `gomod` too. The policy is one number. (Dependabot applies a 3-day cooldown of its own when the option is absent — enough to show the mechanism is the right one, nowhere near the window rule 2 asks for, and worth stating explicitly rather than inheriting.)

**Rule 3 — waive the wait for a real fix.** This one needs no configuration: `cooldown` governs _version_ updates only. A Dependabot **security** update is exempt by construction, so a version that fixes a vulnerability affecting us still arrives immediately. The mechanism differs from the catalogue's — there the waiver is a human line in the promotion PR — but the outcome is rule 3 either way.

**Why `gomod` gets rule 2 but not rule 1.** The Go checksum database already makes a published version immutable: `go.sum` records the content hash, and a republished `v1.2.3` fails verification instead of quietly substituting itself. Rule 1's problem does not exist there. What sumdb guarantees is that everyone gets the _same_ bytes — not that those bytes are benign, so a maliciously published _new_ version lands like any other release. That is the delay rule 2 buys, and why the cooldown applies to `gomod` as well.

**Scope, again.** This covers this repository's workflows. `pkg/generate/github.go` still emits major tags in the workflows it generates for other projects, on purpose: pinning by SHA is a policy, and shipping it in generated output would make CIDX impose one on its users. Whether that trade is worth making is a separate decision, not a consequence of this one.

## Future Enhancements

- Override flags: `--force-production`, `--force-dry-run`
- Pipeline-level `require_ci`
- Custom local behaviors per environment
- Validation warnings for risky operations
