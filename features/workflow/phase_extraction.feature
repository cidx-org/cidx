Feature: Phases a GitHub Actions workflow really runs
  As a developer whose CI runs cidx
  I want "check workflow" to read the phases my workflow actually executes
  So that it never reports a phase as missing from a workflow that runs it

  # The lesson of issue #233: the phases were extracted by looking for the
  # literal substring "cidx run " in a step. ci.yml runs
  # `./bin/cidx --verbose run test` since #271, so the phase was simply lost:
  # `check workflow` said "Missing in GitHub workflow: test" while
  # `check drift` said "test ✓ match", on the same file.

  Rule: The phase is read off the command line, whatever surrounds it

    Scenario Outline: Forms the workflows really use
      Given a workflow job "test" running "<script>"
      When I extract the phases of the workflow
      Then the extracted phases should be "test"

      Examples: A flag never hides the phase
        | script                              |
        | cidx run test                       |
        | bin/cidx run test                   |
        | ./bin/cidx run test                 |
        | ./bin/cidx --verbose run test       |
        | cidx --config ci/pipeline.toml run test |
        | cidx run --dry-run test             |
        | ./bin/cidx run --stream test        |
        | go run ./cmd/cidx run test          |

    Scenario: A job that runs two phases reports both
      Given a workflow job "checks" running:
        """
        chmod +x bin/cidx
        ./bin/cidx run security
        ./bin/cidx --verbose run code
        """
      When I extract the phases of the workflow
      Then the extracted phases should be "security, code"

  Rule: What cannot be read with certainty yields no phase

    Scenario Outline: Forms deliberately left unjudged
      Given a workflow job "test" running "<script>"
      When I extract the phases of the workflow
      Then no phase should be extracted

      Examples: Not a phase, or not readable as one
        | script                          |
        | go build -o bin/cidx ./cmd/cidx |
        | chmod +x bin/cidx               |
        | # cidx run test is the old way  |
        | $CIDX run test                  |
        | ./bin/cidx run $PHASE           |
        | ./bin/cidx check drift          |

    Scenario: A heredoc body is data, not commands
      Given a workflow job "docs" running:
        """
        cat <<'EOF' > docs/usage.md
        cidx run test
        EOF
        """
      When I extract the phases of the workflow
      Then no phase should be extracted

  Rule: A phase the workflow runs is never reported as missing

    Scenario: The workflow runs every declared phase, with a global flag
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And a workflow job "security" running "./bin/cidx run security"
      And a workflow job "test" running "./bin/cidx --verbose run test"
      When I compare the pipeline with the workflow
      Then the workflow should be in sync with the pipeline

    Scenario: A phase the workflow never runs is still reported
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And a workflow job "security" running "./bin/cidx run security"
      When I compare the pipeline with the workflow
      Then phase "test" should be reported as missing from the workflow
