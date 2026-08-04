Feature: Quiet Mode Execution
  As a CI/CD pipeline user
  I want to suppress successful execution logs
  So that I can focus only on failures and reduce log noise

  Rule: Quiet mode suppresses success output but shows failures

    Scenario: Successful execution produces minimal output
      Given I have a tool "echo-success" that exits with code 0
      When I run "cidx run echo-success --quiet"
      Then I should see "✓ echo-success completed"
      And I should NOT see the standard output of the tool

    Scenario: Failed execution shows buffered output
      Given I have a tool "fail-tool" that prints "error details" and exits with code 1
      When I run "cidx run fail-tool --quiet"
      Then the command should fail
      And I should see "error details"
      And I should see "container exited with code 1"

    Scenario: Quiet mode with parallel execution
      Given I have multiple tools running in parallel
      When I run "cidx run all --parallel --quiet"
      Then I should see completion messages for successful tools
      And I should only see logs for failed tools

  Rule: Container output can be streamed without switching the logs to debug

    # Issue #273. A run in CI is quiet by default (#87), which is right for a
    # lint that passes and wrong for a test job: the Test job printed
    # `PHASE: TEST` then `go-test completed`, and a green run showing nothing
    # cannot say whether the suite executed every scenario or none of them.
    # `--verbose` was the only way out, and it also switches logrus to Debug
    # and prints the raw JSON of every image pull -- so the choice was "show
    # nothing" or "show everything, noise included". `--stream` asks for the
    # container output and nothing else.

    # These scenarios call the decision cidx really makes, not a copy of it.

    Scenario Outline: What each flag does to the output of a successful run
      Given cidx runs <where>
      When the run is invoked with "<flags>"
      Then container output should be <output>

      Examples: The default, and the two ways out of it
        | where    | flags     | output   |
        | locally  |           | streamed |
        | in CI    |           | buffered |
        | in CI    | --stream  | streamed |
        | in CI    | --verbose | streamed |
        | locally  | --quiet   | buffered |

    Scenario: Asking to see the output wins over asking for silence
      Given cidx runs in CI
      When the run is invoked with "--quiet --stream"
      Then container output should be streamed

  Rule: Quiet mode is configurable via CLI flag

    @docker-required
    Scenario Outline: Quiet flag variations
      Given Docker daemon is running
      When I run "cidx run security <flag>"
      Then the execution should be quiet

      Examples:
        | flag    |
        | --quiet |
        | -q      |
