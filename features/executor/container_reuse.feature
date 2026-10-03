Feature: A reused container is the container the configuration describes
  As a developer running the same tool again and again
  I want cidx to reuse a container only while it would be created the same way
  So that keeping a warm cache never means running a stale container

  # A container is reused to keep its caches (#144). It is reused only while the
  # hash of what shaped it still matches, so whatever shapes it has to be in the
  # hash. `privileged` was left out on the reasoning that it "affects execution
  # behaviour, not container state" -- but cidx never passes --privileged to
  # Docker: the field decides the user the container is created with, and a
  # container created as the host user is not one a root-only tool can use.
  # Switching a tool to `privileged = true` kept running the old container, and the
  # run failed as if the flag were ignored: "List directory /var/lib/apt/lists/partial
  # is missing. Acquire (13: Permission denied)" (#531).
  #
  # What is hashed is therefore the identity the container runs as, not the flag:
  # the user, and the user-namespace mode Podman rootless needs.

  Rule: Changing who a container runs as recreates it

    Scenario: Switching a tool to privileged recreates its container
      Given a tool "postgres-test" that runs as the host user
      When the tool is switched to privileged
      Then the container created before must not be reused

    Scenario: Switching a tool back to the host user recreates its container
      Given a privileged tool "postgres-test"
      When the tool is switched to run as the host user
      Then the container created before must not be reused

    Scenario: A container created for another host user is not reused
      Given a tool "trivy" that runs as the host user 1000:1000
      When the same tool is run by the host user 1001:1001
      Then the container created before must not be reused

    Scenario: Podman rootless needs a different user-namespace mode
      Given a tool "trivy" that runs as the host user
      When the same tool is run under rootless Podman
      Then the container created before must not be reused

  Rule: What does not shape the container does not recreate it

    # The reuse exists to keep caches: a recreate costs them. Only the fields that
    # change the created container are hashed, so these two stay out on purpose.

    Scenario: The same tool run again keeps its container
      Given a tool "trivy" that runs as the host user
      When the same tool is run again
      Then the container created before is reused

    Scenario: A pull policy or a timeout changes nothing about the container
      Given a tool "trivy" that runs as the host user
      When the tool's pull policy and timeout change
      Then the container created before is reused
