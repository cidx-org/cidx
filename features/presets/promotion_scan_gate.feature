Feature: Catalogue Image Promotion Scan Gate
  As the maintainer of the built-in preset catalogue
  I want a candidate held only when it introduces a vulnerability we have not accepted
  So that the gate blocks real regressions instead of failing on every image we knowingly run

  # Several catalogue images are knowingly vulnerable — known-vulnerabilities.toml
  # records exactly that — so "the candidate has findings" would hold every one of
  # them for ever. The verdict is differential: what the candidate adds to what we
  # already run (#247).

  Rule: A candidate that introduces nothing new is promoted

    Scenario: A candidate with no finding clears the gate
      Given the scanners report no finding on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate
      And the scan verdict should mention "no HIGH/CRITICAL finding"

    Scenario: A finding the running image already carries is not a regression
      Given the running image is affected by "CVE-2026-0001"
      And the scanners report "CVE-2026-0001" on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate
      And the scan verdict should mention "already accepted"

    Scenario: A finding accepted ahead of the promotion does not block it
      Given the running image has no known vulnerabilities
      And the finding "CVE-2026-0009" is accepted for the candidate
      And the scanners report "CVE-2026-0009" on the candidate
      When the scan gate is applied
      Then the candidate should clear the scan gate

  Rule: A candidate that introduces an unaccepted finding is held

    Scenario: The new finding is named and the inherited one is not blamed
      Given the running image is affected by "CVE-2026-0001"
      And the scanners report "CVE-2026-0001" on the candidate
      And the scanners report "CVE-2026-0002" on the candidate
      When the scan gate is applied
      Then the candidate should be held by the scan gate
      And the scan verdict should name "CVE-2026-0002"
      And the scan verdict should not name "CVE-2026-0001"

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
