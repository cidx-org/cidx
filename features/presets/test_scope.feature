Feature: Test Preset Scope
  As someone whose tests live where the language puts them
  I want a test preset to run the whole project without an override
  So that a green test phase means the suite ran, not that it was never compiled

  # A test phase that runs a subset is indistinguishable, from the outside, from
  # one that runs everything: both print that the container completed. This
  # repository paid for that twice — #271 found the godog suite had never run in
  # CI, #344 found internal/commands in the same blind spot — and both times the
  # repair was a local override, which only fixes the project that thought to
  # write it. #357 moved the fix into the catalogue.

  Rule: A test preset runs the project, not a subtree of it

    Scenario: The Go test preset reaches the root package and internal/
      When I resolve the preset "go-test" without overrides
      Then the resolved command should be "go test -v ./..."

    # The BDD suite is selected by test name and not by package, so the
    # whole-tree form stays: narrowing it here would hide the same way.
    Scenario: The godog preset filters by test name, not by package
      When I resolve the preset "godog" without overrides
      Then the resolved command should contain "./..."
      And the resolved command should contain "-run TestFeatures"
