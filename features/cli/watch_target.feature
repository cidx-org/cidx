Feature: A watch reports on the commit you actually have
  As someone who just asked cidx whether their work passes CI
  I want the watch to refuse when it is looking at a different commit
  So that "all checks passed" is never an answer about somebody else's code

  # A watch resolves the pull request for the current branch and reports the
  # checks the provider holds for it. Those checks belong to whatever commit the
  # remote has — which is the same commit as local HEAD only when the push
  # actually happened.
  #
  # When it did not, every layer behaves correctly and the answer is still
  # wrong: `cpw` exits on "No changes to commit" before ever reaching its push
  # step, the branch keeps its local commits, the provider reports green for the
  # commit before them, and the watch prints "All checks passed". Nothing errors.
  # The user is told their work is green when CI has never seen it.
  #
  # That is the shape this repository keeps finding: a signal that reports green
  # because it is looking at something other than what the reader believes. The
  # scan gate answered it by refusing a verdict without evidence; this is the
  # same answer applied to a watch — the commit under test is stated, and when
  # it is not the commit in hand the watch refuses rather than reporting.

  # Deliberately out of scope: an unclean working tree. Uncommitted changes also
  # mean CI is not testing what you have, but they are visible in `git status`
  # and they are the reader's own doing. A commit that never left the machine is
  # invisible from every side, which is what earns the refusal.

  Rule: A watch states the commit it is reporting on

    # The refusal below is only legible if the ordinary case says which commit
    # it is watching. Naming it also makes the mismatch visible to anyone
    # reading scrollback afterwards, which is how this was eventually caught.

    Scenario: The commit under test is named before any check is reported
      Given local HEAD is "c54d85c1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "c54d85c1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the watch is about to report on
      Then the watch proceeds
      And the watch names the commit "c54d85c"

  Rule: A watch refuses when the commit under test is not the one in hand

    Background:
      Given local HEAD is "c11466b1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports its head commit as "dd86dc41a2b3c4d5e6f7089a0b1c2d3e4f506172"

    Scenario: A commit that never reached the remote stops the watch
      When cidx checks what the watch is about to report on
      Then the watch refuses

    Scenario: The refusal names both commits, so the gap is visible
      When cidx checks what the watch is about to report on
      Then the refusal names the commit "c11466b" as the local one
      And the refusal names the commit "dd86dc4" as the one under test

    Scenario: The refusal says what to do about it
      When cidx checks what the watch is about to report on
      Then the refusal mentions "cidx cpw"

  Rule: A check that could not be made is stated, never assumed either way

    # The guard exists to stop a confident wrong answer. "I could not tell" is
    # not one, so it does not refuse — a watch is still the useful thing to run.
    # What it must not do is stay quiet, which would make an unverified report
    # indistinguishable from a verified one.

    Scenario: An unreadable local HEAD lets the watch run, and says so
      Given local HEAD cannot be read
      And the pull request reports its head commit as "dd86dc41a2b3c4d5e6f7089a0b1c2d3e4f506172"
      When cidx checks what the watch is about to report on
      Then the watch proceeds
      And the watch says the commit could not be verified

    Scenario: A provider that reports no head commit lets the watch run, and says so
      Given local HEAD is "c11466b1a2b3c4d5e6f7089a0b1c2d3e4f506172"
      And the pull request reports no head commit
      When cidx checks what the watch is about to report on
      Then the watch proceeds
      And the watch says the commit could not be verified
