Feature: A container can be an alias of a catalogue preset
  As a developer whose project runs the same tool twice, with different options
  I want to declare a second container that is a preset under another name
  So that the image stays declared once, in the catalogue, and cannot rot in my cidx.toml

  # Overrides are keyed by container name, not by phase, so running prettier to
  # check in one phase and to write in another meant a second container carrying
  # a copy of the catalogue's image digest -- a copy the image monitor never
  # looked at, which no catalogue bump could reach (#292, #290).
  #
  # `preset = "prettier"` in a [containers.NAME] section makes NAME that preset,
  # run under its own name, with the section's overrides on top. Nothing else is
  # copied: not the image, not the command, not the volumes.

  Rule: An alias is the preset it names, under a name of its own

    Scenario: An alias runs the image the catalogue pins
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier-write"]

        [containers.prettier-write]
        preset = "prettier"
        write = true
        """
      When the configuration is validated and "prettier-write" is resolved
      Then the configuration should be valid
      And "prettier-write" should run the image of the "prettier" preset

    Scenario: An alias applies its own overrides and options
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier-write"]

        [containers.prettier-write]
        preset = "prettier"
        write = true
        """
      When the configuration is validated and "prettier-write" is resolved
      Then "prettier-write" should have a command containing "--write"
      And the preset "prettier" should keep a command without "--write"

    Scenario: An alias gets a container of its own
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier-write"]

        [containers.prettier-write]
        preset = "prettier"
        write = true
        """
      When the configuration is validated and "prettier-write" is resolved
      Then "prettier-write" should be resolved under the name "prettier-write"

    Scenario: An alias can override the image like any preset
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier-old"]

        [containers.prettier-old]
        preset = "prettier"
        image = "jauderho/prettier:2.8.8"
        """
      When the configuration is validated and "prettier-old" is resolved
      Then the configuration should be valid
      And "prettier-old" should run the image "jauderho/prettier:2.8.8"

  Rule: An alias that cannot work is refused where it is declared

    Scenario: An alias of a preset that does not exist
      Given a cidx.toml containing:
        """
        [format]
        containers = ["fmt"]

        [containers.fmt]
        preset = "no-such-tool"
        """
      When the configuration is validated and "fmt" is resolved
      Then the configuration should be refused, saying "no-such-tool"

    Scenario: An alias of an alias
      Given a cidx.toml containing:
        """
        [format]
        containers = ["second"]

        [containers.first]
        preset = "prettier"

        [containers.second]
        preset = "first"
        """
      When the configuration is validated and "second" is resolved
      Then the configuration should be refused, saying "not another alias"

    Scenario: An alias that takes a catalogue preset's name
      Given a cidx.toml containing:
        """
        [format]
        containers = ["trivy"]

        [containers.trivy]
        preset = "gitleaks"
        """
      When the configuration is validated and "trivy" is resolved
      Then the configuration should be refused, saying "a name of its own"

    Scenario: An alias nobody runs is still checked
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier"]

        [containers.unused]
        preset = "no-such-tool"
        """
      When the configuration is validated and "prettier" is resolved
      Then the configuration should be refused, saying "no-such-tool"

    Scenario: A misspelt key on an alias is reported against the preset it names
      Given a cidx.toml containing:
        """
        [format]
        containers = ["prettier-write"]

        [containers.prettier-write]
        preset = "prettier"
        wirte = true
        """
      When the configuration is validated and "prettier-write" is resolved
      Then the configuration should be refused, saying "containers.prettier-write.wirte"
