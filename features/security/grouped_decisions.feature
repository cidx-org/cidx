Feature: Grouped vulnerability decisions
  As the maintainer of the built-in preset catalogue
  I want one reviewed decision to carry many findings without hiding any of them
  So that a review argues an exposure claim once, and a waiver never outlives what it rested on

  # The behaviour contract agreed in
  # docs/discussions/vulnerability-management-reset.md. A decision is an
  # exposure judgement about one repository, under the aggregate context of
  # every preset consuming it — 119 identical notes proved the old unit cloned
  # one judgement into records nobody could review. Members reference the
  # decision instead of cloning its prose; anything the decision rested on that
  # can no longer be shown — its date, its mechanical context, its semantic
  # predicates — waives nothing. Fail-closed, the posture everything else in
  # this domain already takes.

  Rule: A live decision waives exactly the findings that reference it

    Scenario: A live decision waives each finding it explicitly names
      Given a decision "rust-inert" on "rust" reviewed until "2026-12-01"
      And the decision "rust-inert" expects capabilities "publishing-credential"
      And the "rust" context provides capabilities "publishing-credential"
      And the member "CVE-2026-0001" on "rust" references decision "rust-inert"
      And the member "CVE-2026-0002" on "rust" references decision "rust-inert"
      When the waivers are resolved on "2026-09-01"
      Then "CVE-2026-0001" should be waived
      And "CVE-2026-0002" should be waived

    Scenario: An expired decision waives nothing
      Given a decision "rust-inert" on "rust" reviewed until "2026-06-01"
      And the member "CVE-2026-0001" on "rust" references decision "rust-inert"
      And the member "CVE-2026-0002" on "rust" references decision "rust-inert"
      When the waivers are resolved on "2026-09-01"
      Then "CVE-2026-0001" should not be waived
      And "CVE-2026-0002" should not be waived
      And the verdict for "CVE-2026-0001" should mention "needs review"

  Rule: A decision dies with its context, not only with its date

    # The scanner ignore applies to the image, not to one preset invocation, so
    # the decision context is the union over every consumer: a benign consumer
    # must never hide the exposure of a more privileged one. The reviewer sees
    # the capability by name — an opaque fingerprint mismatch would be a hash
    # with error formatting.

    Scenario: A changed mechanical capability invalidates a decision
      Given a decision "gorel-inert" on "goreleaser/goreleaser" reviewed until "2026-12-01"
      And the decision "gorel-inert" expects capabilities "publishing-credential"
      And the "goreleaser/goreleaser" context provides capabilities "publishing-credential, docker-socket"
      And the member "CVE-2026-0003" on "goreleaser/goreleaser" references decision "gorel-inert"
      When the waivers are resolved on "2026-09-01"
      Then "CVE-2026-0003" should not be waived
      And the verdict for "CVE-2026-0003" should mention "docker-socket"

    # Trust is not an intrinsic property of a preset: the same tool can read a
    # maintainer branch or an untrusted pull request. A model that depends on
    # bounded input is valid only where a review established the boundary —
    # unknown is not bounded.

    Scenario: Missing semantic context fails closed
      Given a decision "bp-tool" on "buildpack-deps" reviewed until "2026-12-01"
      And the decision "bp-tool" requires "bounded-input"
      And the "buildpack-deps" context establishes no semantic predicates
      And the member "CVE-2026-0004" on "buildpack-deps" references decision "bp-tool"
      When the waivers are resolved on "2026-09-01"
      Then "CVE-2026-0004" should not be waived
      And the verdict for "CVE-2026-0004" should mention "bounded-input"

  Rule: Mechanical capabilities are read off the declarations, never guessed

    # The same declarations the executor runs from: env and volumes. Nothing is
    # parsed out of the command string — input origin and repository execution
    # are semantic context, reviewed rather than inferred.

    Scenario: Capabilities are derived from what a preset declares
      Given a preset declaring env "GH_TOKEN" as "${GITHUB_TOKEN}"
      And the preset mounts "/var/run/docker.sock"
      When its capabilities are derived
      Then the derived capabilities should be "docker-socket, publishing-credential"

  Rule: The file never half-loads

    # A member whose decision is missing would fall back to legacy semantics it
    # no longer carries. Refusing the whole file names both halves of the
    # broken reference, so the fix is one edit away.

    Scenario: A missing decision reference fails closed
      Given a vulnerability file where "CVE-2026-0005" references the unknown decision "ghost"
      When the vulnerability file is loaded
      Then loading should fail mentioning "CVE-2026-0005"
      And loading should fail mentioning "ghost"

  Rule: Migration changes nothing it has not reviewed

    Scenario: A legacy entry keeps its existing deadline during migration
      Given a legacy entry "CVE-2026-0006" on "rust" expiring "2026-11-18"
      When the waivers are resolved on "2026-09-01"
      Then "CVE-2026-0006" should be waived
      When the waivers are resolved on "2026-11-19"
      Then "CVE-2026-0006" should not be waived

  Rule: A fix is learned from evidence, never inferred from absence

    # The evidence channel is #312's: what the scanners recorded as suppressed,
    # fix version included. The fixable-age clock starts on the day the fix was
    # observed, not on the day somebody got around to looking — and one member
    # gaining a fix says nothing about its neighbours.

    Scenario: A newly observed fix starts remediation without relying on absence
      Given a decision "rust-inert" on "rust" reviewed until "2026-12-01"
      And the member "CVE-2026-0001" on "rust" references decision "rust-inert"
      And the member "CVE-2026-0002" on "rust" references decision "rust-inert"
      And suppressed scanner evidence reports "CVE-2026-0001" fixed in "1.2.3" observed on "2026-09-15"
      When the decision lifecycle is evaluated
      Then "CVE-2026-0001" should be queued for remediation with its clock starting "2026-09-15"
      And "CVE-2026-0002" should not be queued for remediation
