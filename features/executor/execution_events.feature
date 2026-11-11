Feature: A run publishes its progress as events on NATS
  As an orchestrator that launches "cidx run"
  I want to follow the run through structured events
  So that I know which phase and container is running, and what failed, without parsing text

  # Until now a caller saw an exit code and a text log. With a NATS address, the
  # run also publishes versioned JSON events: start and end of the run, of each
  # phase and of each container. Without an address nothing changes and nothing
  # is dialled. The broker is never allowed to change the outcome of a run.

  Rule: With an address, the run is published

    Scenario: A passing run publishes its structure under a common run id
      Given a NATS server
      And a pipeline "ci" with the phases "code, test"
      When the pipeline "ci" is run publishing to the server as run "run-42"
      Then the run should succeed
      And the server should have received the events "run.started, phase.started, container.started, container.finished, phase.finished, phase.started, container.started, container.finished, phase.finished, run.finished"
      And every event should be format "cidx.event/v1" and carry the run id "run-42"
      And the event "container.finished" for "scenario-test" should name an image, the exit code 0 and a duration

    Scenario: The subject prefix is configurable
      Given a NATS server
      And a pipeline "ci" with the phases "code"
      When the pipeline "ci" is run publishing under the subject prefix "team.builds"
      Then the server should have received a message on "team.builds.run.started"

    Scenario: A run id is generated when the caller gives none
      Given a NATS server
      And a pipeline "ci" with the phases "code"
      When the pipeline "ci" is run publishing to the server without a run id
      Then every event should carry the same non-empty run id

    Scenario: A failing container is reported with its exit code
      Given a NATS server
      And a pipeline "ci" with the phases "code, test"
      And the container of the phase "test" exits with code 7
      When the pipeline "ci" is run publishing to the server as run "run-7"
      Then the run should fail
      And the event "container.finished" for "scenario-test" should have the status "failed" and the exit code 7
      And the event "run.finished" should have the status "failed" and the exit code 7

  Rule: Without an address, nothing changes

    Scenario: No address, no connection and no events
      Given a NATS server
      And a pipeline "ci" with the phases "code, test"
      When the pipeline "ci" is run without a NATS address
      Then the run should succeed
      And the server should have been neither contacted nor sent anything
      And the output should not mention NATS

  Rule: The broker never changes the outcome of a run

    Scenario: An unreachable broker warns once and the run goes on
      Given a NATS address where nothing listens
      And a pipeline "ci" with the phases "code, test"
      And the container of the phase "test" exits with code 3
      When the pipeline "ci" is run publishing to that address
      Then the run should fail with the exit code of the container, 3
      And the output should warn once that NATS events are disabled

    Scenario: Connection credentials are never written
      Given a NATS server
      And the connection credentials "alice" and "hunter2pw"
      And a pipeline "ci" with the phases "code"
      When the pipeline "ci" is run publishing to the server with those credentials
      Then the run should succeed
      And neither the output nor any event should contain "hunter2pw"

  Rule: Events carry no container output and no secret

    Scenario: A container that prints and holds a secret leaks neither
      Given a NATS server
      And a pipeline "ci" with the phases "code"
      And the container of the phase "code" prints "BUILD-LOG-LINE" and holds the secret "TOP-SECRET-VALUE"
      When the pipeline "ci" is run publishing to the server as run "run-9"
      Then the run should succeed
      And no event should contain "BUILD-LOG-LINE"
      And no event should contain "TOP-SECRET-VALUE"

  Rule: The options exist on the command

    Scenario: cidx run accepts the address, the subject and the run id
      When I inspect the flags of "cidx run"
      Then "cidx run" should accept "--nats-url" from "CIDX_NATS_URL"
      And "cidx run" should accept "--nats-subject" from "CIDX_NATS_SUBJECT"
      And "cidx run" should accept "--run-id" from "CIDX_RUN_ID"
