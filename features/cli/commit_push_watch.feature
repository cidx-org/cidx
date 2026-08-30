Feature: cpw pushes what it has, not only what it just made
  As someone whose commits are already written but not yet on the remote
  I want cpw to push them rather than stop
  So that the branch I am watching is the branch I am on

  # This is the cause behind #414 and #415, which caught its consequences.
  #
  # `cidx cpw` is commit, push, watch. Its first step asked whether there was
  # anything to commit and returned when there was not — before ever reaching
  # the push. A branch whose commits were already written and whose tree was
  # clean therefore never left the machine, and cpw said "No changes to commit",
  # which is true and reads like "nothing to do".
  #
  # Everything downstream then behaved correctly on the wrong commit: the
  # provider reported checks for the commit before, and a watch called it green.
  # That happened on this repository — a security fix was reported passing on
  # its parent, and the invalid YAML it introduced went unnoticed.
  #
  # The two guards refuse those answers now. This is the reason they rarely have
  # to: what cpw has to push, cpw pushes.

  Rule: Nothing to commit is not the same as nothing to do

    Scenario: Commits the remote has not seen are pushed
      Given the working tree has nothing to commit
      And the branch has commits the remote has not seen
      When cidx plans what cpw will do
      Then cpw pushes without committing
      And cpw says it is pushing commits that never reached the remote

    Scenario: A tree with changes is committed and pushed, as before
      Given the working tree has changes
      And the branch has commits the remote has not seen
      When cidx plans what cpw will do
      Then cpw commits and pushes

    Scenario: Changes are committed even when the branch is level with the remote
      Given the working tree has changes
      And the branch is level with the remote
      When cidx plans what cpw will do
      Then cpw commits and pushes

    Scenario: With nothing to commit and nothing to push, cpw resumes the current PR
      Given the working tree has nothing to commit
      And the branch is level with the remote
      When cidx plans what cpw will do
      Then cpw watches without pushing
      And cpw says it is resuming the current PR

  Rule: What reaches CI is checked first, whether or not it was just committed

    # The code phase is the gate on what CI is about to run (#307), so a push of
    # commits written by hand earns it as much as a push of one cpw just made.
    # `--no-verify` remains the escape hatch, spelled the way git spells it.

    Scenario: A push-only run still runs the code phase
      Given the working tree has nothing to commit
      And the branch has commits the remote has not seen
      When cidx plans what cpw will do
      Then cpw runs the code phase first

    Scenario: A watch-only run runs no code phase
      Given the working tree has nothing to commit
      And the branch is level with the remote
      When cidx plans what cpw will do
      Then cpw runs no code phase

  # Where "has commits the remote has not seen" comes from is a git question,
  # answered by Repository.HasUnpushedCommits and specified by the unit tests of
  # pkg/vcs against real repositories. It is deliberately not a scenario here:
  # this suite runs without a container runtime and without a remote, and a
  # rule that needed either would be the one nobody can run.
  #
  # The case those tests exist for is the branch that was never pushed. `git
  # rev-list @{u}..HEAD` has no upstream to resolve there and errors rather than
  # answering zero, and reading that as "nothing to push" would strand exactly
  # the branches most in need of one.
