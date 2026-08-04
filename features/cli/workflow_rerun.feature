Feature: Restarting a run and finding the runs of a branch
  As someone whose CI has just failed on something that is not the change
  I want to restart the failed job and see what ran on this branch
  So that recovery does not start by leaving the tool

  # Issue #342. A Test job died pulling a pinned image — `read: connection reset
  # by peer`, pure infrastructure — and the way back was `gh run rerun --failed`,
  # because `cidx repo workflow run` only dispatches and `ci.yml` declares no
  # workflow_dispatch. The other half: `cidx repo workflow list` demanded a
  # workflow name, which is the one thing you do not have when a check has just
  # failed and you only know the job's name.

  Rule: Only the jobs that failed are restarted

    Background:
      Given run "724" ended with jobs:
        | job      | conclusion |
        | Security | success    |
        | Test     | failure    |

    Scenario: The flake is retried and the passing jobs are not re-paid for
      When I rerun the failed jobs of run "724"
      Then the failed jobs of run "724" are restarted

    Scenario: Without the flag the whole run is restarted, as gh run rerun does
      When I rerun run "724"
      Then every job of run "724" is restarted

  Rule: A rerun that cannot do anything says which command can

    Scenario: A green run has no failed job to restart
      Given run "725" ended with jobs:
        | job  | conclusion |
        | Test | success    |
      When I rerun the failed jobs of run "725"
      Then the rerun is refused
      And the refusal names "cidx repo workflow rerun 725"
      And no rerun is requested from the provider

    Scenario: The run number is not the run identifier
      Given run "640" is unknown to the provider
      When I rerun the failed jobs of run "640"
      Then the rerun is refused
      And the refusal names "cidx repo workflow list"

  Rule: The listing answers "what ran on this branch" without being told the workflow

    Scenario: No workflow name means every workflow
      When I list the runs of branch "main"
      Then the provider is asked for every workflow on "main"
      And the listing names the workflow of each run

    Scenario: A workflow name still lists that workflow
      When I list the runs of workflow "ci"
      Then the provider is asked for workflow "ci" on every branch

  Rule: The flag guard covers these commands like every other

    # The issue writes the invocation as `rerun <id> [--failed]`, which is the
    # order the parser gives up on: the flag lands behind the argument and never
    # reaches the command, so the run would be restarted whole. Refused, with the
    # line to retype (issue #268).
    Scenario: --failed written behind the run ID is refused
      When I type the command line cidx repo workflow rerun 18234567890 --failed
      Then cidx should refuse the line
      And it should suggest cidx repo workflow rerun --failed 18234567890

    Scenario: The same invocation, typed the way the parser reads it
      When I type the command line cidx repo workflow rerun --failed 18234567890
      Then cidx should accept the line

    Scenario: The branch listing takes no argument at all
      When I type the command line cidx repo workflow list --branch main
      Then cidx should accept the line
