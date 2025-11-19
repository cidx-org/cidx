Feature: Local Safety Modes
  As a developer
  I want to test dangerous operations locally
  Without accidentally publishing to production
  So that I can validate the release process safely

  Background:
    Given CIDX has environment detection enabled

  Rule: Docker operations are safe by default in local environment

    Scenario: docker-buildx runs in no-push mode locally
      Given I am in local environment
      And the "docker-buildx" preset has local_behavior = "no-push"
      When I run "cidx run docker"
      Then I should see "Local safety: no-push - Local mode: build without push"
      And the command should NOT include "--push" flag
      And the environment variable "DOCKER_PUSH" should be "false"
      And Docker image should be built successfully
      But Docker image should NOT be pushed to registry

    Scenario: docker-buildx pushes in CI environment
      Given I am in CI environment (GitHub Actions)
      And the "docker-buildx" preset has local_behavior = "no-push"
      When I run "cidx run docker"
      Then the command should include "--push" flag
      And Docker image should be built
      And Docker image should be pushed to registry
      And I should see "Pushed to ghcr.io"

    Scenario: kaniko builds without push locally
      Given I am in local environment
      And the "kaniko" preset has local_behavior = "no-push"
      When I run tool "kaniko"
      Then the command should NOT include "--destination" flag
      And Docker image should be built
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

    Scenario: goreleaser creates snapshot locally
      Given I am in local environment
      And the "goreleaser" preset has local_behavior = "draft"
      When I run tool "goreleaser"
      Then the command should include "--snapshot" flag
      And release should be built
      But release should NOT be published to GitHub

  Rule: Preset can require CI environment

    Scenario: Preset with require_ci fails in local
      Given I am in local environment
      And a preset has require_ci = true
      And the preset has NO local_behavior defined
      When I try to run that preset
      Then it should fail immediately
      And I should see error "preset requires CI environment"

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
        | no-push    | build without push              |
        | dry-run    | dry-run only                    |
        | production | production (use with caution!)  |

    Scenario: Disabled preset refuses local execution
      Given I am in local environment
      And a preset has local_behavior = "disabled"
      When I try to run that preset
      Then it should fail immediately
      And I should see error "disabled in local environment"
