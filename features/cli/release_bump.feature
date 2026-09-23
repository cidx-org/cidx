Feature: Release version bump
  As the maintainer cutting a release with `cidx release create`
  I want the bump container to be authored, read and undone by cidx itself
  So that a release either goes through or leaves the branch exactly as it was

  # Issue #484, found wiring `cidx release create` into a Rust project. The
  # flow assumed two things nothing checked: a VERSION file to read the new
  # version back from, and a git identity inside a container that runs as the
  # invoking uid with no config of its own. A missing VERSION failed *after*
  # commitizen had committed and tagged, and stranded both on the base branch.

  Rule: The bump is authored by whoever releases

    # The container has no identity of its own. The host's is the one cidx
    # already reads, and passing it as the environment git consults first means
    # nothing has to live in the repository or be mounted.

    Scenario: The host's git identity authors the bump
      Given the host git identity is "Ada Lovelace" <"ada@example.test">
      And the release action declares no git identity
      When the bump environment is resolved
      Then the bump environment sets "GIT_AUTHOR_NAME" to "Ada Lovelace"
      And the bump environment sets "GIT_AUTHOR_EMAIL" to "ada@example.test"
      And the bump environment sets "GIT_COMMITTER_NAME" to "Ada Lovelace"
      And the bump environment sets "GIT_COMMITTER_EMAIL" to "ada@example.test"

    Scenario: An identity the action declares is kept
      Given the host git identity is "Ada Lovelace" <"ada@example.test">
      And the release action declares "GIT_AUTHOR_EMAIL" as "release-bot@example.test"
      When the bump environment is resolved
      Then the bump environment sets "GIT_AUTHOR_EMAIL" to "release-bot@example.test"
      And the bump environment sets "GIT_COMMITTER_EMAIL" to "ada@example.test"

    Scenario: No identity anywhere is refused before the container runs
      Given the host has no git identity
      And the release action declares no git identity
      When the bump environment is resolved
      Then resolving the bump environment fails mentioning "git config user.email"

  Rule: The new version is read from the tag the bump created

    # A VERSION file is one project's choice of version file, not something
    # every commitizen configuration writes. The tag is: `cz bump` always tags
    # the commit it creates.

    Scenario: The version comes from the tag on the bump commit
      Given a repository with no VERSION file
      And a bump commit tagged "v1.4.0"
      When the bumped version is read
      Then the bumped version is "1.4.0"

    Scenario: A bump that created no commit is reported as such
      Given a repository with no VERSION file
      And the bump created no commit
      When the bumped version is read
      Then reading the bumped version fails mentioning "created no commit"

  Rule: A failed bump leaves the branch as it found it

    # The run starts from a clean tree, so the state before the container is
    # known exactly; whatever the container did to the branch is undone.

    Scenario: A committed and tagged bump is undone
      Given a repository with no VERSION file
      And a bump commit tagged "v1.4.0"
      When the bump is undone
      Then the branch is back where it started
      And the tag "v1.4.0" no longer exists

    Scenario: Files a failed commit left modified are restored
      Given a repository with no VERSION file
      And the bump modified tracked files without committing
      When the bump is undone
      Then the branch is back where it started
      And the working tree is clean

    Scenario: Undoing a bump that committed nothing keeps existing tags
      Given a repository with no VERSION file
      And the starting commit is tagged "v1.3.0"
      And the bump created no commit
      When the bump is undone
      Then the tag "v1.3.0" still exists
