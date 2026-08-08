# Security & Environments

This page was four documents welded together. It is now four documents.

It stays behind as an index because its URL is not only a link: `sarif.go` writes it into the `helpUri` of every alert the security audit uploads, and the alerts already in this repository's Security tab still carry it. Deleting the file would turn each of those into a 404 — the alerts are the record, and a record whose citations are dead is worth less than one that is merely out of date.

## Environments

Environment detection, `local_behavior`, contextual pipelines — what CIDX refuses to do when it works out that it is running on someone's laptop.

→ **[Environments & Local Safety Modes](environments.md)**

## Supply-Chain Policy

The three rules governing how the third-party artefacts CIDX depends on are pinned and updated, why scanning alone does not cover the threat, and how the image is chosen in the first place — the decision that turned out to dominate the numbers.

→ **[Supply-Chain Policy](supply-chain-policy.md)**

## The Image Lifecycle

Every distinct way a pinned image can go quiet: a version that appears, a release that has not happened yet, a tag carrying no version, a tag rebuilt under the same name, a reference deleted upstream, a variant line that froze, a base that stopped being supported. One section per state, each with the incident that put it there.

→ **[The Image Lifecycle](image-lifecycle.md)**

## Vulnerability Management

What a scanner's answer is worth: the threat model every judgement rests on, the four questions that triage a finding, when an exception is the right instrument and when it records a decision nobody took, and where to look for any of it. Plus what the whole policy costs, and the same three rules applied to this repository's own GitHub Actions and Go modules.

→ **[Vulnerability Management](vulnerability-management.md)**

## The map

The same machinery as five diagrams, for reading before the prose rather than after it.

→ **[The Image Supply Chain, in Diagrams](image-supply-chain.md)**
