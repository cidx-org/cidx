Feature: Local Safety Modes
  As a developer
  I want to test dangerous operations locally
  Without accidentally publishing to production
  So that I can validate the release process safely

  Background:
    Given CIDX has environment detection enabled

  Rule: Docker operations are safe by default in local environment

    # The guardrail is the dry-run, and it always was. `no-push` set the same
    # IsDryRun as `dry-run`, so nothing was built under either; what it added on
    # top — stripping --push, injecting DOCKER_PUSH=false — doctored a command
    # that is only ever printed. #353 removed the mode and kept the guarantee.
    # The command now shows what CI will run, --push included, which is the
    # honest answer to "what would this do".

    Scenario: docker-buildx builds nothing locally
      Given I am in local environment
      And the "docker-buildx" preset has local_behavior = "dry-run"
      When I run "cidx run docker"
      Then I should see "Local safety: dry-run - Local mode: dry-run only"
      And the run should be held as a dry-run
      But Docker image should NOT be pushed to registry

    Scenario: docker-buildx pushes in CI environment
      Given I am in CI environment (GitHub Actions)
      And the "docker-buildx" preset has local_behavior = "dry-run"
      When I run "cidx run docker"
      Then the command should include "--push" flag
      And Docker image should be built
      And Docker image should be pushed to registry
      And I should see "Pushed to ghcr.io"

    # kaniko keeps its --destination, as it always did: ApplyExecutionMode
    # rewrites the command of gh-release by name and of nothing else. That is
    # not the leak it looks like — the run is held as a dry-run, so the command
    # is never executed. The old line claimed a flag was stripped that never was
    # (#349), and the mode that claimed to strip one is gone (#353).

    Scenario: kaniko builds nothing locally
      Given I am in local environment
      And the "kaniko" preset has local_behavior = "dry-run"
      When I run tool "kaniko"
      Then the run should be held as a dry-run
      But image should NOT be pushed

  Rule: GitHub releases are draft by default in local environment

    Scenario: gh-release creates draft locally
      Given I am in local environment
      And the "gh-release" preset has local_behavior = "draft"
      When I run "cidx run release"
      Then I should see "Local safety: draft - Local mode: draft creation only"
      And the command should include "--draft" flag
      And the environment variable "DRAFT" should be "true"
      And GitHub release should be created as draft
      And release should NOT be published

    Scenario: gh-release publishes in CI environment
      Given I am in CI environment (GitHub Actions)
      And GITHUB_TOKEN is set
      And the "gh-release" preset has local_behavior = "draft"
      When I run "cidx run release"
      Then the command should NOT include "--draft" flag
      And GitHub release should be published
      And release should be public

    # Same correction as kaniko: goreleaser's command is `release --clean` and
    # stays that way — no --snapshot is ever added. draft injects DRAFT=true and
    # holds the run as a dry-run, which is what keeps a local `cidx run
    # goreleaser` from reaching GitHub (#349).

    Scenario: goreleaser creates no release locally
      Given I am in local environment
      And the "goreleaser" preset has local_behavior = "draft"
      When I run tool "goreleaser"
      Then the environment variable "DRAFT" should be "true"
      And the run should be held as a dry-run
      But release should NOT be published to GitHub

  Rule: Preset can require CI environment

    Scenario: Preset with require_ci fails in local
      Given I am in local environment
      And a preset has require_ci = true
      And the preset has NO local_behavior defined
      When I try to run that preset
      Then it should fail immediately
      And I should see error "requires CI environment"

    Scenario: Preset with require_ci and local_behavior works locally
      Given I am in local environment
      And a preset has require_ci = false
      And the preset has local_behavior = "draft"
      When I run that preset
      Then it should execute in draft mode
      And it should NOT fail

  Rule: Local behavior modes work as specified

    Scenario Outline: Different local behaviors
      Given I am in local environment
      And a preset has local_behavior = "<behavior>"
      When I run that preset
      Then it should execute in "<behavior>" mode
      And I should see message containing "<message>"

      Examples:
        | behavior   | message                         |
        | draft      | draft creation only             |
        | dry-run    | dry-run only                    |
        | production | production (use with caution!)  |

    # #353: it was accepted, it meant dry-run, and it promised a local build it
    # never performed. Refused by name rather than as an unknown value, so the
    # reader is not left hunting for a difference that never existed.
    Scenario: The removed no-push behaviour is refused by name
      Given I am in local environment
      And a preset has local_behavior = "no-push"
      When I try to run that preset
      Then it should fail immediately
      And I should see error "removed in cidx v3.0.0"

    Scenario: Disabled preset refuses local execution
      Given I am in local environment
      And a preset has local_behavior = "disabled"
      When I try to run that preset
      Then it should fail immediately
      And I should see error "disabled in local environment"
