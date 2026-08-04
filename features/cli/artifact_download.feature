Feature: Fetching a run's artifacts from cidx
  As someone about to run cidx security vuln prune
  I want cidx to put the scan results on disk
  So that the command that reads artifacts is not the one command that needs another tool

  # Issue #285. `cidx repo artifact` lists, counts and cleans up, but could not
  # fetch — while `cidx security vuln prune --results DIR` and `cidx security
  # baseline --results DIR` read exactly the files a workflow run uploads. The
  # only way to put them there was `gh run download`, which cost two bugs of its
  # own: another repository's artifacts when run outside a checkout (#327), and
  # a subdirectory per artifact where the readers expect one flat directory
  # (#333).

  # The command-line scenarios type against the tree cidx ships (issue #317), so
  # a flag that is renamed or a default that moves fails here.

  Rule: The download and the commands that read what it downloads agree on the directory

    Scenario: The line that pairs the two halves of the flow
      When I type the command line cidx repo artifact download --run 18234567890
      Then cidx should accept the line

    Scenario: Its default is the default the readers already have
      Then the default of "cidx repo artifact download --output" is the default of "cidx security vuln prune --results"
      And the default of "cidx repo artifact download --output" is the default of "cidx security baseline --results"

  Rule: What lands on disk is one flat directory

    Scenario: Two artifacts, one directory, no subdirectory in sight
      Given run "724" produced artifacts:
        | artifact | file                      | content |
        | trivy-0  | trivy-alpine_3.20.json    | {}      |
        | trivy-1  | results/trivy-golang.json | {}      |
      When I download the artifacts of run "724"
      Then the results directory holds exactly:
        | trivy-alpine_3.20.json |
        | trivy-golang.json      |

    Scenario: An archive entry cannot name a path outside the destination
      Given run "724" produced artifacts:
        | artifact | file               | content |
        | hostile  | ../../escaped.json | {}      |
      When I download the artifacts of run "724"
      Then the results directory holds exactly:
        | escaped.json |

  Rule: A name two artifacts share is a fact about the run, not a reason to stop

    Scenario: The same file in two artifacts, with the same content
      Given run "724" produced artifacts:
        | artifact | file               | content            |
        | trivy-0  | ignore-report.json | nothing suppressed |
        | trivy-1  | ignore-report.json | nothing suppressed |
        | grype-0  | grype-alpine.json  | no matches         |
      When I download the artifacts of run "724"
      Then the results directory holds exactly:
        | grype-alpine.json  |
        | ignore-report.json |

    Scenario: The same file with different content keeps the first copy
      Given run "724" produced artifacts:
        | artifact | file               | content            |
        | trivy-0  | ignore-report.json | written by trivy-0 |
        | grype-0  | ignore-report.json | written by grype-0 |
      When I download the artifacts of run "724"
      Then "ignore-report.json" holds "written by trivy-0"

  Rule: An empty results directory is never the answer to a mistyped request

    Scenario: A name pattern that matches nothing is refused, and says what the run has
      Given run "724" produced artifacts:
        | artifact | file             | content |
        | trivy-0  | trivy-alpine.json | {}     |
        | grype-0  | grype-alpine.json | {}     |
      When I download the artifacts of run "724" matching "trivvy-*"
      Then the download is refused
      And the refusal names "trivy-0"
      And the refusal names "grype-0"

    Scenario: Only the artifacts the pattern selects are fetched
      Given run "724" produced artifacts:
        | artifact | file              | content |
        | trivy-0  | trivy-alpine.json | {}      |
        | grype-0  | grype-alpine.json | {}      |
      When I download the artifacts of run "724" matching "trivy-*"
      Then the results directory holds exactly:
        | trivy-alpine.json |

    Scenario: A run that uploaded nothing is reported as such
      Given run "999" produced artifacts:
        | artifact | file | content |
      When I download the artifacts of run "999"
      Then the download is refused
      And the refusal names "999"
