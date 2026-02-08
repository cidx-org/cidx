Feature: Custom Preset Loading
  As a developer or platform engineer
  I want to define custom tool presets at the user or project level
  So that I can use tools not built into CIDX or override default behaviors

  Rule: Presets are loaded in a specific hierarchy (Built-in < User < Project)

    Scenario: Built-in presets are available by default
      Given I have no custom configuration files
      When I run "cidx run trivy"
      Then it should use the built-in "trivy" preset

    Scenario: User-level presets override built-in presets
      Given I have a user config file at "~/.config/cidx/presets.toml"
      And the user config defines a custom image for "trivy"
      When I run "cidx run trivy"
      Then it should use the custom image from the user config

    Scenario: Project-level presets override user and built-in presets
      Given I have a project config file at ".cidx/presets.toml"
      And the project config defines a custom command for "trivy"
      When I run "cidx run trivy"
      Then it should use the custom command from the project config

    Scenario: New custom tools can be defined in project presets
      Given I have a project config file at ".cidx/presets.toml"
      And the project config defines a new tool "my-custom-linter"
      When I run "cidx run my-custom-linter"
      Then it should execute the "my-custom-linter" container
      And the container should use the configuration from ".cidx/presets.toml"

  Rule: Custom presets follow the standard TOML structure

    Scenario: Valid custom preset file
      Given a file ".cidx/presets.toml" with content:
        """
        [presets.custom-tool]
        name = "custom-tool"
        image = "alpine:latest"
        command = "echo hello"
        phase = "test"
        """
      When I validate the configuration
      Then it should be valid
      And the tool "custom-tool" should be available
