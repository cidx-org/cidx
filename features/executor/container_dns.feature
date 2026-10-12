Feature: A container can be given DNS servers
  As a developer on a host whose Docker containers cannot resolve names
  I want to declare the DNS servers a container uses in cidx.toml
  So that the run works without changing the host daemon or mounting a resolver file

  # A bridge container inherits the host's resolver. On some self-hosted runners
  # that resolver cannot be reached from the bridge network, and `rustup` dies on
  # "Temporary failure in name resolution" while the same image resolves with
  # `docker run --dns 1.1.1.1` (#543). The setting is per container, an IP
  # address list, and absent by default: nothing changes for a project that does
  # not use it.

  Rule: dns reaches the container

    Scenario: A container declares its DNS servers
      Given a cidx.toml containing:
        """
        [build]
        containers = ["cargo-build"]

        [containers.cargo-build]
        dns = ["1.1.1.1", "9.9.9.9"]
        """
      When the configuration is validated and "cargo-build" is resolved
      Then the configuration should be valid
      And "cargo-build" should resolve the DNS servers "1.1.1.1, 9.9.9.9"

    Scenario: A container that declares none keeps the runtime default
      Given a cidx.toml containing:
        """
        [build]
        containers = ["cargo-build"]
        """
      When the configuration is validated and "cargo-build" is resolved
      Then "cargo-build" should resolve no DNS servers

  Rule: A value Docker would refuse is refused when the file is read

    Scenario: A hostname is not a DNS server
      Given a cidx.toml containing:
        """
        [build]
        containers = ["cargo-build"]

        [containers.cargo-build]
        dns = ["one.one.one.one"]
        """
      When the configuration is validated and "cargo-build" is resolved
      Then the configuration should be refused, saying "is not an IP address"

    Scenario: A single string is not a list
      Given a cidx.toml containing:
        """
        [build]
        containers = ["cargo-build"]

        [containers.cargo-build]
        dns = "1.1.1.1"
        """
      When the configuration is validated and "cargo-build" is resolved
      Then the configuration should be refused, saying "must be a list of IP addresses"

  Rule: A container is reused only while it resolves names the same way

    Scenario: Giving a container a DNS server recreates it
      Given a tool "cargo-build" that runs as the host user
      When the tool is given the DNS servers "1.1.1.1"
      Then the container created before must not be reused

    Scenario: Changing a container's DNS servers recreates it
      Given a tool "cargo-build" with the DNS servers "1.1.1.1"
      When the tool is given the DNS servers "9.9.9.9"
      Then the container created before must not be reused

    Scenario: Reordering the DNS servers recreates the container
      Given a tool "cargo-build" with the DNS servers "1.1.1.1,9.9.9.9"
      When the tool is given the DNS servers "9.9.9.9,1.1.1.1"
      Then the container created before must not be reused

    Scenario: Unchanged DNS servers keep the container
      Given a tool "cargo-build" with the DNS servers "1.1.1.1"
      When the same tool is run again
      Then the container created before is reused
