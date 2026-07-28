Feature: Stale cidx invocations in CI workflows
  As a developer whose CI calls cidx
  I want to be told when a workflow calls a subcommand that no longer exists
  So that moving a command does not turn into days of silently red nightly runs

  # The lesson of issue #239: security-audit.yml kept calling `cidx vuln ...`
  # for 8 days after that subcommand moved under `cidx security vuln`, and both
  # `cidx validate` and `cidx check drift` stayed green the whole time.

  Rule: An invocation that no longer resolves is reported

    Scenario: A subcommand that moved under a namespace
      Given a workflow step running "./bin/cidx vuln list"
      When I validate the workflow invocations
      Then the invocation should be reported as stale
      And the report should mention "unknown command"
      And the report should mention "vuln"

    Scenario: The report says where the command went
      Given a workflow step running "./bin/cidx vuln list"
      When I validate the workflow invocations
      Then the report should mention "cidx security vuln"

    Scenario: A subcommand that never existed
      Given a workflow step running "cidx check drifts"
      When I validate the workflow invocations
      Then the invocation should be reported as stale

  Rule: Invocations that resolve are left alone

    Scenario Outline: Supported invocation forms
      Given a workflow step running "<script>"
      When I validate the workflow invocations
      Then no invocation should be reported

      Examples: The forms CI really uses
        | script                                    |
        | cidx run ci                               |
        | ./bin/cidx run security                   |
        | go run ./cmd/cidx security vuln list      |
        | cidx --config ci/pipeline.toml run ci     |
        | cidx --verbose check drift                |
        | cidx --version                            |
        | TARGETS=$(./bin/cidx preset scan-targets) |
        | if ./bin/cidx security vuln list; then    |

  Rule: The check stays silent on what it cannot read with certainty

    Scenario Outline: Forms deliberately left unjudged
      Given a workflow step running "<script>"
      When I validate the workflow invocations
      Then no invocation should be reported

      Examples: Not an invocation, or not readable as one
        | script                          |
        | go build -o bin/cidx ./cmd/cidx |
        | chmod +x bin/cidx               |
        | # cidx vuln list is the old way |
        | echo Old form: cidx vuln list   |
        | $CIDX vuln list                 |
        | cidx ${{ matrix.command }}      |

    Scenario: A heredoc body is data, not commands
      Given a workflow step running:
        """
        cat <<'EOF' > docs/usage.md
        cidx vuln list
        EOF
        """
      When I validate the workflow invocations
      Then no invocation should be reported
