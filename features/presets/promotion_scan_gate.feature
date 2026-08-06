Feature: Catalogue Image Promotion Scan Gate
  As the maintainer of the built-in preset catalogue
  I want a candidate held only when it introduces a vulnerability we have not accepted
  So that the gate blocks real regressions instead of failing on every image we knowingly run

  # Several catalogue images are knowingly vulnerable — known-vulnerabilities.toml
  # records exactly that — so "the candidate has findings" would hold every one of
  # them for ever. The verdict is differential: what the candidate adds to what we
  # already run (#247).
  #
  # "What we already run" is two records, and the distinction is the whole of
  # issue #379. The acceptances file is what a human has argued; the running
  # image's own same-day scan is what is actually carried. The gate used to
  # consult only the file, on the assumption that a green audit kept the two
  # identical — and with the audit red, every CVE the database had learned since
  # the file was written read as "introduced" by candidates that also carried
  # none of them: rust:1.97.1-slim was held for 80 CVEs, 80 of which the running
  # rust:1.97.0-slim reported the same day. The baseline is the union of both
  # records now, so a candidate is held only for what it truly adds.

  Rule: A candidate that introduces nothing new is promoted

    Scenario: A candidate with no finding clears the gate
      Given the scanners report no finding on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate
      And the scan verdict should mention "no HIGH/CRITICAL finding"

    Scenario: A finding the running image already carries is not a regression
      Given the finding "CVE-2026-0001" is accepted for the candidate
      And the scanners report "CVE-2026-0001" on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate
      And the scan verdict should mention "already carried or accepted"

    # The 80/80 case of issue #379: the database learned a CVE after the file
    # was last argued, so it is on both sides of the promotion and on file
    # nowhere. Nobody has judged it — that is the audit's backlog — but the
    # promotion does not change it, and holding the upgrade over it is how the
    # catalogue froze for months.
    Scenario: A finding the database learned after the file was written is not the candidate's
      Given the running image's scan reports "CVE-2026-0400"
      And the scanners report "CVE-2026-0400" on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate
      And the scan verdict should mention "already carried or accepted"

    Scenario: A finding accepted ahead of the promotion does not block it
      Given the running image has no known vulnerabilities
      And the finding "CVE-2026-0009" is accepted for the candidate
      And the scanners report "CVE-2026-0009" on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate

  Rule: A candidate that introduces an unaccepted finding is held

    Scenario: The new finding is named and the inherited one is not blamed
      Given the running image's scan reports "CVE-2026-0001"
      And the scanners report "CVE-2026-0001" on the candidate
      And the scanners report "CVE-2026-0002" on the candidate
      When the scan gate is applied
      Then the candidate should be held by the scan gate
      And the scan verdict should name "CVE-2026-0002"
      And the scan verdict should not name "CVE-2026-0001"

  Rule: Without the running image's scan, only the file vouches

    # Fail-closed (#379): a missing or unreadable scan of the running image
    # falls back to the acceptances alone — the stricter baseline, never the
    # permissive one. A promotion is still never taken on an assumption.
    Scenario: An unreadable current-image scan does not widen the baseline
      Given the running image's scan is unreadable
      And the scanners report "CVE-2026-0400" on the candidate
      When the scan gate is applied
      Then the candidate should be held by the scan gate
      And the scan verdict should name "CVE-2026-0400"

  Rule: The cooldown and the scan gate are cumulative

    # The cooldown runs first, in `cidx preset scan-targets`: only a candidate it
    # clears is scanned as a candidate at all. The scan gate then judges what came
    # back. Both have to pass.

    Scenario: A cooldown waiver does not excuse a new finding
      Given a candidate version published 3 days ago
      And the running image is affected by "CVE-2026-0001"
      And the scanners report "CVE-2026-0009" on the candidate
      When the promotion policy is applied
      And the scan gate is applied
      Then the candidate should be promoted
      And the waiver should name "CVE-2026-0001"
      But the candidate should be held by the scan gate
      And the scan verdict should name "CVE-2026-0009"

    Scenario: A waived candidate that brings nothing new is promoted
      Given a candidate version published 3 days ago
      And the running image is affected by "CVE-2026-0001"
      And the scanners report "CVE-2026-0001" on the candidate
      When the promotion policy is applied
      And the scan gate is applied
      Then the candidate should be promoted
      And the candidate should clear the scan gate
