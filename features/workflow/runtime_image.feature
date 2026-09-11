Feature: Verify the published runtime image
  CI jobs using the CIDX image need curl as well as the CIDX executable.

  Scenario Outline: Publication requires both runtime commands
    Given the runtime image command "<command>" fails
    When the release workflow verifies the runtime image
    Then runtime image verification should <result>

    Examples:
      | command | result  |
      | none    | succeed |
      | cidx    | fail    |
      | curl    | fail    |
