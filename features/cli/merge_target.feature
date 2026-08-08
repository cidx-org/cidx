Feature: A merge lands the commit you actually have
  As someone about to merge their own pull request
  I want cidx to refuse when the branch and the remote disagree
  So that a commit only I have is never merged around and then deleted

  # The sibling of features/cli/watch_target.feature, and the dangerous half.
  #
  # `pr merge` asked the provider for the checks of whatever the remote had —
  # WaitForChecksToStart was called with an empty expected SHA — and never
  # compared it against the branch in hand. A commit that failed to push was
  # therefore merged *around*: the pull request squashed the commit before it,
  # and the work stayed local.
  #
  # Then postMergeCleanup deleted the branch that held it. `git branch -d`
  # refuses a branch carrying unmerged commits — that is git's own safety net —
  # and the cleanup falls through to `git branch -D`, which does not. So the one
  # copy of that work was destroyed, recoverable only by someone who thinks to
  # read the reflog within ninety days, and nothing in the output said so.
  #
  # Hence a refusal rather than a warning: a warning is read after the fact, and
  # by then the branch is gone.

  Rule: A merge refuses when the pull request is not on the commit in hand

    # Both directions refuse, and they are different accidents.
    #
    # Ahead — commits that never reached the remote — is the destructive one:
    # they are merged around and then deleted with the branch.
    #
    # Behind — the remote moved on — destroys nothing, but merges code the
    # reader has not seen, under their name, after their review. Telling the two
    # apart needs the remote commit to exist locally, which is exactly what is
    # not guaranteed here, so the refusal names both remedies instead of
    # guessing which one applies.

    Scenario: A commit that never reached the remote stops the merge
      Given local HEAD is "6414eb11a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the merge refuses
      And the refusal names the commit "6414eb1" as the local one
      And the refusal names the commit "a88cb3e" as the one under test

    Scenario: The refusal offers the push, for work that never left the machine
      Given local HEAD is "6414eb11a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the refusal mentions "cidx cpw"

    Scenario: The refusal offers the pull, for a remote that moved on
      Given local HEAD is "6414eb11a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the refusal mentions "git pull"

    Scenario: The refusal says the branch is deleted afterwards
      Given local HEAD is "6414eb11a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the refusal mentions "deleted"

  Rule: A merge proceeds when they agree

    Scenario: The pull request is on the commit in hand
      Given local HEAD is "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the merge proceeds

  Rule: A check that could not be made does not block a merge

    # Same posture as the watch, and for the same reason: the guard exists to
    # stop a confident wrong answer. Refusing every merge because a SHA was
    # unreadable would replace a rare accident with a permanent one.

    Scenario: An unreadable local HEAD lets the merge run, and says so
      Given local HEAD cannot be read
      And the pull request reports its head commit as "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the merge is about to land
      Then the merge proceeds
      And the merge says the commit could not be verified

    Scenario: A provider that reports no head commit lets the merge run, and says so
      Given local HEAD is "a88cb3e1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports no head commit
      When cidx checks what the merge is about to land
      Then the merge proceeds
      And the merge says the commit could not be verified
