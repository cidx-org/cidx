Feature: The workflow a pipeline is compared with
  As a developer whose repository has more than one workflow
  I want "check workflow" to compare a pipeline with the workflow that implements it
  So that a workflow which merely shares its name is not reported as drift

  # The second half of issue #233. `check workflow` paired [pipelines.release]
  # with release.yml because their names line up. On this repository they are
  # not the same thing: release.yml publishes the release natively — it
  # cross-compiles, then attaches the assets with softprops/action-gh-release,
  # because release publishing is deliberately outside cidx's scope — and
  # delegates a single phase to `cidx run docker`. [pipelines.release] is the
  # end-to-end rehearsal `cidx run release` walks locally with the guardrails on.
  #
  # Nothing in a workflow file tells the two cases apart: a job doing its phase
  # natively and a job that lost its `cidx run` call look exactly alike. Any
  # inference would therefore mistake the drift the check exists to find for an
  # intended shape, so the exception is declared instead of guessed.

  Rule: By default, the filename names the pipeline a workflow implements

    Scenario: A workflow that runs every declared phase is in sync
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And workflow "ci.yml" has a job "security" running "./bin/cidx run security"
      And workflow "ci.yml" has a job "test" running "./bin/cidx --verbose run test"
      When I check every workflow
      Then pipeline "ci" should be in sync with its workflow

    Scenario: A phase the workflow stopped running is reported
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And workflow "ci.yml" has a job "security" running "./bin/cidx run security"
      And workflow "ci.yml" has a job "test" running "echo tests disabled"
      When I check every workflow
      Then pipeline "ci" should be reported out of sync
      And phase "test" should be reported as missing from the workflow

    Scenario: A release workflow that really runs the release pipeline is compared
      Given cidx.toml defines pipeline "ci" with phases "security"
      And cidx.toml defines pipeline "release" with phases "build, docker, release"
      And workflow "ci.yml" has a job "security" running "./bin/cidx run security"
      And workflow "release.yml" has a job "docker" running "./bin/cidx run docker"
      When I check every workflow
      Then pipeline "release" should be reported out of sync

  Rule: A pipeline can declare that no workflow implements it

    Scenario: The declared pipeline is left alone
      Given cidx.toml defines pipeline "ci" with phases "security"
      And cidx.toml defines pipeline "release" with phases "build, docker, release"
      And pipeline "release" declares that no workflow implements it
      And workflow "ci.yml" has a job "security" running "./bin/cidx run security"
      And workflow "release.yml" has a job "docker" running "./bin/cidx run docker"
      When I check every workflow
      Then pipeline "release" should not be compared with any workflow
      And pipeline "ci" should be in sync with its workflow

    Scenario: The exception covers one pipeline, not the check
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And cidx.toml defines pipeline "release" with phases "build, docker, release"
      And pipeline "release" declares that no workflow implements it
      And workflow "ci.yml" has a job "security" running "./bin/cidx run security"
      And workflow "ci.yml" has a job "test" running "echo tests disabled"
      And workflow "release.yml" has a job "docker" running "./bin/cidx run docker"
      When I check every workflow
      Then pipeline "release" should not be compared with any workflow
      And pipeline "ci" should be reported out of sync

  Rule: A pipeline whose workflow is named otherwise says so

    Scenario: The named workflow is the one compared
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And pipeline "ci" declares workflow "main.yml"
      And workflow "main.yml" has a job "security" running "./bin/cidx run security"
      And workflow "main.yml" has a job "test" running "./bin/cidx run test"
      When I check every workflow
      Then pipeline "ci" should be in sync with its workflow
