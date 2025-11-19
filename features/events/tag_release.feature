Feature: Release Tag Deployment
  As a release manager
  I want CIDX to automate deployments on version tags
  So that releases are consistent and safe

  Background:
    Given CIDX is configured with release and docker phases
    And I am in a Git repository

  Rule: Tags trigger full release pipeline in CI only

    Scenario: Tag triggers complete release pipeline in CI
      Given I am in CI environment (GitHub Actions)
      And I push a tag "v1.0.0"
      When I run "cidx run release"
      Then it should execute all phases in order:
        | phase    |
        | security |
        | code     |
        | test     |
        | build    |
        | release  |
        | docker   |
      And all phases should succeed
      And the pipeline should complete successfully

    Scenario: Semantic versioning tags are recognized
      Given I am in CI environment
      When I push tag "<tag>"
      Then CIDX should recognize it as a release tag

      Examples:
        | tag          |
        | v1.0.0       |
        | v2.1.3       |
        | v0.0.1       |
        | v10.20.30    |

  Rule: Release phase publishes in CI, creates draft locally

    Scenario: Release publishes in CI environment
      Given I am in CI environment (GitHub Actions)
      And GITHUB_TOKEN is set
      When I run "cidx run release"
      Then the "release" phase should publish the GitHub release
      And the release should be public
      And release notes should be generated
      And artifacts should be attached

    Scenario: Release creates draft in local environment
      Given I am in local environment
      And I have tag "v1.0.0" locally
      When I run "cidx run release"
      Then I should see "Local safety: draft - Local mode: draft creation only"
      And the "release" phase should create a draft release
      And the GitHub release should NOT be published
      And I should see message "Created draft release"

  Rule: Docker phase pushes in CI, builds only locally

    Scenario: Docker pushes images in CI environment
      Given I am in CI environment (GitHub Actions)
      And I have valid registry credentials
      When I run "cidx run release"
      Then the "docker" phase should build images
      And the "docker" phase should push images to registry
      And I should see "Pushed image to ghcr.io"

    Scenario: Docker builds without push in local environment
      Given I am in local environment
      When I run "cidx run release"
      Then I should see "Local safety: no-push - Local mode: build without push"
      And the "docker" phase should build images
      But the "docker" phase should NOT push images
      And I should see message "Image built successfully (not pushed)"

  Rule: Release requires proper environment setup in CI

    Scenario: Release fails without GitHub token in CI
      Given I am in CI environment
      But GITHUB_TOKEN is not set
      When I run "cidx run release"
      Then the "release" phase should fail
      And I should see error "GITHUB_TOKEN not set"

    Scenario: Docker fails without registry credentials in CI
      Given I am in CI environment
      But registry credentials are not set
      When I run "cidx run release"
      Then the "docker" phase should fail
      And I should see error about missing credentials
