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
      Then the resolved command should be "sh -c 'pip install --quiet mypy && PATH=$PATH:/tmp/.local/bin mypy . --strict'"

    Scenario: A shell-wrapped command is untouched without overrides
      When I resolve the preset "mypy" without overrides
      Then the resolved command should be "sh -c 'pip install --quiet mypy && PATH=$PATH:/tmp/.local/bin mypy .'"

    Scenario: The reported cargo-audit override reaches cargo-audit
      When I resolve the preset "cargo-audit" with option "deny" set to "true"
      Then the resolved command should contain "/tmp/cargo-audit audit --deny warnings'"

  Rule: A boolean option is a switch, not a key/value pair

    Scenario: An enabled boolean emits the flag alone
      When I resolve the preset "golangci-lint" with option "fix" set to "true"
      Then the resolved command should be "golangci-lint run --timeout 5m --fix"

    Scenario: A disabled boolean emits nothing
      When I resolve the preset "golangci-lint" with option "fix" set to "false"
      Then the resolved command should be "golangci-lint run --timeout 5m"

    Scenario: A value that is not a boolean is refused and the default stands
      When I resolve the preset "golangci-lint" with option "fix" set to "yes"
      Then the resolved command should be "golangci-lint run --timeout 5m"
