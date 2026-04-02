Feature: Environment Doctor
  As a developer
  I want to validate my environment before running pipelines
  So that I get clear diagnostics instead of cryptic errors

  Rule: Doctor checks container runtime availability

    Scenario: Docker is available
      Given Docker daemon is running
      When I run "cidx doctor"
      Then I should see a passing check for "Container runtime"
      And the check should show the Docker version

    @docker-required
    Scenario: No container runtime available
      Given Docker daemon is NOT running
      And Podman is NOT available
      When I run "cidx doctor"
      Then I should see a failing check for "Container runtime"
      And I should see a suggestion to install Docker or Podman

  Rule: Doctor checks Git repository

    Scenario: Valid Git repository detected
      Given I am in a Git repository
      When I run "cidx doctor"
      Then I should see a passing check for "Git repository"

    Scenario: Not in a Git repository
      Given I am NOT in a Git repository
      When I run "cidx doctor"
      Then I should see a failing check for "Git repository"

  Rule: Doctor checks configuration file

    Scenario: Valid config file
      Given a valid "cidx.toml" exists
      When I run "cidx doctor"
      Then I should see a passing check for "Config file"

    Scenario: No config file
      Given no "cidx.toml" exists
      When I run "cidx doctor"
      Then I should see a warning check for "Config file"
      And I should see a suggestion to run "cidx init"

  Rule: Doctor reports overall status

    Scenario: All checks pass
      Given Docker daemon is running
      And I am in a Git repository
      And a valid "cidx.toml" exists
      When I run "cidx doctor"
      Then the exit code should be 0
      And I should see "All checks passed"

    Scenario: Some checks fail
      Given Docker daemon is running
      And I am NOT in a Git repository
      When I run "cidx doctor"
      Then the exit code should be 1
      And I should see the number of issues found
