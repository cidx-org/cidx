Feature: The audit reuses the catalogue summary
  As the maintainer of the preset catalogue
  I want the audit report to reuse the existing status page
  So that findings and exceptions have one maintained presentation

  Rule: The job summary uses the generated page and links to the evidence

    Scenario: A successful scan preserves the existing catalogue summary
      Given the catalogue runs "rust"
      And the scanners report "CVE-2026-0201" on "rust"
      And the exception "CVE-2026-0202" on "rust" expired on "2020-01-01"
      And the base of "rust" stopped being supported
      And the audit scanner jobs both finished with "success"
      When the audit workflow writes its job summary
      Then the audit report should contain the complete catalogue status page
      And the audit report should link to the raw scan artifacts
      And the audit report should show the scanner job results
      And the status issue should reuse the generated page

    Scenario Outline: An incomplete scan remains visible in the report
      Given the catalogue runs "rust"
      And the scanners produced no result for "rust"
      And the audit scanner jobs both finished with "<result>"
      When the audit workflow writes its job summary
      Then the audit report should contain the complete catalogue status page
      And the audit report should show the scanner job results
      And the summary should name "rust" as unscanned

      Examples:
        | result    |
        | failure   |
        | cancelled |
        | skipped   |

    Scenario: A rendering failure leaves diagnostics and cannot publish an empty issue
      Given the catalogue runs "rust"
      And the audit scanner jobs both finished with "failure"
      And the catalogue summary command will fail
      When the audit workflow writes its job summary
      Then the audit report step should fail
      And the audit report should show the scanner job results
      And the status issue should not be published without a generated page
