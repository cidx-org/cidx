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

    Scenario: Pipeline respects an unusual declared order
      Given the pipeline "custom" is configured with phases "test, code, security"
      When I run pipeline "custom"
      Then phases should execute in order: "test, code, security"

  Rule: Pipeline stops on first failure

    Scenario: Pipeline stops after a middle phase fails
      Given I have a pipeline: code → security → test → build
      And the "security" phase will fail
      When I run the pipeline
      Then phases should execute in order: "code, security"
      And the pipeline should stop
      And subsequent phases should NOT execute
      And the pipeline should exit with non-zero code


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
      Given the pipeline "<pipeline>" is configured with phases "<phases>"
      When I run pipeline "<pipeline>"
      Then it should execute phases: <phases>

      Examples:
        | pipeline | phases                                       |
        | pr       | security, code, test                         |
        | main     | security, code, test, build                  |
        | release  | security, code, test, build, release, docker |
        | quick    | code                                         |

  Rule: A dry run is how a pipeline is inspected before it runs

    # `cidx list pipelines` and `cidx info release` were never commands — the
    # steps behind them returned nil, so nobody found out (#349). The command
    # that answers both questions is `cidx run <pipeline> --dry-run`: it
    # resolves the pipeline from cidx.toml and names every phase, in order,
    # without starting a container.

    Scenario: A dry run names the phases a pipeline includes
      Given I have a pr pipeline configured:
        """
        [pipelines.pr]
        phases = ["security", "code", "test"]
        """
      When I run "cidx run pr --dry-run"
      Then I should see which phases it includes
      And I should see the execution order

    Scenario: Show pipeline details
      Given I have a release pipeline configured:
        """
        [pipelines.release]
        phases = ["security", "code", "test", "build", "release", "docker"]
        """
      When I run "cidx run release --dry-run"
      Then I should see the release pipeline configuration
      And I should see the execution order

  Rule: This repository validates code, security, tests and build in that order

    Scenario: GitHub CI waits for each preceding validation gate
      When I inspect this repository's CI workflow
      Then its validation jobs should depend on each preceding gate
