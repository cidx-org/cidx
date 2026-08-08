# The Image Lifecycle

The [three rules](supply-chain-policy.md#the-three-rules) say how a reference is pinned and when it may move. This page is the other half: how CIDX finds out that one _should_ move — and the several distinct ways an image can go quiet without ever answering 404.

Each section is one state `cidx preset scan-targets` can report, the incident that put it there, and the recurring question behind all of them: whether a human or the machine is expected to act. [The diagrams](image-supply-chain.md) show how the states fit together.

## Finding versions on the registries that only list tags

`GET /v2/<repo>/tags/list` works on every OCI registry the catalogue pulls from, behind the same Bearer challenge as the manifest lookup, and that is how `cidx preset scan-targets` reaches gcr.io, ghcr.io and dhi.io — 9 of the 21 catalogue images, every Docker Hardened Image among them, which had no update detection at all before #245.

The listing is an unordered set of names, so the newest version is worked out from the names themselves. A version qualifies only if it has the **same shape** as the tag the catalogue pins: the same `v` prefix, the same variant suffix, the same number of components. `dhi.io/golang:1.23-alpine3.21-dev` is therefore never offered a plain `1.24` — a different base image — and an image pinned `0.68` is offered `0.71` rather than `0.71.2`. Versions compare as numbers: `1.24` is newer than `1.9`, which no lexical ordering would say.

**That rule now governs every registry, and did not always.** Docker Hub and Quay.io had been reached first and kept their own selection: a bare semver regex over a listing ordered by push date, first match wins, with the variant family standing in as a hardcoded list of seven suffixes — `-alpine`, `-slim`, `-bullseye`, `-bookworm`, `-buster`, `-jammy`, `-focal`. Anything else was read as no variant at all. So `buildpack-deps:trixie-curl` — Debian 13, and the base [`cargo-audit` runs on](supply-chain-policy.md#when-no-variant-is-the-answer-cargo-audit) — was offered `buildpack-deps:26.10`, an **Ubuntu development branch**, past the cooldown and ready to promote (#328). Three things had to be wrong at once, and all three were: `-curl` was not on the list, `trixie` is not a number so the pin looked unversioned, and `buildpack-deps` publishes Debian codenames and Ubuntu release numbers in one namespace, where the newest number is not the newest version of anything in particular.

The listing shape is per registry; the choice made from it is not. Both paths now return the same `(names, dates)` and hand it to the same comparison — which is what the rule above had always claimed, and what its tests had only ever exercised on one of the two routes.

**A newer version is not always a candidate.** ghcr.io and dhi.io date nothing:

- ghcr.io's dates live in the GitHub Packages API, which needs a `read:packages` token and answers 403 for a package owned by another organisation.
- dhi.io has no repository on `hub.docker.com` to ask, and its registry response carries names only.

Reporting versions found there as candidates would be worse than reporting nothing: the cooldown is fail-closed, so each one would be held in every weekly run from now until someone acted on it by hand — noise that never resolves. They are reported in a state of their own instead, `newer_version` with a reason saying the registry publishes no date, and the workflow summary lists them under **Newer version, not promotable automatically**. Pinning one is a deliberate act with a human behind it.

## A release that has not happened yet

A pre-release usually says so in its name — `-rc1`, `-beta`, `-nightly` — and the family rule above already refuses every one of them without knowing what any of those words mean: a candidate has to carry the pinned tag's suffix verbatim, so a marker the pin does not have cannot get through. A list of pre-release words would be unreachable code.

The channel that gets through is the one carrying no marker at all. A calendar-versioned distribution names a release by the month it is **due** and publishes images for it throughout its development: on 2026-07-30, `buildpack-deps:26.10` was "Ubuntu Stonking Stingray (development branch)" and would not be a release until October. Nothing in the name says so. The tag is pushed weekly like any other, so the cooldown ages it exactly like a release — and to an image pinned `24.04-curl` it is a perfectly well-formed successor: same variant, same precision, larger number.

The calendar is the one thing that answers, and it answers without a request: **a candidate whose version reads as a year and month later than the current month is not offered.** `26.10` is refused in August 2026, `26.04` is not, and `26.10` becomes offerable of its own accord in October — the rule reads a date rather than keeping a list of development branches, so nothing has to be edited when one ships.

It applies only where the **pinned** tag is itself calendar-versioned, which is what proves the repository numbers its releases that way: two components, a two-digit month in 01–12. Without that guard a tool sitting at `26.1` would see its own `26.10` read as October and refused; with it, `v2.95`, `0.71` and `3.24` are compared as the plain versions they are. This is the narrower half of the pair — the wider half is that a Debian suite is not offered an Ubuntu number at all — and it is the half that survives a repin onto `buildpack-deps:26.04-curl`, where the family rule would have nothing left to say.

## A tag that carries no version

`buildpack-deps:trixie-curl` and `koalaman/shellcheck:stable` are names, not versions. No tag a registry lists can be shown to be newer than a name, so the whole promotion path — which compares versions end to end — has nothing to say about these two images, and never will. Their updates arrive as **rebuilds of the same tag** under a new digest, and nothing here sees that: the cooldown, the family rule and the candidate all read tag names.

That is a real blind spot, and it is stated rather than papered over. `cidx preset scan-targets` reports `unversioned_tag` with the reason, and the workflow summary lists it under **Tag carries no version**, deliberately away from **Current (no updates)** — being unwatchable and being current are different facts, and this repository has twice been caught by the second hiding the first ([the deleted images](#a-pinned-image-that-vanished), [the frozen variant lines](#a-variant-line-that-froze)). Both images keep being scanned every week at the digest they are pinned to; what they never get is a candidate.

It is not annotated as a warning. This is a standing property of the pin, not an event: it would fire on every run from now until the pin changes, and a weekly alarm that cannot resolve is one nobody reads. Detecting the rebuild itself is a different mechanism from anything the promotion path does, and it is [the section below](#a-tag-rebuilt-under-the-same-name).

## A tag rebuilt under the same name

Everything above compares tag _names_. A rebuild changes no name, so none of it can see one — and the pin, whose whole purpose is that content cannot change under us, is exactly what makes the question unaskable anywhere else. `cidx preset scan-targets` therefore asks the registry one extra thing per image: **what does the pinned tag resolve to today?** When that digest is not the digest pinned beside it, the target reports `rebuilt`, and the summary lists it under **Pinned tag rebuilt upstream**.

The same comparison reads two ways, which is why it runs against every image rather than only the unversioned ones:

- On a tag carrying no version, a moved digest is the update channel working as designed. It is the only channel `trixie-curl` and `stable` have.
- On a versioned tag, the version names the same release, so whatever changed is something the version does not describe — normally a rebuild against patched base packages.

**The second case is not the rare one.** The first run against the catalogue found five moved digests, and four were `dhi.io` images on versioned tags: `docker:29-cli`, `golang:1.26.5-alpine-dev`, `python:3.13.14-alpine-dev`, `trivy:0.71`. The promotion path, which only ever sees a new version _number_, reported all four as current. The fifth was `buildpack-deps:trixie-curl`, republished the day before, which is the case [#332](https://github.com/cidx-org/cidx/issues/332) was opened about.

**What those four rebuilds carried was measured before anything was decided**, old digest against new. Every layer differed and the images had been rebuilt weeks apart, on the same base version underneath. Three of the four carried **no vulnerability change at any severity, under either scanner**. The fourth did: the `trivy:0.71` rebuild fixed two HIGH findings — `GHSA-hrxh-6v49-42gf` in `google.golang.org/grpc` and `GO-2026-5970` in `golang.org/x/text`.

**And the first pass got that wrong**, which is the more useful half of the story. It measured with Trivy alone and concluded that all four rebuilds carried nothing. Grype, run afterwards on the same eight images, found the two fixes Trivy had never reported. `container-monitor.yml` has always run both scanners and taken the union, for precisely this reason — the ad-hoc measurement did not, and one scanner produced a confident, wrong answer. A number is not a measurement until it says which instrument produced it.

Both halves argue for the design rather than against it. A signal that usually has nothing to say is exactly the one that must not be wired to automatic adoption: three of those four promotions would have spent the pin's guarantee on unreviewed content for no measurable gain — and the fourth was indistinguishable from them until someone measured properly. Reporting costs a line in a summary and a scan a human can run in an afternoon; adopting spends a guarantee that cannot be bought back.

**Reported, never promoted.** Adopting a new digest on an unchanged tag is the quietest substitution there is, and precisely the one digest pinning exists to refuse: a compromised publisher pushes to the same name, and no scan sees a backdoor that carries no CVE ([what we are actually defending against](vulnerability-management.md#what-we-are-actually-defending-against)). The pin's promise is that what runs is what was reviewed, and a rebuild is by definition unreviewed. So this produces a line for a human to weigh and a PR to open by hand — roughly monthly, against a guarantee that cannot be bought back once it is spent.

Unlike the two sections around it, it _is_ annotated as a warning: a repin clears it. That is the test those sections set — a signal that no action resolves is noise, a signal that an action resolves is work.

A lookup that fails says nothing rather than `rebuilt`. An absence of evidence must not read as evidence — the rule the cooldown and [the staleness signal](#a-base-that-stopped-being-supported) both already follow, and the one that matters most here, since a false positive on this signal sends someone chasing a supply-chain incident that never happened.

## A pinned image that vanished

Rule 1 makes a reference immutable; it does not make it eternal. Two catalogue images — `dhi.io/alpine-base:3.21` and `dhi.io/docker:27-cli` — were deleted upstream and answered 404, and nothing noticed until the presets using them failed to start (#244).

`cidx preset scan-targets` now resolves the exact reference each catalogue image is pinned to, digest included, and marks it `missing` when the registry says it does not exist. `container-monitor.yml` annotates the run with an error and fails its summary job, so the weekly run goes red. A 401 from a registry we hold no credentials for is reported as an unverified image, never as a deleted one — the loudest signal the command has must not cry wolf.

## A variant line that froze

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

That record only works while it points at what we actually run. Entries were keyed `repo:tag`, so every promotion left the ones recorded against the replaced version behind: they stopped matching, which is correct, and then nothing said so — 138 of the file's 155 entries were keyed to tags the catalogue had passed, and rule 3's waiver had gone quiet with them (#248). Keying by repository ([below](vulnerability-management.md#an-exception-dies-with-its-cve-not-with-a-tag)) removes the case a promotion creates; `cidx security vuln list --stale` lists what is left, the entries whose repository the catalogue no longer runs at all.

**The catalogue, and only the catalogue.** These rules govern the built-in preset catalogue. `cidx preset scan-targets` therefore reads `pkg/presets/presets.toml` rather than the resolved preset registry, which also carries whatever the user and the project declared in their own `presets.toml` — images the policy does not govern and the promotion job could not update anyway (guardrail 1, #248).

## A base that stopped being supported

The two sections above are about a _reference_ going quiet: one deleted, one whose variant family stopped being published. There is a third, and it is wider than either, because it is a property of the **base** rather than of the tag.

An image whose distribution has reached end of support receives no further security updates. Its packages are frozen at whatever they were on the day support ended, so the findings the scanners report on it are permanent — not "not fixed yet", but never. The tag can be current, the digest can resolve, the cooldown can promote it on schedule, and none of that changes the answer. Every question the [triage](vulnerability-management.md#judging-a-finding) asks is downstream of this one: "is a fix available?" has no meaning for a base nobody is fixing.

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
