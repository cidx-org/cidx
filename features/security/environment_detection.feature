Feature: Environment Detection
  As a CIDX user
  I want automatic environment detection
  So that behavior adapts intelligently to where CIDX runs

  Rule: CIDX detects major CI providers automatically

    Scenario Outline: Detect CI provider from environment variables
      Given the environment variable "<env_var>" is set to "<value>"
      When CIDX detects the environment
      Then it should identify as CI environment
      And the provider should be "<provider>"
      And I should see "Environment: <provider> (CI mode)"

      Examples:
        | provider        | env_var        | value |
        | GitHub Actions  | GITHUB_ACTIONS | true  |
        | GitLab CI       | GITLAB_CI      | true  |
        | Jenkins         | JENKINS_URL    | http://jenkins.local |
        | CircleCI        | CIRCLECI       | true  |

    Scenario: Detect local environment
      Given no CI environment variables are set
      When CIDX detects the environment
      Then it should identify as local environment
      And environment.IsCI should be false
      And I should see "Environment: Local (safe mode)"

  Rule: CIDX detects Git event context

    Scenario: Detect pull request in GitHub Actions
      Given I am in GitHub Actions
      And the environment variable "GITHUB_EVENT_NAME" is "pull_request"
      When CIDX detects the environment
      Then environment.IsPR should be true
      And environment.IsTag should be false
      And environment.BranchName should not be empty

    Scenario: Detect tag push in GitHub Actions
      Given I am in GitHub Actions
      And the environment variable "GITHUB_REF" is "refs/tags/v1.0.0"
      When CIDX detects the environment
      Then environment.IsTag should be true
      And environment.TagName should be "v1.0.0"
      And environment.IsPR should be false

    Scenario: Detect merge request in GitLab CI
      Given I am in GitLab CI
      And the environment variable "CI_MERGE_REQUEST_ID" is set
      When CIDX detects the environment
      Then environment.IsPR should be true
      And environment.Provider should be "GitLab CI"

    Scenario: Detect branch name in CI
      Given I am in CI environment
      And the branch is "feature/new-feature"
      When CIDX detects the environment
      Then environment.BranchName should be "feature/new-feature"
      And environment.IsTag should be false

  Rule: Environment detection happens once at startup

    Scenario: Environment is detected at CIDX startup
      When CIDX starts
      Then environment should be detected immediately
      And environment information should be logged
      And environment should NOT be re-detected during execution

  Rule: Environment detection is reliable and accurate

    Scenario: Multiple CI indicators prefer most specific
      Given the environment variable "GITHUB_ACTIONS" is "true"
      And the environment variable "CI" is "true"
      When CIDX detects the environment
      Then the provider should be "GitHub Actions"
      And NOT "Generic CI"

    Scenario: Local environment with Git repository
      Given I am in local environment
      And I am in a Git repository
      When CIDX detects the environment
      Then environment.IsCI should be false
      But Git information should be available
      And I can still run CIDX commands
