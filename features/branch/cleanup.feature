Feature: Scoped branch cleanup
  As a developer tidying up after a merge
  I want cleanup to touch the branch I named and nothing else
  So that removing one branch cannot remove seventeen (issue #269)

  Background:
    Given the repository has branches:
      | name       | status    | pr |
      | main       | protected |    |
      | feat/one   | merged    |    |
      | feat/two   | merged    |    |
      | feat/three | merged    |    |

  Rule: A cleanup run is scoped to one branch unless a sweep is asked for

    Scenario: Standing on a merged branch does not reach the ones beside it
      Given the current branch is "feat/one"
      When I clean up branches
      Then no branch is deleted
      And branch "feat/one" is kept because "current branch"

    Scenario: --branch names the branch to delete
      Given the current branch is "main"
      When I clean up branches with "--branch feat/two"
      Then branches "feat/two" are deleted

    Scenario: On main there is nothing to clean up
      Given the current branch is "main"
      When I clean up branches
      Then no branch is deleted
      And branch "main" is kept because "protected branch"

    Scenario: A branch that does not exist is named as such
      Given the current branch is "main"
      When I clean up branches with "--branch feat/nope"
      Then the cleanup fails with "feat/nope"

    Scenario: --all still sweeps every merged branch
      Given the current branch is "main"
      When I clean up branches with "--all"
      Then branches "feat/one, feat/two, feat/three" are deleted

    Scenario: --all leaves the branch it is standing on
      Given the current branch is "feat/one"
      When I clean up branches with "--all"
      Then branches "feat/two, feat/three" are deleted
      And branch "feat/one" is kept because "current branch"

  Rule: A named branch goes only when the repository is finished with it

    Scenario: A branch merged into main goes
      Given the current branch is "main"
      When I clean up branches with "--branch feat/two"
      Then branches "feat/two" are deleted

    Scenario: A branch whose PR was closed without merging is an orphan, and goes
      Given the repository has branches:
        | name         | status | pr        |
        | feat/dropped | orphan | 43 closed |
      And the current branch is "main"
      When I clean up branches with "--branch feat/dropped"
      Then branches "feat/dropped" are deleted

    Scenario: A branch whose PR is still open is refused
      Given the repository has branches:
        | name     | status | pr      |
        | feat/wip | active | 42 open |
      And the current branch is "main"
      When I clean up branches with "--branch feat/wip"
      Then no branch is deleted
      And branch "feat/wip" is kept because "PR #42 is still open"

    Scenario: --force deletes a branch with an open PR
      Given the repository has branches:
        | name     | status | pr      |
        | feat/wip | active | 42 open |
      And the current branch is "main"
      When I clean up branches with "--branch feat/wip --force"
      Then branches "feat/wip" are deleted

    Scenario: A branch nothing has finished with is refused
      Given the repository has branches:
        | name      | status | pr |
        | feat/solo | active |    |
      And the current branch is "main"
      When I clean up branches with "--branch feat/solo"
      Then no branch is deleted
      And branch "feat/solo" is kept because "not merged"

    Scenario: --force does not delete a protected branch
      Given the current branch is "feat/one"
      When I clean up branches with "--branch main --force"
      Then no branch is deleted
      And branch "main" is kept because "protected branch"
