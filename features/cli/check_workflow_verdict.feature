Feature: One channel per audience for the verdict of a check
  As someone reading the conclusion of a check, or scripting on it
  I want that conclusion printed once, in one place
  So that what is on screen and what a pipe captures are the same string

  # Issue #345. `cidx check workflow <name>` emitted its summary twice: once
  # through logrus to stderr, once plain to stdout, in two different formats.
  # The line the user is meant to read arrived in duplicate, and a script
  # capturing stdout got a string that was not the one displayed. Cosmetic, but
  # it is the conclusion of the command — the one line that has to be
  # unambiguous. Present before the #317 refactor, introduced around #318/#339.

  # These scenarios run the real command against the tree cidx ships (#317), in
  # a project written for the scenario. Nothing reaches the network.

  Rule: The verdict is printed once, and only on stdout

    Scenario: A pipeline in sync signs off once
      Given a project whose "ci" pipeline runs phases "security, code"
      And its workflow runs phases "security, code"
      When I run cidx check workflow ci
      Then stdout says "Pipeline 'ci' is in sync with its workflow" once
      And nothing is logged

    Scenario: A pipeline that has drifted reports it once
      Given a project whose "ci" pipeline runs phases "security, code"
      And its workflow runs phases "security"
      When I run cidx check workflow ci
      Then stdout says "Pipeline 'ci' has differences with its workflow" once
      And nothing is logged

    Scenario: The sweep summarises the repository once
      Given a project whose "ci" pipeline runs phases "security, code"
      And its workflow runs phases "security, code"
      When I run cidx check workflow
      Then stdout says "All 1 workflow(s) are in sync with pipelines" once
      And nothing is logged

  Rule: What is not the verdict follows the same rule

    Scenario: A sweep with nothing to check says so once
      Given a project whose "ci" pipeline runs phases "security, code"
      And it has no workflow at all
      When I run cidx check workflow
      Then stdout says "No GitHub Actions workflows found" once
      And nothing is logged

    Scenario: A missing workflow file is reported by the error, not also logged
      Given a project whose "ci" pipeline runs phases "security, code"
      And it has no workflow at all
      When I run cidx check workflow ci
      Then the command fails
      And nothing is logged
