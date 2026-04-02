Feature: CI Workflow Generation
  As a developer
  I want to generate CI platform files from cidx.toml
  So that I don't have to manually write platform-specific YAML

  Background:
    Given a valid "cidx.toml" exists

  Rule: Generate produces correct GitHub Actions structure

    Scenario: Generate GitHub Actions workflow
      Given cidx.toml defines pipeline "ci" with phases "security, code, test, build"
      When I run "cidx generate github"
      Then the output should be valid YAML
      And the output should contain a "bootstrap" job
      And each phase should have its own job

    Scenario: Phases run in parallel after bootstrap
      Given cidx.toml defines pipeline "ci" with phases "security, code, test"
      When I run "cidx generate github"
      Then jobs "security", "code", "test" should depend on "bootstrap"
      And jobs "security", "code", "test" should NOT depend on each other

    Scenario: Pipeline triggers map to GitHub events
      Given cidx.toml defines pipeline "pr"
      And cidx.toml defines pipeline "main"
      When I run "cidx generate github"
      Then "pr" pipeline should trigger on "pull_request"
      And "main" pipeline should trigger on "push" to "main" branch

  Rule: Generate respects output options

    Scenario: Output to stdout by default
      When I run "cidx generate github"
      Then the output should be printed to stdout

    Scenario: Output to file with -o flag
      When I run "cidx generate github -o .github/workflows/cidx.yml"
      Then the file ".github/workflows/cidx.yml" should be created

  Rule: Generate handles edge cases

    Scenario: No pipelines defined
      Given cidx.toml has no pipelines defined
      When I run "cidx generate github"
      Then the command should fail
      And I should see "no pipelines defined"

    Scenario: Unknown platform
      When I run "cidx generate unknown"
      Then the command should fail
      And I should see "unsupported platform"
