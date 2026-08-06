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

    # A pipeline used to have to "indicate its purpose" as well, and could not:
    # the `description` key cidx.toml writes was dropped by the decoder without
    # a word, and the step behind that line asserted nothing, which is why the
    # claim survived (#349). #352 gave config.Pipeline the field and the runner
    # prints it, so the description is now a fact rather than an intention —
    # covered by TestLoad_PipelineDescription, since what a scenario can see
    # here is still the phases.

    Scenario Outline: Different pipelines for different purposes
      Given I run pipeline "<pipeline>"
      Then it should execute phases: <phases>

      Examples:
        | pipeline | phases                                        |
        | pr       | security, code, test                          |
        | main     | security, code, test, build                   |
        | release  | security, code, test, build, release, docker  |
        | quick    | code                                          |

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
