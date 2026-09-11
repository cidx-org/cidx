Feature: Follow the commit produced by a merge
  A successful run of an older commit must not validate a new merge.

  Scenario: Ignore an old preferred workflow and follow the merged commit
    Given a completed old cidx workflow and a CI run for the merged commit
    When CIDX waits for the post-merge workflow
    Then it selects the CI run for the merged commit

  Scenario: Wait for the merged commit to appear
    Given the merged commit workflow appears after the first lookup
    When CIDX waits for the post-merge workflow
    Then it selects the CI run for the merged commit

  Scenario: Refuse an unverifiable merge target
    Given the merge response has no commit
    When CIDX waits for the post-merge workflow
    Then post-merge verification fails

  Scenario: Never report an older run as success on timeout
    Given only an older successful workflow exists
    When CIDX waits for the post-merge workflow
    Then post-merge verification fails
