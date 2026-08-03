Feature: The catalogue's state, published to code scanning
  As the maintainer of the built-in preset catalogue
  I want one place that says what the catalogue needs me for
  So that reading four artefacts is no longer the way to find out

  # The evidence was scattered: scan results in artifacts deleted after a day,
  # acceptances in known-vulnerabilities.toml, totals in a generated markdown
  # table, reasoning in issue threads. None of them answers "where does the
  # catalogue stand" without the other three (#301).
  #
  # GitHub's code scanning is the place, and SARIF is what it reads. The file is
  # written from the JSON the audit already uploads — not asked of the scanners a
  # second time — because what makes it readable is what it leaves out, and no
  # scanner can do that filtering.
  #
  # known-vulnerabilities.toml stays the source of truth. This is a view of it.

  Rule: Only what needs a human is published

    # 456 findings reads as a catastrophe and answers nothing. The population an
    # exception is the right instrument for — no fix at any version, not exempt
    # by class — is the one where a human has something to decide, and it is the
    # only one worth an alert somebody has to dismiss.

    Scenario: A finding with no fix anywhere is published
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0001" on "rust"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0001" should be published as an alert
      And the alert for "CVE-2026-0001" should say "No fix exists at any version"

    Scenario: A finding the publisher has already fixed is not published
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0002" on "rust", fixed in "1.2.3"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0002" should not be published as an alert

    Scenario: The Go standard library in a CLI binary is not published
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0003" on "rust" in package "stdlib" of type "gobinary"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0003" should not be published as an alert

    Scenario: Kernel headers are not published
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0004" on "rust" in package "linux-libc-dev" of type "debian"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0004" should not be published as an alert

  Rule: Both scanners are published, and each finding once

    # Of the 150 findings needing triage, 39 are seen by both scanners, 102 by
    # Grype alone and 9 by Trivy alone. A run per scanner would show 189 alerts
    # for 150 problems; one scanner would hide two thirds of them. Merging on the
    # identifier gives 150 and loses nothing — the attribution travels in the
    # alert, which is where a reader deciding what to do actually wants it.

    Scenario: A finding both scanners report is one alert naming both
      Given the catalogue runs "rust"
      And "Trivy" reports "CVE-2026-0005" on "rust"
      And "Grype" reports "CVE-2026-0005" on "rust"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0005" should be published as 1 alert
      And the alert for "CVE-2026-0005" should say "Reported by Grype and Trivy"

    Scenario: A finding only one scanner knows about is still published
      Given the catalogue runs "rust"
      And "Grype" reports "CVE-2026-0006" on "rust"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0006" should be published as an alert
      And the alert for "CVE-2026-0006" should say "Reported by Grype"

  Rule: An acceptance past its expiry date is a thing to argue again

    # An exception is written with a date so the acceptance gets argued again
    # rather than inherited. Past it the entry waives nothing (#303), and this
    # is where a reader finds out that one has lapsed.

    Scenario: An expired exception is published
      Given the exception "CVE-2026-0007" on "rust" expired on "2020-01-01"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0007" should be published as an alert
      And the alert for "CVE-2026-0007" should say "the finding is back in the audit's scan results"

    Scenario: An exception still within its date is not published
      Given the exception "CVE-2026-0008" on "rust" expires on "2999-01-01"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0008" should not be published as an alert

  Rule: One alert per repository and CVE, whatever raised it

    # The two families could not collide until the ignore file started honouring
    # `expires`: a lapsed acceptance filtered its finding out of the results, so
    # the finding never reached the triage above — which is exactly why the
    # expired alert was added. Now it does reach it, and both would fire on the
    # same CVE. Two alerts is one decision asked about twice: one of them gets
    # dismissed to clear the list, and which one is a coin toss.
    #
    # The expired one survives because it says strictly more: it carries the
    # judgement somebody already wrote and the date it lapsed, and it points at
    # the entry to re-date or delete rather than at the image pin.

    Scenario: The finding a lapsed acceptance stopped waiving is not published twice
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0012" on "rust"
      And the exception "CVE-2026-0012" on "rust" expired on "2020-01-01"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0012" should be published as 1 alert
      And the alert for "CVE-2026-0012" should point at "known-vulnerabilities.toml"

    Scenario: A lapsed acceptance silences nothing but its own CVE
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0013" on "rust"
      And the exception "CVE-2026-0014" on "rust" expired on "2020-01-01"
      When the catalogue findings are published to code scanning
      Then "CVE-2026-0013" should be published as an alert
      And the alert for "CVE-2026-0013" should point at "pkg/presets/presets.toml"

  Rule: An alert points at the line that has to change

    Scenario: A finding points at the catalogue line pinning the image
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0009" on "rust"
      When the catalogue findings are published to code scanning
      Then the alert for "CVE-2026-0009" should point at "pkg/presets/presets.toml"

    Scenario: An expired exception points at the entry that expired
      Given the exception "CVE-2026-0010" on "rust" expired on "2020-01-01"
      When the catalogue findings are published to code scanning
      Then the alert for "CVE-2026-0010" should point at "known-vulnerabilities.toml"

  Rule: An alert keeps its identity across a repin

    # The fingerprint is what GitHub matches an alert on from one run to the
    # next, and it was keyed on the pinned reference — tag and digest included.
    # Every repin therefore changed the identity of every alert on that image:
    # the tab closed all of them and opened near-identical replacements,
    # including for CVEs that never went away (#313). It is keyed on the
    # repository instead, exactly as the rule identifier and the exception are.
    # The reference stays in the message, where it informs rather than identifies.

    Scenario: Repinning an image does not close the alerts that survived it
      Given the catalogue runs "rust:1.97.0-slim"
      And the scanners report "CVE-2026-0011" on "rust:1.97.0-slim"
      When the catalogue findings are published to code scanning
      And "rust:1.97.0-slim" is repinned to "rust:1.98.0-slim"
      And the catalogue findings are published to code scanning
      Then "CVE-2026-0011" should be published as 1 alert
      And the alert for "CVE-2026-0011" should have kept its identity
      And the alert for "CVE-2026-0011" should say "rust:1.98.0-slim"

  Rule: An image nobody scanned carries no alerts and is not thereby clean

    # The absent number is not a zero — the confusion SECURITY-BASELINE.md was
    # written to remove, and it would be worse here, where a scan leg that died
    # would read as an image that got cleaner overnight.

    Scenario: An unscanned image is named rather than counted as clean
      Given the catalogue runs "rust"
      And the scanners produced no result for "rust"
      When the catalogue findings are published to code scanning
      Then no alert should be published
      And "rust" should be reported as unscanned
