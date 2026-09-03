Feature: Release change summary
  The release preview is the last review before publishing a tag.
  When it abbreviates a long history, it must say how much history it hid.

  Rule: The abbreviated count is computed before truncation

    Scenario: More than ten commits are summarized
      Given 12 commits since the latest tag
      When I render the release change summary
      Then the release summary lists 10 commits
      And the release summary says "... and 2 more commits"

