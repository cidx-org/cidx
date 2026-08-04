Feature: A dry run changes nothing and needs nothing
  As someone about to open a pull request
  I want --dry-run to describe the work without doing any of it
  So that I can ask the question on a repository I do not want touched, from a train

  # Issue #276, found while fixing #268. `cidx pr create --dry-run` pulled from
  # the remote before it reached the dry-run branch, so the preview did two
  # things it must never do: it fast-forwarded the checked-out branch, and it
  # required the network to answer at all. #227 had just made the release
  # commands resolve their remote lazily for the same reason -- this is the same
  # shape, one command over.

  # The repository these scenarios build has a remote that does not exist, so a
  # command that reaches for it fails here rather than passing on a machine that
  # happens to be online.

  Rule: The preview reports the pull instead of performing it

    Scenario: Previewing a branch and a pull request with no remote to talk to
      Given a repository on main whose remote does not answer
      When I preview a pull request titled "feat: something"
      Then the preview should succeed
      And it should report that it would pull the latest changes from main
      And it should report the branch "feat/something" it would create

  Rule: The repository is where it was afterwards

    Scenario: The checked-out commit does not move
      Given a repository on main whose remote does not answer
      When I preview a pull request titled "feat: something"
      Then the checked-out commit should be the one it started on
      And no branch "feat/something" should exist
