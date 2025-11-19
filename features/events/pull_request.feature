Feature: Pull Request Validation
  As a developer
  I want CIDX to validate my pull request
  So that code quality is ensured before merge

  Background:
    Given CIDX is configured with default presets
    And I am in a Git repository

  Rule: PR triggers only validation phases, not deployment

    Scenario: PR triggers security, code, and test phases only
      Given I create a pull request
      When I run "cidx run pr"
      Then it should execute the "security" phase
      And it should execute the "code" phase
      And it should execute the "test" phase
      But it should NOT execute the "build" phase
      And it should NOT execute the "release" phase
      And it should NOT execute the "docker" phase

    Scenario: Quick feedback for developers
      Given I create a pull request
      When I run "cidx run pr"
      Then the pipeline should complete in less than 10 minutes
      And I should receive clear feedback on failures

  Rule: PR fails fast on critical issues

    Scenario: PR fails on security vulnerability
      Given I create a pull request with a HIGH severity vulnerability
      When I run "cidx run pr"
      Then the "security" phase should fail
      And the pipeline should stop immediately
      And no further phases should execute
      And I should see error message containing "Security scan failed"

    Scenario: PR fails on code quality issues
      Given I create a pull request with linting errors
      When I run "cidx run pr"
      Then the "security" phase should pass
      And the "code" phase should fail
      And the "test" phase should NOT execute
      And I should see error message about code quality

    Scenario: PR fails on test failures
      Given I create a pull request with failing tests
      When I run "cidx run pr"
      Then the "security" phase should pass
      And the "code" phase should pass
      But the "test" phase should fail
      And I should see which tests failed

  Rule: PR works in any environment

    Scenario: PR validation in local environment
      Given I am in local environment
      When I run "cidx run pr"
      Then I should see "Environment: Local (safe mode)"
      And all phases should execute normally

    Scenario: PR validation in CI environment
      Given I am in CI environment (GitHub Actions)
      When I run "cidx run pr"
      Then I should see "Environment: GitHub Actions (CI mode)"
      And all phases should execute normally
