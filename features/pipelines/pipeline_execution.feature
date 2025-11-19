Feature: Pipeline Execution
  As a DevOps engineer
  I want pipelines to execute phases in order
  So that my CI/CD process is reliable and predictable

  Rule: Pipelines execute phases in defined order

    Scenario: Release pipeline executes all phases sequentially
      Given I have a release pipeline configured:
        """
        [pipelines.release]
        phases = ["security", "code", "test", "build", "release", "docker"]
        """
      When I run "cidx run release"
      Then phases should execute in this exact order:
        | order | phase    |
        | 1     | security |
        | 2     | code     |
        | 3     | test     |
        | 4     | build    |
        | 5     | release  |
        | 6     | docker   |
      And each phase should complete before the next starts

    Scenario: PR pipeline executes validation phases only
      Given I have a pr pipeline configured:
        """
        [pipelines.pr]
        phases = ["security", "code", "test"]
        """
      When I run "cidx run pr"
      Then phases should execute in order:
        | security |
        | code     |
        | test     |
      And the pipeline should complete successfully

  Rule: Pipeline stops on first failure

    Scenario: Pipeline stops when security phase fails
      Given I have a pipeline with multiple phases
      And the "security" phase will fail
      When I run the pipeline
      Then the "security" phase should execute
      And the "security" phase should fail
      And the "code" phase should NOT execute
      And subsequent phases should NOT execute
      And the pipeline should exit with non-zero code

    Scenario: Pipeline continues through passing phases
      Given I have a pipeline: security → code → test
      And all phases will pass
      When I run the pipeline
      Then all three phases should execute
      And the pipeline should complete successfully
      And exit code should be 0

  Rule: Named pipelines provide clear intent

    Scenario Outline: Different pipelines for different purposes
      Given I run pipeline "<pipeline>"
      Then it should execute phases: <phases>
      And the description should indicate "<purpose>"

      Examples:
        | pipeline | phases                                        | purpose           |
        | pr       | security, code, test                          | PR validation     |
        | main     | security, code, test, build                   | Main branch build |
        | release  | security, code, test, build, release, docker  | Full release      |
        | quick    | code                                          | Quick check       |

  Rule: Pipelines can be listed and inspected

    Scenario: List all available pipelines
      When I run "cidx list pipelines"
      Then I should see all configured pipelines
      And each pipeline should show its phases
      And each pipeline should show its description

    Scenario: Show pipeline details
      When I run "cidx info release"
      Then I should see the release pipeline configuration
      And I should see which phases it includes
      And I should see the execution order
