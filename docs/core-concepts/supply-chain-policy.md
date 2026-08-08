# Supply-Chain Policy

This governs how the third-party artefacts CIDX itself depends on are pinned and updated. There are two classes, and the same three rules apply to both:

- **The built-in preset catalogue** — the container images CIDX runs. This page states the rules and how an image is chosen in the first place; [The image lifecycle](image-lifecycle.md) is what the monitor does with them week to week, and [Vulnerability management](vulnerability-management.md) is what becomes of the findings a scan produces.
- **This repository's own CI** — the GitHub Actions its workflows call, and its Go modules. See [The same rules, applied to GitHub Actions](vulnerability-management.md#the-same-rules-applied-to-github-actions).

Neither is imposed on projects using CIDX. You pin whatever you want in your own `cidx.toml`, and the workflows `cidx generate github` writes for you are yours to harden or not — per guardrail 5, CIDX is an execution engine, not a governance framework.

The reverse does bind the generators, though: what CIDX writes for someone else has to run on their runners. A generated workflow therefore never references a Docker Hardened Image, however clean it measures — `dhi.io` answers `401` to anyone without an entitlement, and the catalogue's Go image is only usable here because this repository's own CI holds credentials for it (#288). `cidx generate gitlab` bootstraps on the Docker Official `golang:<version>-alpine`, pinned by digest, from the same version constant the GitHub generator hands to `actions/setup-go`.

## Why scanning is not enough

`container-monitor.yml` scans every candidate image with Trivy and Grype, and both refresh their databases on each run. That covers known vulnerabilities well. It does not cover a compromised image, for two reasons:

- **A CVE exists only once someone has found the flaw.** A deliberately backdoored image has no CVE until the world notices. In the xz-utils case the backdoor landed in February 2024 and was found on 29 March — for six weeks a perfectly current scanner reported green, because there was nothing yet to know.
- **Neither scanner does behavioural analysis.** They match an inventory of installed packages against CVE lists. A modified entrypoint, an added `curl | sh`, a binary swapped at an unchanged version — none of it registers, ever.

So there are two delays, and database freshness only addresses the first:

| Delay                                           | Duration      | Addressed by       |
| ----------------------------------------------- | ------------- | ------------------ |
| CVE published → scanner knows it                | hours         | database refresh   |
| Malicious code published → someone discovers it | days to weeks | the cooldown below |

## The three rules

**1. Pin by digest.** Every catalogue image is written `image:tag@sha256:...`. The tag stays readable; the digest makes the reference immutable. A tag alone is mutable — `commitizen:4.15.1` can point at different content tomorrow with nothing in `presets.toml` changing. This is the quietest vector, and no version-based rule catches it.

**2. Wait 14 days.** A newly published version is not promoted until it has been publicly available for 14 days, comfortably past the usual detection window for a compromise (24–72h). The monitor runs weekly, so this is roughly two cycles.

Why not "always stay one version behind"? An attacker publishing twice in a row defeats it, and on a project that ships twice a year it would strand the catalogue on months-old CVEs. Age is measurable and independent of upstream's release cadence; lagging a version or two falls out of it naturally when upstream ships often.

**3. Waive the wait for a real fix.** When a new version fixes a vulnerability that actually affects us, it is promoted immediately — deliberately running a known-vulnerable image to guard against a hypothetical one is the worse trade. The waiver is stated in the promotion PR: which CVE, affecting which image, fixed by which version.

## At equal tool and version, take the smaller image

The three rules govern how a reference is pinned and when it moves. They say nothing about which reference to pick in the first place, and that choice turns out to dominate the numbers.

`golangci-lint` was pinned to the publisher's default image, built on Debian. Moving to `-alpine` — same publisher, same build, same version of the same tool — took it from **604 HIGH / 23 CRITICAL to 98 HIGH / 0 CRITICAL** (#280). 515 of those findings were never in golangci-lint at all: they were in a base image the linter does not use, shipped along with it.

So: **at equal tool and equal version, prefer the smallest variant the publisher offers** — `alpine`, `slim`, `distroless`, in that order of preference where they exist — and check at the moment of choosing, not later. A finding that is not in the image cannot be scanned, triaged, excepted, or re-argued in ninety days; the cheapest exception is the one never written.

Two caveats, both real:

- **The variant has to actually work.** A distroless image with no shell breaks any preset whose command is a pipeline, and an alpine image is musl, not glibc — a tool that ships a glibc binary will not run in one. The variant is a candidate, not an automatic winner; `cidx run <preset> --dry-run` and one real run are what settle it.
- **The variant line is a commitment.** Moving between families later is a repin by hand, never a promotion (see [A variant line that froze](image-lifecycle.md#a-variant-line-that-froze)), so picking `-alpine3.21` means watching whether that family is still published.

## Applying it to the rest of the catalogue

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

The remaining images publish no smaller variant at their pinned version (`commitizen`, `commitlint`, `gitleaks`, `black`, `goreleaser`, `gh`, `gosec`, `shellcheck`, `probatum`, the Ansible dev-tools image), or are Docker Hardened Images that are already the minimal build (`dhi.io/*`). `dhi.io/golang:1.23-alpine3.21-dev` carried 340 HIGH / 23 CRITICAL, the catalogue's second-worst, and no smaller variant would have helped: it was the frozen variant line of [the section below](image-lifecycle.md#a-variant-line-that-froze), and getting out of it took a base-version decision, not a variant choice.

## When no variant is the answer: `cargo-audit`

`cargo-audit` was the one Rust preset the sweep above left on the full image. `rust:<version>-slim` ships no HTTP client — the official Dockerfile installs `wget` to fetch rustup and `apt-get remove`s it in the same layer — and the preset downloads the RustSec release binary rather than `cargo install`ing it, which #161 measured in minutes and #188 fixed for non-root. On `-slim` it failed with `sh: curl: not found`.

That the rule had no move to offer was the tell. The question was never which variant of the Rust image to take: auditing a `Cargo.lock` reads a text file against an advisory list — no compiler, no crates, no toolchain — and one preset was holding the catalogue's worst image, **179 HIGH/CRITICAL findings against 415 for the other twenty put together**, in order to run a binary it downloads anyway (#287).

So it left the family rather than the variant: `buildpack-deps:trixie-curl`, **179 → 41**, and the catalogue 594 → 456. That is the Docker Official base `rust:` is itself built on, minus the toolchain, carrying exactly what the preset's one line needs — `sh`, `curl`, `tar` and CA certificates. The command is unchanged but for one flag. **The rule generalises: before comparing variants, ask whether the tool needs that image at all.** A preset that fetches its own binary is coupled to a libc and a shell, not to a toolchain, and the second question is much cheaper to answer than the first.

**Why not alpine, which measures 0 / 0 at 4 MB.** RustSec publishes a musl asset for `x86_64` only; `cargo-audit-aarch64-unknown-linux-musl` is a 404. A musl base would buy the last 41 findings by breaking every aarch64 user — [#195](image-lifecycle.md#a-variant-line-that-froze) with the arrow reversed, and the second time this catalogue has refused alpine over libc. glibc keeps `$(uname -m)-unknown-linux-gnu` resolving on both architectures, exactly as the preset already did; the run was verified under `linux/arm64` as well as `linux/amd64`, detecting RUSTSEC-2021-0139 on each.

**What it cost: the yanked-crate check.** cargo-audit shells out to `cargo -V` to learn which crates.io index protocol to use, so with no toolchain in the image it reports `couldn't update crates.io index: registry: No such file or directory`. The preset passes `--no-yanked` rather than print that on every run — a yank is not an advisory, the RustSec scan the preset exists for is untouched, and a project that wants the check back overrides `command` in its own `cidx.toml`. What the preset does **not** need is `git`: the release binary fetches the advisory database itself, measured on an image carrying no `git` at all.

**What it gives up in exchange.** `buildpack-deps:trixie-curl` carries no version in its tag, so `cidx preset scan-targets` offers it no candidate — the same position `koalaman/shellcheck:stable` is already in. The digest keeps being scanned every week; what stops is automatic promotion, because a Debian suite is not a version to compare. Moving `trixie` on is a repin by hand, like the variant lines above.

That was written as a consequence accepted in advance, and for six weeks it was not what the code did: both images were being offered a candidate, and `trixie-curl` was offered an Ubuntu development branch (#328). Both are now reported as [a tag that carries no version](image-lifecycle.md#a-tag-that-carries-no-version), which is what this paragraph had assumed all along.

## How the rules are applied

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
