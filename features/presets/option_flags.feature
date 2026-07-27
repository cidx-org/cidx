Feature: Preset Option Flags
  As a developer overriding a built-in preset option
  I want the generated flag to reach the tool it configures
  So that my override changes what actually runs

  Rule: A command_flag lands where the configured tool can read it

    Scenario: A plain command receives the flag at the end
      When I resolve the preset "trivy" with option "severity" set to "HIGH,CRITICAL"
      Then the resolved command should be "fs /work --severity HIGH,CRITICAL"

    Scenario: A shell-wrapped command receives the flag inside the quoting
      When I resolve the preset "mypy" with option "strict" set to "true"
      Then the resolved command should be "sh -c 'pip install --quiet mypy && mypy . --strict true'"

    Scenario: A shell-wrapped command is untouched without overrides
      When I resolve the preset "mypy" without overrides
      Then the resolved command should be "sh -c 'pip install --quiet mypy && mypy .'"

    Scenario: The reported cargo-audit override reaches cargo-audit
      When I resolve the preset "cargo-audit" with option "deny" set to "warnings"
      Then the resolved command should contain "/tmp/cargo-audit audit --deny warnings warnings'"
