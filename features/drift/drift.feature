Feature: CI Drift Detection
  As a developer
  I want to compare my cidx.toml with the actual CI configuration
  So that I can detect unintentional divergence between my declaration and the platform

  Background:
    Given a valid "cidx.toml" exists

  Rule: Drift detection compares phases

    Scenario: All phases match
      Given cidx.toml defines pipeline "ci" with phases "security, code, test"
      And the GitHub Actions workflow has jobs "security, code, test"
      When I run "cidx check drift"
      Then I should see a phases table
      And all phases should show "match"
      And the exit code should be 0

    Scenario: Phase missing from CI
      Given cidx.toml defines pipeline "ci" with phases "security, code, test, build"
      And the GitHub Actions workflow has jobs "security, code, test"
      When I run "cidx check drift"
      Then phase "build" should show "missing from CI"
      And the exit code should be 1

    Scenario: Extra job in CI not in cidx.toml
      Given cidx.toml defines pipeline "ci" with phases "security, code"
      And the GitHub Actions workflow has jobs "security, code, test"
      When I run "cidx check drift"
      Then job "test" should show "extra in CI"

  Rule: Drift detection compares triggers

    Scenario: Triggers match
      Given cidx.toml defines pipeline "pr"
      And the GitHub Actions workflow triggers on "pull_request"
      When I run "cidx check drift"
      Then trigger "pull_request" should show "match"

    Scenario: Missing trigger
      Given cidx.toml defines pipeline "pr"
      And the GitHub Actions workflow does NOT trigger on "pull_request"
      When I run "cidx check drift"
      Then trigger "pull_request" should show "missing"

  Rule: Drift summary

    Scenario: No drift detected
      Given cidx.toml and CI workflow are in sync
      When I run "cidx check drift"
      Then I should see "No drift detected"
      And the exit code should be 0

    Scenario: Drift detected
      Given cidx.toml and CI workflow have differences
      When I run "cidx check drift"
      Then I should see the number of differences
      And the exit code should be 1
