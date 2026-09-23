Feature: Release housekeeping
  As the maintainer cutting a release with cidx
  I want cidx's own plumbing to stay out of what the release says and leaves behind
  So that the notes describe the changes and main is clean once the release is out

  # Both found cutting v3.4.3.

  Rule: The commit that opens a PR branch is not a change (#488)

    # `cidx pr create` commits an empty "chore: initialize PR branch for …" so a
    # PR can exist before any work. It is squashed away on merge, but a notes
    # PR prepared on its own branch saw it and listed it under Maintenance.

    Scenario: The PR-branch initialization commit is left out of the history
      Given a commit log holding "fix(release): undo a failed bump" and the commit that opened the PR branch
      When the commit log is parsed
      Then the parsed history holds "undo a failed bump"
      And the parsed history does not hold the commit that opened the PR branch

  Rule: A release removes its scratch files, never its record (#489)

    # The version file is gitignored scratch. The notes file is committed
    # through the prepare PR and stays in the repository like every previous
    # release's; deleting it left main with an unstaged deletion after a
    # successful release.

    Scenario: Committed release notes survive the cleanup
      Given a repository whose release notes for "3.4.3" are committed
      When the prepared release files are cleaned up
      Then the release notes for "3.4.3" still exist
      And the working tree is clean

    Scenario: Uncommitted release notes are removed by the cleanup
      Given a repository whose release notes for "3.4.3" were prepared but not committed
      When the prepared release files are cleaned up
      Then the release notes for "3.4.3" no longer exist
