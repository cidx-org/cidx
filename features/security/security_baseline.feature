Feature: What the published baseline says the catalogue carries
  As someone about to run the built-in preset catalogue
  I want the committed record to state everything those images carry
  So that "what do I install" is not answered by a number with a hole in it

  # `SECURITY-BASELINE.md` is committed so that its diff is the history of what
  # the catalogue delivers, and it states two numbers that are not the same:
  # what the images carry, and what has been accepted on them (#238).
  #
  # The first was being read out of scan results the acceptances had already
  # been subtracted from. `security-audit.yml` generates each image's ignore
  # file out of the entries accepted on that image's repository, so an accepted
  # finding is deleted from that image's own report by construction — and the
  # published total was "what the catalogue carries, minus what we had already
  # argued about", under a heading that said otherwise. On one day's artifacts
  # it read 447 where it should have read 465 (#310).
  #
  # Nothing could have fixed it before #311: the removed half was not in the
  # artifacts at all until the audit started passing `--show-suppressed`.

  Rule: An accepted finding is carried until the image stops carrying it

    # Accepting a finding records that somebody looked at it. It does not take
    # it out of the image, so it belongs in the number that says what the image
    # holds — and next to it, in the column that says how much of that has been
    # argued.

    Scenario: The accepted CVE the ignore file removed is still carried
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy"
      And the audit's ignore file suppressed "CVE-2026-0002" on "dhi.io/trivy"
      And the exception "CVE-2026-0002" was recorded against "dhi.io/trivy"
      When the catalogue's carried findings are counted
      Then the catalogue should carry 2 findings

    # The evidence that retires an exception is the same evidence that counts
    # it: an accepted CVE the image no longer holds is reported by neither half,
    # and the baseline must not carry it on the strength of the entry alone.
    # That is what the entry is for, not what it proves.

    Scenario: An accepted CVE the image no longer carries is counted nowhere
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0002" was recorded against "dhi.io/trivy"
      When the catalogue's carried findings are counted
      Then the catalogue should carry 1 finding

  Rule: Only what the ignore file hid is read back

    # A scanner suppresses more than it was asked to. Grype ships a default rule
    # dropping indirect `linux-libc-dev` matches — 188 of them on the Rust image
    # alone — and reading the suppressed half wholesale would move a committed
    # number by a scanner's defaults rather than by anything the catalogue
    # ships. The ignore file removes exactly the entries it was generated from,
    # so exactly those are added back.

    Scenario: A finding the scanner suppressed under its own rule is not carried
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy"
      And the audit's ignore file suppressed "CVE-2026-0500" on "dhi.io/trivy"
      When the catalogue's carried findings are counted
      Then the catalogue should carry 1 finding

    # Both scanners report most findings, and the two halves are read together.
    # A CVE one of them suppressed while the other still showed it is one thing
    # the image carries, not two.

    Scenario: A CVE one scanner suppressed and the other showed is counted once
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0002" on "dhi.io/trivy"
      And the audit's ignore file suppressed "CVE-2026-0002" on "dhi.io/trivy"
      And the exception "CVE-2026-0002" was recorded against "dhi.io/trivy"
      When the catalogue's carried findings are counted
      Then the catalogue should carry 1 finding
