Feature: A branch is never force-deleted over work only it holds
  As someone who runs branch cleanup without reading every line of it
  I want cidx to stop forcing a deletion git had just refused
  So that housekeeping cannot destroy the only copy of a commit

  # `git branch -d` refuses a branch that is not fully merged, and it is
  # cleverer than it looks: when the branch has an upstream it compares against
  # *that*, not against HEAD. So after a squash merge it accepts a branch whose
  # commits are all on origin, and refuses one carrying a commit that never
  # left the machine.
  #
  # In other words -d already asks the right question. What the three deletion
  # sites did was answer it and then overrule it: each fell through to
  # `git branch -D`, which asks nothing. That fallback rescues no safe case —
  # safe cases already pass -d — so it could only ever fire on the two cases
  # where deleting loses work.
  #
  # Two of the three simply stop forcing. The third, `branch cleanup`, needs a
  # decision, because it runs after the remote branch may already be gone: with
  # no upstream left to compare against, -d refuses a squash-merged branch that
  # is perfectly safe, and cleanup would stop cleaning anything up.

  Rule: A merged branch is forced only when the remote holds the same tip

    Scenario: A merged branch level with its remote is deleted
      Given the branch is merged
      And the remote holds the same commit as the local branch
      When cidx decides how to delete it
      Then the deletion is forced

    Scenario: A merged branch holding a commit the remote lacks is not forced
      Given the branch is merged
      And the local branch is ahead of the remote
      When cidx decides how to delete it
      Then the deletion is not forced

    # The residual limit, stated rather than papered over. Once GitHub has
    # deleted the remote branch there is nothing local to compare against, and
    # a squash-merged branch is indistinguishable from one holding unique work:
    # its commits are reachable from no remote ref and are not in the trunk,
    # because the squash replaced them. Only the merged verdict remains, so it
    # is what decides. Refusing here instead would make cleanup refuse every
    # branch it exists to remove, and a guard that cries wolf gets switched off.
    Scenario: A merged branch whose remote is already gone is still deleted
      Given the branch is merged
      And the remote branch no longer exists
      When cidx decides how to delete it
      Then the deletion is forced

  Rule: Anything not known to be merged is left to git

    Scenario: An unmerged branch is not forced
      Given the branch is not merged
      And the local branch is ahead of the remote
      When cidx decides how to delete it
      Then the deletion is not forced

    Scenario: --force is the one way to overrule git deliberately
      Given the branch is not merged
      And the local branch is ahead of the remote
      And --force was given
      When cidx decides how to delete it
      Then the deletion is forced
