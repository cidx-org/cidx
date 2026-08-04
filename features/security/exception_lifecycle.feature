Feature: Vulnerability Exception Lifecycle
  As the maintainer of the built-in preset catalogue
  I want an exception to die with its CVE, and only then
  So that the record keeps saying something true about what we ship

  # An exception records a judgement about a CVE in an image, in our usage. That
  # judgement survives a version bump, because none of what it rests on changed —
  # so it is keyed by repository and CVE, and the tag it was first seen on is
  # context, not identity. Keying it to `repo:tag` was the original design and it
  # was wrong: all 155 entries ended up describing images no preset runs (#238).
  #
  # "The tag changed" is not the criterion either. When an image is promoted an
  # accepted CVE either went away with the image it was recorded against, or it
  # followed the promotion — and only the first is a purge. The findings decide,
  # not the tags.

  Rule: An exception on a running repository lives exactly as long as its CVE

    # The audit builds its ignore file from these very entries, so an accepted
    # CVE is deleted from its own repository's scan results by construction.
    # Reading that absence as "gone" would purge every exception doing its job —
    # which is why the repository match used to answer live before consulting
    # the findings at all, and why an exception could then never be retired by a
    # repin of the same repository (#311). The evidence is what the scanners
    # recorded as suppressed: Grype in `ignoredMatches`, Trivy in
    # `ExperimentalModifiedFindings` under `--show-suppressed`.

    Scenario: The catalogue still runs the repository and still carries the CVE
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be live

    Scenario: The tag moved under the exception
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be live

    # The monitor generates no ignore file at all, so its results show
    # everything and suppress nothing. A CVE visible there is carried just as
    # plainly, and the lifecycle must not need to know which workflow produced
    # the artifacts it was pointed at.

    Scenario: The results were not filtered at all
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be live

    # The case the lifecycle could not close. A repin of the same repository is
    # the most ordinary way for an accepted CVE to disappear, and it changes
    # nothing about the repository — so nothing mechanical could ever retire the
    # entry, and `cidx security vuln check` went on reporting it as expired.

    Scenario: A repin removed the CVE from the repository the catalogue still runs
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2026-0002" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be obsolete
      And the exception verdict should mention "no image of it carries"

    # Fail-closed applies to a running repository too: the catalogue running it
    # is not evidence about the CVE, and conflating the two is what #311 was.

    Scenario: The running repository produced no scan result
      Given the catalogue runs "dhi.io/trivy"
      And no catalogue repository has been scanned
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved
      And the exception verdict should mention "cannot be shown to have gone"

    # And to the evidence itself. Trivy keeps the record of what its ignore file
    # removed only under `--show-suppressed`, and its report says nothing about
    # whether the flag was passed — a scan that hid four findings without
    # recording them looks exactly like one that hid none. Measured on the
    # audit's artifacts from the day before the flag landed: four ansible
    # entries, every one still carried, all four would have been deleted.

    Scenario: The results keep no record of what they suppressed
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scan results record nothing they suppressed
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved
      And the exception verdict should mention "cannot be told apart"

    # And the state that guard then became blind to. Since #303 stopped an
    # expired acceptance from filtering anything, every entry on file is past
    # its date, every ignore file the audit writes is empty and nothing is ever
    # recorded as suppressed — so the guard above held every absence as
    # unreadable, on the one ground that makes an absence readable. An empty
    # ignore file hid nothing. No scan result can state that, because what it
    # leaves behind is exactly the silence a dropped flag leaves, so the step
    # that builds the file states it instead (#327).

    Scenario: The audit states it filtered nothing, so an absence is an absence
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scan results record nothing they suppressed
      And the audit declared an empty ignore file for "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be obsolete
      And the exception verdict should mention "no image of it carries"

    # The declaration settles an absence only when it says nothing was filtered.
    # An ignore file with entries in it did remove something, and what that was
    # still has to be on the record before a missing CVE means anything.

    Scenario: The audit states it did filter, and nothing recorded what it removed
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scan results record nothing they suppressed
      And the audit declared an ignore file of 3 entries for "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "dhi.io/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved
      And the exception verdict should mention "cannot be told apart"

  Rule: An acceptance past its date waives nothing

    # The other half of "how long does an exception live", and for a long time
    # it was not enforced at all: `cidx security vuln ignore` emitted every
    # entry's CVE with no date check, so the eighteen acceptances that lapsed on
    # 2026-03-02 went on removing their findings from the audit's own scan
    # results, five months past their date, exactly as a live one does. The JSON
    # is written after the filter, so those findings could not appear anywhere
    # downstream — and `expires` was the whole of the mechanism meant to force
    # the acceptance to be argued again rather than inherited (#303).

    Scenario: An acceptance within its date still filters its finding out
      Given the exception "CVE-2026-0001" on "rust" expires on "2026-12-01"
      When the scanners' ignore file is built on "2026-08-03"
      Then the ignore file should carry "CVE-2026-0001"

    Scenario: An acceptance past its date filters nothing
      Given the exception "CVE-2026-0002" on "rust" expired on "2026-03-02"
      When the scanners' ignore file is built on "2026-08-03"
      Then the ignore file should not carry "CVE-2026-0002"

    # "Expires on 2026-03-02" reads as a deadline, not as a cut-off, so the
    # named day is included. It is also the boundary the Security tab already
    # applied to call an entry expired: the day an acceptance stops filtering is
    # the day the tab says it lapsed, rather than the day before it.

    Scenario: The day named is the last one it covers
      Given the exception "CVE-2026-0003" on "rust" expires on "2026-03-02"
      When the scanners' ignore file is built on "2026-03-02"
      Then the ignore file should carry "CVE-2026-0003"

    Scenario: The morning after, it covers nothing
      Given the exception "CVE-2026-0003" on "rust" expires on "2026-03-02"
      When the scanners' ignore file is built on "2026-03-03"
      Then the ignore file should not carry "CVE-2026-0003"

    # Fail-closed, the posture an unresolvable digest, an undatable candidate
    # and an unreadable scan all take. An acceptance is a *dated* judgement, and
    # with no readable date nothing says it is still one somebody has taken. It
    # is not reported expired — an entry with no date never began rather than
    # having lapsed — so `cidx security vuln check` names the malformed date,
    # and the finding it stops hiding reaches the audit like any other.

    Scenario: An acceptance carrying no expiry date at all covers nothing
      Given the exception "CVE-2026-0004" on "rust" carries no expiry date
      When the scanners' ignore file is built on "2026-08-03"
      Then the ignore file should not carry "CVE-2026-0004"

    Scenario: An acceptance whose date cannot be read covers nothing
      Given the exception "CVE-2026-0005" on "rust" expires on "next quarter"
      When the scanners' ignore file is built on "2026-08-03"
      Then the ignore file should not carry "CVE-2026-0005"

  Rule: An exception whose CVE went with its image is obsolete

    Scenario: The repository was replaced and the finding did not follow
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be obsolete
      And the exception verdict should mention "no catalogue image carries"

  Rule: An exception whose CVE survived the promotion is re-filed, not purged

    # Deleting this one loses the justification and leaves the next audit red on
    # a finding somebody reviewed months ago. It is re-filed onto the repository
    # that carries it instead.

    Scenario: The finding followed the promotion into the new repository
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be carried over
      And the exception verdict should name the repository "dhi.io/trivy"

    Scenario: An entry still keyed the old way names no repository at all
      Given the catalogue runs "golangci/golangci-lint"
      And the catalogue runs "rust"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2013-7445" on "rust"
      And the exception "CVE-2013-7445" was recorded against "golangci/golangci-lint:v2.6.2"
      When the exception lifecycle is applied
      Then the exception should be carried over
      And the exception verdict should name the repository "rust"

    Scenario: The scanners spell the identifier differently
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the scanners report "ghsa-cgrx-mc8f-2prm" on "dhi.io/trivy"
      And the exception "GHSA-cgrx-mc8f-2prm" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be carried over

    # The repository that carries it has an accepted entry of its own, so the
    # CVE is filtered out of its results too. Judging on the visible findings
    # alone reads it as gone and deletes a justification the audit still needs.

    Scenario: The repository carrying it suppressed it as well
      Given the catalogue runs "dhi.io/trivy"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2026-0001" on "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be carried over
      And the exception verdict should name the repository "dhi.io/trivy"

  Rule: An exception is never the answer to a CVE that is fixed upstream

    # A fix upstream means the finding disappears when the publisher republishes,
    # so the question is the image's age, not the vulnerability. The policy says
    # never to write an exception for one; the tooling has to name them rather
    # than re-file them in silence.

    Scenario: The carried-over CVE turns out to have a fix
      Given the catalogue runs "ghcr.io/ansible/community-ansible-dev-tools"
      And every catalogue repository has been scanned
      And the scanners report "CVE-2025-52881" on "ghcr.io/ansible/community-ansible-dev-tools", fixed in "1.2.8"
      And the exception "CVE-2025-52881" was recorded against "docker"
      When the exception lifecycle is applied
      Then the exception should be carried over
      And the exception verdict should name the fix "1.2.8"

    # A live entry is the case where this matters most and the one that said
    # nothing: an entry accepted on a repository the catalogue still runs, whose
    # CVE the publisher has since fixed, is an entry that should never have been
    # written — a repin candidate, not a renewal (#312). The evidence cannot come
    # from the scan results, because the audit builds its ignore file from these
    # very entries and the finding is filtered out of its own repository's scan.
    # Grype records what it suppressed; that is where the fix is read.

    Scenario: The live entry's CVE turns out to have been fixed upstream
      Given the catalogue runs "ghcr.io/ansible/community-ansible-dev-tools"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2025-52881" on "ghcr.io/ansible/community-ansible-dev-tools", fixed in "1.2.8"
      And the exception "CVE-2025-52881" was recorded against "ghcr.io/ansible/community-ansible-dev-tools"
      When the exception lifecycle is applied
      Then the exception should be live
      And the exception verdict should name the fix "1.2.8"

    # A fix nobody reported is silence, never a claim that no fix exists. The
    # finding is on the record as suppressed, so the entry is plainly still
    # doing its job — neither scanner named a version to repin to.

    Scenario: A live entry no scanner reported a fix for names none
      Given the catalogue runs "ghcr.io/ansible/community-ansible-dev-tools"
      And every catalogue repository has been scanned
      And the audit's ignore file suppressed "CVE-2025-52881" on "ghcr.io/ansible/community-ansible-dev-tools"
      And the exception "CVE-2025-52881" was recorded against "ghcr.io/ansible/community-ansible-dev-tools"
      When the exception lifecycle is applied
      Then the exception should be live
      And the exception verdict should name no fix

  Rule: Without scan evidence nothing is concluded

    # Fail-closed, like the cooldown on an undatable candidate and the scan gate
    # on an unreadable result: a purge decided on no evidence is a guess.

    Scenario: No scanner result is available at all
      Given the catalogue runs "dhi.io/trivy"
      And no catalogue repository has been scanned
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved
      And the exception verdict should mention "cannot be shown to have gone"

    Scenario: One catalogue repository is missing from the results
      Given the catalogue runs "dhi.io/trivy"
      And the catalogue runs "golangci/golangci-lint"
      And the scanners produced no result for "golangci/golangci-lint"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved

    # "No catalogue image carries it any more" is a claim about all of them, so
    # every repository has to be able to say what it filtered. One that cannot
    # holds the verdict, exactly as one that was never scanned does.

    Scenario: One catalogue repository cannot say what it filtered
      Given the catalogue runs "dhi.io/trivy"
      And the catalogue runs "golangci/golangci-lint"
      And every catalogue repository has been scanned
      And the scan results record nothing they suppressed
      And the audit declared an empty ignore file for "dhi.io/trivy"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be unresolved

    Scenario: Every catalogue repository states it filtered nothing
      Given the catalogue runs "dhi.io/trivy"
      And the catalogue runs "golangci/golangci-lint"
      And every catalogue repository has been scanned
      And the scan results record nothing they suppressed
      And the audit declared an empty ignore file for "dhi.io/trivy"
      And the audit declared an empty ignore file for "golangci/golangci-lint"
      And the exception "CVE-2026-0001" was recorded against "aquasec/trivy"
      When the exception lifecycle is applied
      Then the exception should be obsolete
      And the exception verdict should mention "no catalogue image carries"

  Rule: Two classes of finding never enter the triage queue

    # A CIDX container lives for seconds, listens on nothing and persists
    # nothing. Two classes are unreachable by construction and are exempt as
    # classes rather than case by case: the Go standard library compiled into a
    # CLI binary, and the kernel headers package. Together they were ~20% of
    # findings and ~0% of risk (#238).

    Scenario: The Go standard library in a CLI binary is exempt
      Given the catalogue runs "dhi.io/trivy"
      And the scanners report "CVE-2026-0100" on "dhi.io/trivy" in package "stdlib" of type "gobinary"
      When the findings are triaged
      Then 1 finding(s) should be exempt as Go stdlib
      And 0 finding(s) should need triage

    Scenario: The kernel headers package is exempt
      Given the catalogue runs "rust"
      And the scanners report "CVE-2013-7445" on "rust" in package "linux-libc-dev" of type "debian"
      When the findings are triaged
      Then 1 finding(s) should be exempt as kernel headers
      And 0 finding(s) should need triage

    Scenario: Another Go module in the same binary is not exempt
      Given the catalogue runs "ghcr.io/ansible/community-ansible-dev-tools"
      And the scanners report "CVE-2025-31133" on "ghcr.io/ansible/community-ansible-dev-tools" in package "github.com/opencontainers/runc" of type "gobinary"
      When the findings are triaged
      Then 1 finding(s) should need triage
      And 0 finding(s) should be exempt as Go stdlib

    Scenario: A package whose name merely starts with linux is not exempt
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0200" on "rust" in package "util-linux" of type "debian"
      When the findings are triaged
      Then 0 finding(s) should be exempt as kernel headers
      And 1 finding(s) should need triage

  Rule: A fixable finding is a question of the image's age, not a decision

    Scenario: A finding with a fix upstream leaves the triage queue
      Given the catalogue runs "ghcr.io/ansible/community-ansible-dev-tools"
      And the scanners report "CVE-2025-52881" on "ghcr.io/ansible/community-ansible-dev-tools", fixed in "1.2.8"
      When the findings are triaged
      Then 1 finding(s) should be fixed upstream
      And 0 finding(s) should need triage

    Scenario: A finding with no fix at any version is the only exception territory
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0300" on "rust" in package "libxml2" of type "debian"
      When the findings are triaged
      Then 0 finding(s) should be fixed upstream
      And 1 finding(s) should need triage
