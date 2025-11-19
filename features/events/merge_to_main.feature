Feature: Merge to Main Branch
  As a developer
  I want CIDX to validate and build on main branch
  So that main is always in a releasable state

  Background:
    Given CIDX is configured with main pipeline
    And I am in a Git repository

  Rule: Main branch builds artifacts but doesn't deploy

    Scenario: Merge to main triggers validation and build
      Given I merge a pull request to main branch
      When I run "cidx run main"
      Then it should execute the following phases in order:
        | phase    |
        | security |
        | code     |
        | test     |
        | build    |
      But it should NOT execute the "release" phase
      And it should NOT execute the "docker" phase

    Scenario: Main branch creates build artifacts
      Given I am on main branch
      When I run "cidx run main"
      Then the "build" phase should create artifacts
      And artifacts should be stored in "bin/" directory
      And I should see message "Build artifacts created"

  Rule: Main branch validates everything before building

    Scenario: Main fails if security issues detected
      Given I merge code with security vulnerabilities to main
      When I run "cidx run main"
      Then the "security" phase should fail
      And the "build" phase should NOT execute
      And I should see security scan results

    Scenario: Main fails if tests fail
      Given I merge code with failing tests to main
      When I run "cidx run main"
      Then the "security" phase should pass
      And the "code" phase should pass
      But the "test" phase should fail
      And the "build" phase should NOT execute

  Rule: Main branch ensures production readiness

    Scenario: Main validates production-ready state
      Given I am on main branch
      When I run "cidx run main"
      And all phases pass successfully
      Then I should see message "Main branch is production-ready"
      And artifacts should be ready for release

    Scenario: Main branch can be released immediately
      Given I am on main branch
      And I run "cidx run main" successfully
      When I create a tag "v1.0.0"
      Then the release should use the pre-built artifacts
      And deployment should be faster
