Feature: Run events on NATS
  As a system that launches `cidx run`
  I want structured events on NATS
  So that I know which phase and container is running, and what failed,
  without parsing the text output

  Scenario: A run announces itself, its phases and its containers
    Given a NATS server is listening
    And a run-events pipeline with the phases "security,test"
    When I run the pipeline with run events published
    Then the run events should be, in order:
      | type               | phase    | container     |
      | run.started        |          |               |
      | phase.started      | security |               |
      | container.started  | security | scenario-security |
      | container.finished | security | scenario-security |
      | phase.finished     | security |               |
      | phase.started      | test     |               |
      | container.started  | test     | scenario-test |
      | container.finished | test     | scenario-test |
      | phase.finished     | test     |               |
      | run.finished       |          |               |
    And every run event should carry the format "cidx.run-event/v1"
    And every run event should share one run id
    And the finished container events should carry the image, the exit code and the duration

  Scenario: The caller chooses the run id and the subject
    Given a NATS server is listening
    And a run-events pipeline with the phases "test"
    When I run the pipeline with run events published as run "build-1842" on subject "acme.ci.events"
    Then every run event should be on the subject "acme.ci.events"
    And every run event should carry the run id "build-1842"

  Scenario: A failing container is reported with its exit code
    Given a NATS server is listening
    And a run-events pipeline with the phases "security,test"
    And the run-events container "scenario-security" exits with code 7
    When I run the pipeline with run events published
    Then the run should fail
    And the run.finished event should be "failed" with exit code 7
    And no event should mention the phase "test"

  Scenario: Without a NATS address nothing changes
    Given a NATS server is listening
    And a run-events pipeline with the phases "security,test"
    When I run the pipeline without a NATS address
    Then the run should pass
    And the NATS server should have received no connection
    And no run-events warning should be printed

  Scenario: NATS unavailable does not change the run
    Given a NATS address with credentials where nothing is listening
    And a run-events pipeline with the phases "security,test"
    And the run-events container "scenario-security" exits with code 7
    When I run the pipeline with run events published
    Then the run should fail
    And the run exit code should be 7
    And exactly one run-events warning should be printed
    And the run-events warning should not contain the address or its credentials

  Scenario: A healthy run survives an unavailable NATS
    Given a NATS address with credentials where nothing is listening
    And a run-events pipeline with the phases "security,test"
    When I run the pipeline with run events published
    Then the run should pass
    And exactly one run-events warning should be printed

  Scenario: Events carry no secret and no container output
    Given a NATS server is listening
    And a run-events pipeline with the phases "test"
    And the run-events container "scenario-test" holds the secret "tok-9f8e7d6c"
    When I run the pipeline with run events published
    Then no run event should contain "tok-9f8e7d6c"
    And no run event should have a field other than the documented ones
