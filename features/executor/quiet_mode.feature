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

  Rule: Quiet mode is configurable via CLI flag

    Scenario Outline: Quiet flag variations
      Given Docker daemon is running
      When I run "cidx run security <flag>"
      Then the execution should be quiet

      Examples:
        | flag    |
        | --quiet |
        | -q      |
