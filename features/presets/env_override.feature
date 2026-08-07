Feature: A Preset Env Value Is A Default
  As someone parameterising a preset from a CI workflow
  I want an environment variable I export to reach the container
  So that a release publishes the tag it was cut for rather than the preset's default

  # Issue #384. `release.yml` passed the release tag the idiomatic way —
  # `env: IMAGE_TAG: ${{ github.ref_name }}` in front of `cidx run docker` —
  # and it did nothing: the kaniko preset declares `IMAGE_TAG = "latest"`, and
  # the declared value won. Every release ever published `:latest` and no
  # version tag at all, and nightly.yml published `:latest` where it asked for
  # `:nightly`. The verification step of #281 is what finally said so, by
  # running `docker run ghcr.io/cidx-org/cidx:v3.0.0` and getting
  # `manifest unknown`.
  #
  # The asymmetry that hid it: a reference *inside* a value has always been
  # resolved from the environment — `IMAGE_NAME = "ghcr.io/${GITHUB_REPOSITORY}"`
  # works — so the mechanism looked present while the key itself was never
  # looked up.

  Rule: What the environment defines wins over what the preset declares

    Scenario: An exported value reaches the command
      Given the environment sets "GITHUB_REPOSITORY" to "cidx-org/cidx"
      And the environment sets "IMAGE_TAG" to "v3.0.0"
      When I resolve the preset "kaniko" without overrides
      Then the resolved command should contain "--destination=ghcr.io/cidx-org/cidx:v3.0.0"

    Scenario: The declared value stands when the environment says nothing
      Given the environment sets "GITHUB_REPOSITORY" to "cidx-org/cidx"
      And the environment does not set "IMAGE_TAG"
      When I resolve the preset "kaniko" without overrides
      Then the resolved command should contain "--destination=ghcr.io/cidx-org/cidx:latest"

  Rule: The invariants a container needs are not parameters

    # HOME, GOPATH, GOCACHE and the cache directories exist so a container works
    # as the invoking uid (#188). They are declared by presets and referenced by
    # no command, so widening the parameterisation does not reach them: what the
    # override applies to is the placeholder a command spells out.

    Scenario: A developer's own HOME does not follow them into the container
      Given the environment sets "HOME" to "/home/dev"
      When I resolve the preset "bandit" without overrides
      Then the resolved environment should set "HOME" to "/tmp"
