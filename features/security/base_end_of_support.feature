Feature: End of support for the base an image is built on
  As the maintainer of the built-in preset catalogue
  I want to know when the distribution under an image stops being supported
  So that I repin before its findings become permanent, rather than after

  # A CVE scan says what is broken in an image today. It does not say whether
  # anything will ever fix it. Once the distribution underneath stops being
  # supported, its packages receive no further security updates: every finding
  # on that image is permanent, however fresh the tag is and however diligently
  # the cooldown promotes it.
  #
  # This is the third signal of the same family. `missing` (#245) catches a
  # pinned reference deleted upstream; `frozen_variant` (#252) catches a variant
  # line the publisher stopped building. Both describe an image that still pulls
  # and will never improve. This one is the widest of the three, because it is a
  # property of the base rather than of the tag — and the catalogue has already
  # been caught by the quiet version of it once.
  #
  # The input costs nothing: Trivy reports the base of every image it scans, in
  # `Metadata.OS`, and the daily audit already scans them all.

  Rule: A base still receiving updates says nothing

    # Reporting an end of support two years out is a fact about the calendar.
    # Nothing is decided by knowing it, and printing it every day is how a
    # section trains its reader to skip it.

    Scenario: The catalogue's Debian, two years from its date
      Given the image is built on "debian" "13.6"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as supported
      And the report should name the date "2028-08-09"

    Scenario: A base one day beyond the warning threshold
      Given the image is built on "alpine" "3.21.4"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as supported

  Rule: A base about to stop being updated is worth planning for

    # Getting off an abandoned base is a repin by hand — the replacement has to
    # exist, to work, and to be scanned before it is trusted. A quarter is the
    # smallest window that leaves room to schedule that rather than rush it, and
    # it survives the slowest loop that could act on it: the container monitor
    # runs weekly, so it is roughly thirteen chances to notice.

    Scenario: A base exactly at the warning threshold
      Given the image is built on "alpine" "3.21.4"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-03"
      Then the base should be reported as approaching end of support
      And the report should ask for attention

    Scenario: The last day of support has not passed yet
      Given the image is built on "alpine" "3.21.4"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-11-01"
      Then the base should be reported as approaching end of support

  Rule: A base past its date is the signal this exists for

    # Measured on the catalogue: `probatum:0.2.1` is built on Alpine 3.20, whose
    # support ended on 2026-04-01. The tag is current, the digest resolves, the
    # scanners run — and nothing said the findings on it are now permanent.

    Scenario: An image whose base stopped being supported months ago
      Given the image is built on "alpine" "3.20.10"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as past end of support
      And the report should ask for attention
      And the report should explain that its findings will never be fixed

    Scenario: The day after support ended
      Given the image is built on "alpine" "3.21.4"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-11-02"
      Then the base should be reported as past end of support

  Rule: A base nothing can resolve is reported, never assumed supported

    # The same posture as the rest of the supply-chain policy: an unresolvable
    # digest, an undatable candidate and an unreadable scan all refuse rather
    # than assume. A family this code does not map is a gap in this code, one
    # line away from being closed, and silence would hide it for ever.

    Scenario: An OS family the catalogue has never run
      Given the image is built on "ubuntu" "24.04"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as unresolvable
      And the report should ask for attention
      And the report should name the family "ubuntu"

    Scenario: A version no published release line covers
      Given the image is built on "alpine" "3.99.0"
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as unresolvable

  Rule: An image with no distribution base has nothing to lose

    # kaniko, ruff and shellcheck are scratch or static builds: Trivy reports no
    # `Metadata.OS` at all for them. That is an answer, not a gap, and it must
    # not be filed with the bases nothing could resolve.

    Scenario: An image built on scratch
      Given the image is built on nothing at all
      And endoflife.date publishes the release lines it publishes today
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as having no distribution base
      And the report should not ask for attention

  Rule: An endoflife.date outage never fails anything

    # The check is a convenience on top of a scan that already happened. An
    # outage on somebody else's API is not a reason to fail a scan or a monitor
    # run — it is a reason to say the check did not happen. It is the one
    # outcome that says nothing about the catalogue, so it asks for nothing.

    Scenario: endoflife.date does not answer
      Given the image is built on "alpine" "3.20.10"
      And endoflife.date does not answer
      When the base is checked for end of support on "2026-08-02"
      Then the base should be reported as unchecked
      And the report should not ask for attention
      And the report should explain that the check did not happen
