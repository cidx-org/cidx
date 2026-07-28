Feature: Catalogue Image Promotion Cooldown
  As the maintainer of the built-in preset catalogue
  I want a newly published version to sit in public for 14 days before it is promoted
  So that a compromise published upstream has time to be noticed before we run it

  Background:
    Given the running image has no known vulnerabilities

  Rule: A candidate is promoted once it has been public for the cooldown

    Scenario: A candidate older than the cooldown is promoted
      Given a candidate version published 20 days ago
      When the promotion policy is applied
      Then the candidate should be promoted
      And the promotion reason should mention "past the 14-day cooldown"

    Scenario: A candidate younger than the cooldown is held
      Given a candidate version published 3 days ago
      When the promotion policy is applied
      Then the candidate should be held
      And the promotion reason should mention "11 days"

    Scenario: A candidate published exactly at the cooldown is promoted
      Given a candidate version published 14 days ago
      When the promotion policy is applied
      Then the candidate should be promoted

  Rule: A candidate that cannot be dated is never promoted

    Scenario: An undatable candidate is held rather than assumed old enough
      Given a candidate version with no publication date
      When the promotion policy is applied
      Then the candidate should be held
      And the promotion reason should mention "no publication date"
      And the candidate age should be unreported

  Rule: The cooldown is waived for a version that fixes a vulnerability affecting us

    Scenario: A fresh candidate is promoted when the running image is knowingly vulnerable
      Given a candidate version published 3 days ago
      And the running image is affected by "CVE-2026-0001"
      When the promotion policy is applied
      Then the candidate should be promoted
      And the waiver should name "CVE-2026-0001"

    Scenario: An undatable candidate is promoted when the running image is knowingly vulnerable
      Given a candidate version with no publication date
      And the running image is affected by "CVE-2026-0001"
      When the promotion policy is applied
      Then the candidate should be promoted
      And the waiver should name "CVE-2026-0001"

    Scenario: A candidate that served the cooldown claims no waiver
      Given a candidate version published 20 days ago
      And the running image is affected by "CVE-2026-0001"
      When the promotion policy is applied
      Then the candidate should be promoted
      And no waiver should be reported
