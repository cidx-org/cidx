Feature: Flags placed after a positional argument
  As someone typing a cidx command
  I want a flag written behind an argument to be refused, with the line to retype
  So that a command never silently does the opposite of what I asked for

  # Issue #268. urfave/cli v2 hands the command line to the standard flag
  # package, which stops parsing at the first token that is not a flag:
  # `cidx workflow run ci.yml --ref main` ran on the current branch, and
  # `cidx repo pr create "title" --dry-run` did the real thing while reporting
  # a preview. The guard refuses rather than reordering — reordering means
  # guessing on a line the parser already gave up on.

  # These scenarios type command lines against the tree cidx ships (issue #317).
  # A command that is renamed or loses a flag fails here, because the line no
  # longer resolves; nothing has to be kept in step with the CLI by hand.

  Rule: A flag written after an argument is refused, and the line to retype is shown

    Scenario: The flag that never reached the command
      When I type the command line cidx workflow run ci.yml --ref main
      Then cidx should refuse the line
      And it should suggest cidx workflow run --ref main ci.yml

    Scenario: The dry run that ran for real
      When I type the command line cidx repo pr create "feat: something" --dry-run
      Then cidx should refuse the line
      And it should suggest cidx repo pr create --dry-run "feat: something"

    Scenario: A command with no flags of its own still drops the global ones
      When I type the command line cidx preset info trivy --verbose
      Then cidx should refuse the line
      And it should suggest cidx preset info --verbose trivy

  Rule: Flags written in front are the form that works

    Scenario: The same invocation, typed the way the parser reads it
      When I type the command line cidx workflow run --ref main ci.yml
      Then cidx should accept the line

  Rule: What was never a flag is left alone

    Scenario: A flag named inside a quoted title is part of the title
      When I type the command line cidx repo pr create "fix: --dry-run is no longer ignored"
      Then cidx should accept the line

    Scenario: A negative number is a value, not a flag
      When I type the command line cidx workflow list ci.yml -3
      Then cidx should accept the line

    Scenario: An explicit terminator is how a dash-leading argument is passed
      When I type the command line cidx workflow list ci.yml -- --limit
      Then cidx should accept the line
