Feature: Naming the check that failed
  As someone whose pull request has just gone red
  I want cidx to say which check failed, and where
  So that choosing between fixing, rerunning and ignoring does not start in a browser

  # Issue #347. `cidx pr status` reported "✗ 4/5 passed" and stopped there,
  # naming neither the failing check nor the reason — so the next step was
  # always the web UI or `gh`, at the exact moment `cidx repo workflow rerun
  # --failed` (#342) had just made the alternative possible from here.
  # remote.PRChecks already carried every check's name, its failing step and an
  # error excerpt; nothing read them.

  # These scenarios assert the renderers the two commands print, byte for byte.
  # That `pr watch` and `pr status` do print them is covered by the unit tests
  # of internal/commands, which drive the watch loops through a package seam.

  # Where the staged "failed step" comes from on a real run is issue #355: the
  # GitHub client reads it from the Actions job behind the check run, once per
  # failed check. That belongs to the provider, whose rules — which check suites
  # count (#240), which app's checks mean CI has started (#257) — are specified
  # by the unit tests of pkg/remote/github, not from here.

  Rule: The count is followed by the names behind it

    Background:
      Given the pull request ended with checks:
        | check        | conclusion | failed step | error                              |
        | Bootstrap    | success    |             |                                    |
        | Security     | success    |             |                                    |
        | Code Quality | success    |             |                                    |
        | Build        | skipped    |             |                                    |
        | Test         | failure    | Run tests   | Process completed with exit code 1. |

    Scenario: The status block keeps its count and gains the name behind it
      When I read the PR status
      Then the report still counts "4/5 passed"
      And the report names the failing check "Test"

    Scenario: The failing step is named when the provider reports one
      When I read the PR status
      Then the report names "Run tests" as the failing step of "Test"

    Scenario: The error excerpt is shown when the provider carries one
      When I read the PR status
      Then the report shows "Process completed with exit code 1."

    Scenario: A watch closes on the same names
      When a watch closes on those checks
      Then the report names the failing check "Test"
      And the report names "Run tests" as the failing step of "Test"

  Rule: Only what the count calls a failure is named

    # The names have to add up to the number printed next to them: a block
    # saying "4/5 passed" and listing two checks contradicts itself.

    Background:
      Given the pull request ended with checks:
        | check     | conclusion  | failed step | error |
        | Build     | skipped     |             |       |
        | Lint      | neutral     |             |       |
        | Test      | failure     |             |       |
        | E2E       | cancelled   |             |       |

    Scenario: A skipped or neutral check counts as passed
      When I read the PR status
      Then the report does not name "Build"
      And the report does not name "Lint"

    Scenario: A cancelled check counted as a failure is named as one
      When I read the PR status
      Then the report names the failing check "E2E"

  Rule: A green pull request gets no failure list

    Scenario: Nothing is added under a count with nothing behind it
      Given the pull request ended with checks:
        | check | conclusion | failed step | error |
        | Test  | success    |             |       |
      When I read the PR status
      Then the report lists no failing check

  Rule: A completed workflow run names where it failed

    # Issue #466. Reopening an already completed run returned only
    # "Workflow failed: failure", although the provider had fetched its jobs.

    Scenario: The completed run points directly at the failed step
      Given the workflow run ended with jobs:
        | job              | conclusion | failed step                                |
        | Setup            | success    |                                            |
        | Trivy            | success    |                                            |
        | Report           | failure    | Generate Report                            |
        | Promotion Policy | failure    | Fail if the catalogue is waiting on a human |
      When a workflow watch reports that completed run
      Then the report names the failing check "Report"
      And the report names "Generate Report" as the failing step of "Report"
      And the report names the failing check "Promotion Policy"
