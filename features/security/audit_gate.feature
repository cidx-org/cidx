Feature: The audit gate fails on how long a finding has waited
  As the maintainer of the built-in preset catalogue
  I want a finding published this morning to be listed without failing the audit
  So that the audit is red for what is overdue, not for how fast advisories arrive

  # Three audits in one morning found 70, then 4, then 8 findings, each new batch
  # a set of CVE published after the previous scan (#524). The gate asked "is
  # anything unanswered?", which on images that all carry a Debian or Alpine base
  # layer is true almost every day: a red nobody can act on the same day, which is
  # read like a green.
  #
  # The window is opt-in: without one the gate is exactly what it was. With one,
  # the page still lists every finding waiting -- the grace delays the failure,
  # never the visibility -- and names how many are past the window. Where the age
  # comes from (the date the audit first reported it) and how long the window is
  # are inputs, not rules: the rule is what happens once they are known.
  #
  # The days below are counted the way an acceptance's expiry is: the last day of
  # the window is still inside it.

  Rule: A finding is given a window before it fails the audit

    Scenario: A finding inside the window is listed and does not fail the audit
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0201" on "rust"
      And the audit gives a finding 7 days
      And "CVE-2026-0201" on "rust" was first reported 3 days ago
      When the audit gate is evaluated
      Then the audit should pass
      And the summary should say 1 finding needs a judgement

    Scenario: A finding past the window fails the audit
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0202" on "rust"
      And the audit gives a finding 7 days
      And "CVE-2026-0202" on "rust" was first reported 8 days ago
      When the audit gate is evaluated
      Then the audit should fail
      And the summary should say 1 finding is past the 7-day window

    Scenario: The last day of the window is still inside it
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0203" on "rust"
      And the audit gives a finding 7 days
      And "CVE-2026-0203" on "rust" was first reported 7 days ago
      When the audit gate is evaluated
      Then the audit should pass

    Scenario: A finding the source has no date for is new today
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0204" on "rust"
      And the audit gives a finding 7 days
      And the first-seen source answered with nothing
      When the audit gate is evaluated
      Then the audit should pass

  Rule: Some things do not wait

    Scenario: A known-exploited finding gets no window
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0205" on "rust" as actively exploited
      And the audit gives a finding 7 days
      And the first-seen source answered with nothing
      When the audit gate is evaluated
      Then the audit should fail

    Scenario: An acceptance past its date gets no window
      Given the catalogue runs "rust"
      And every catalogue repository has been scanned
      And the exception "CVE-2026-0206" on "rust" expired on "2020-01-01"
      And the audit gives a finding 7 days
      And the first-seen source answered with nothing
      When the audit gate is evaluated
      Then the audit should fail

  Rule: An age nobody can establish fails closed

    Scenario: A first-seen source that could not be read fails the audit
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0207" on "rust"
      And the audit gives a finding 7 days
      And the first-seen source could not be read
      When the audit gate is evaluated
      Then the audit should fail

  Rule: Without a window the gate is what it was

    Scenario: A finding published today fails the audit when no window is given
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0208" on "rust"
      When the audit gate is evaluated
      Then the audit should fail
