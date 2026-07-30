Feature: Vulnerability Exception Lifecycle
  As the maintainer of the built-in preset catalogue
  I want an exception to die with the image it covers, and only then
  So that the record keeps saying something true about what we ship

  # Exceptions are recorded against `repo:tag`, and the catalogue moves. Nothing
  # ever asked whether an entry still had an object, and all 155 of them ended up
  # describing images no preset runs (#238).
  #
  # "The tag changed" is not the criterion, though. When an image is promoted an
  # accepted CVE either went away with the image it was recorded against, or it
  # followed the promotion — and only the first is a purge. The findings decide,
  # not the tags.

  Rule: An exception covering a running image is never in question

    Scenario: The catalogue still runs the image the exception was written for
      Given the catalogue runs "dhi.io/trivy:0.68"
      And every catalogue image has been scanned
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy:0.68"
      When the exception lifecycle is applied
      Then the exception should be live

  Rule: An exception whose CVE went with its image is obsolete

    Scenario: The image was replaced and the finding did not follow
      Given the catalogue runs "dhi.io/trivy:0.68"
      And every catalogue image has been scanned
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy:0.67.2"
      When the exception lifecycle is applied
      Then the exception should be obsolete
      And the exception verdict should mention "no catalogue image carries"

  Rule: An exception whose CVE survived the promotion is reported, not purged

    # Deleting this one loses the justification and leaves the next audit red on
    # a finding somebody reviewed months ago. It has to be re-filed instead.

    Scenario: The finding followed the promotion into the new image
      Given the catalogue runs "dhi.io/trivy:0.68"
      And every catalogue image has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy:0.68"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy:0.67.2"
      When the exception lifecycle is applied
      Then the exception should be carried over
      And the exception verdict should name the image "dhi.io/trivy:0.68"

    Scenario: The scanners spell the identifier differently
      Given the catalogue runs "dhi.io/trivy:0.68"
      And every catalogue image has been scanned
      And the scanners report "ghsa-cgrx-mc8f-2prm" on "dhi.io/trivy:0.68"
      And the exception "GHSA-cgrx-mc8f-2prm" was recorded against "aquasec/trivy:0.67.2"
      When the exception lifecycle is applied
      Then the exception should be carried over

  Rule: Without scan evidence nothing is concluded

    # Fail-closed, like the cooldown on an undatable candidate and the scan gate
    # on an unreadable result: a purge decided on no evidence is a guess.

    Scenario: No scanner result is available at all
      Given the catalogue runs "dhi.io/trivy:0.68"
      And no catalogue image has been scanned
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy:0.67.2"
      When the exception lifecycle is applied
      Then the exception should be unresolved
      And the exception verdict should mention "cannot be shown to have gone"

    Scenario: One catalogue image is missing from the results
      Given the catalogue runs "dhi.io/trivy:0.68"
      And the catalogue runs "golangci/golangci-lint:v2.12.2-alpine"
      And the scanners produced no result for "golangci/golangci-lint:v2.12.2-alpine"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy:0.67.2"
      When the exception lifecycle is applied
      Then the exception should be unresolved
