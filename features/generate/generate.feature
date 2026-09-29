Feature: CI Workflow Generation
  As a developer
  I want to generate CI platform files from cidx.toml
  So that I don't have to manually write platform-specific YAML

  Background:
    Given a valid "cidx.toml" exists

  Rule: Generate produces correct GitHub Actions structure

    Scenario: Generate GitHub Actions workflow
      Given cidx.toml defines pipeline "ci" with phases "security, code, test, build"
      When I run "cidx generate github"
      Then the output should be valid YAML
      And the output should contain a "bootstrap" job
      And each phase should have its own job

    Scenario: Phases run in parallel after bootstrap
      Given cidx.toml defines pipeline "ci" with phases "security, code, test"
      When I run "cidx generate github"
      Then jobs "security", "code", "test" should depend on "bootstrap"
      And jobs "security", "code", "test" should NOT depend on each other

    Scenario: Pipeline triggers map to GitHub events
      Given cidx.toml defines pipeline "pr"
      And cidx.toml defines pipeline "main"
      When I run "cidx generate github"
      Then "pr" pipeline should trigger on "pull_request"
      And "main" pipeline should trigger on "push" to "main" branch

  Rule: A superseded pull request run is cancelled, and nothing else (#504)

    # Pushing twice to a PR branch -- `pr create`'s initialization commit then
    # `cpw` -- ran two complete pipelines side by side, the older one useless.
    # Only runs of the same pull request share a group: every other run gets a
    # group of its own, so a push to main or a tag is never cancelled. A shared
    # group would not do even with cancel-in-progress off: GitHub keeps one
    # running and one pending run per group and cancels the older pending ones,
    # so three quick merges to main would lose the middle commit's run.

    Scenario: Runs of the same pull request cancel each other
      Given cidx.toml defines pipeline "pr" with phases "security, code"
      When I run "cidx generate github"
      Then the workflow should group runs by pull request number
      And the workflow should cancel a run its group supersedes

    Scenario: A run that is not a pull request never shares a group
      Given cidx.toml defines pipeline "main" with phases "security, code"
      When I run "cidx generate github"
      Then the workflow should give a run that is not a pull request a group of its own

  Rule: A phase can cache what its containers rebuild (#503)

    # A Rust build compiled tokio, hyper and rustls from scratch on every job
    # and every push -- 21 minutes cold against 6 warm. `target/` lands in the
    # workspace on the runner and was thrown away after each job. The cache is
    # opt-in and per phase: debug artefacts (test, clippy) and release ones
    # (build) differ, so sharing one cache would keep evicting the other. The
    # key names the files whose content decides when the cache is stale; cidx
    # does not guess a language's lockfile, and a cache keyed on nothing would
    # never be invalidated.

    Scenario: A phase that declares a cache restores and saves it
      Given cidx.toml defines pipeline "ci" with phases "build"
      And the "build" phase caches "target" keyed on "**/Cargo.lock"
      When I run "cidx generate github"
      Then the "build" job should cache "target"
      And the cache key of the "build" job should hash "**/Cargo.lock"
      And the cache of the "build" job should fall back to an older cache of that phase

    Scenario: Each phase has a cache of its own
      Given cidx.toml defines pipeline "ci" with phases "test, build"
      And the "test" phase caches "target" keyed on "**/Cargo.lock"
      And the "build" phase caches "target" keyed on "**/Cargo.lock"
      When I run "cidx generate github"
      Then the cache keys of the "test" and "build" jobs should differ

    Scenario: A phase that declares no cache gets no cache step
      Given cidx.toml defines pipeline "ci" with phases "security, build"
      And the "build" phase caches "target" keyed on "**/Cargo.lock"
      When I run "cidx generate github"
      Then the "security" job should have no cache step

    Scenario: A cache with nothing to key it on is refused
      Given cidx.toml defines pipeline "ci" with phases "build"
      And the "build" phase caches "target" keyed on nothing
      When I run "cidx generate github"
      Then generating should fail mentioning "cache_key"

    Scenario: A cache path that leaves the workspace is refused
      Given cidx.toml defines pipeline "ci" with phases "build"
      And the "build" phase caches "../outside" keyed on "**/Cargo.lock"
      When I run "cidx generate github"
      Then generating should fail mentioning "workspace"

  Rule: A cache can refuse to grow on every key change (#515)

    # The fallback restores the previous cache in full when the key misses, and
    # the build adds its new artefacts next to the old ones before saving under
    # the new key. A directory that never prunes itself -- cargo's target/ -- then
    # stacks a generation per key change: 3.33 GiB, then 5.44 GiB after one
    # version bump, toward GitHub's 10 GB repository quota. Turning the fallback
    # off makes the first run after a key change cold, and the cache it saves
    # holds only current artefacts, so its size stays flat. It stays on by
    # default: a project that prefers speed to size keeps it.

    Scenario: A phase can turn the fallback off
      Given cidx.toml defines pipeline "ci" with phases "test"
      And the "test" phase caches "target" keyed on "**/Cargo.lock" without falling back to an older cache
      When I run "cidx generate github"
      Then the "test" job should cache "target"
      And the cache key of the "test" job should hash "**/Cargo.lock"
      And the cache of the "test" job should not fall back to an older cache

  Rule: A phase can declare the evidence it uploads, so regeneration keeps it (#509)

    # A cluster phase uploads what it recorded after it runs, including when it
    # fails -- that is when the evidence matters. Such a step could only be added
    # to the generated workflow by hand, and the next `cidx generate` removed it:
    # upgrading cidx meant regenerating and re-adding it, and the evidence of a
    # failed run was lost the day someone forgot. A phase declares it instead.

    Scenario: A declared artifact is uploaded after the phase, whatever its outcome
      Given cidx.toml defines pipeline "ci" with phases "test"
      And the "test" phase uploads "test-evidence" from ".probatum/runs/*/" for 7 days
      When I run "cidx generate github"
      Then the "test" job should upload "test-evidence" after the phase runs, even when it fails
      And that upload should keep hidden files and only warn when nothing matches
      And that upload should keep the artifact for 7 days

    Scenario: A phase that declares no artifact uploads nothing
      Given cidx.toml defines pipeline "ci" with phases "security, test"
      And the "test" phase uploads "test-evidence" from ".probatum/runs/*/" for 7 days
      When I run "cidx generate github"
      Then the "security" job should have no upload step

    Scenario: An artifact path that leaves the workspace is refused
      Given cidx.toml defines pipeline "ci" with phases "test"
      And the "test" phase uploads "test-evidence" from "../outside" for 7 days
      When I run "cidx generate github"
      Then generating should fail mentioning "workspace"

    Scenario: Two phases cannot upload under the same artifact name
      Given cidx.toml defines pipeline "ci" with phases "test, build"
      And the "test" phase uploads "evidence" from "out-a/" for 7 days
      And the "build" phase uploads "evidence" from "out-b/" for 7 days
      When I run "cidx generate github"
      Then generating should fail mentioning "evidence"

    Scenario: The bootstrap artifact's name is not available
      Given cidx.toml defines pipeline "ci" with phases "test"
      And the "test" phase uploads "cidx-binary" from "out/" for 7 days
      When I run "cidx generate github"
      Then generating should fail mentioning "cidx-binary"

  Rule: Generate respects output options

    Scenario: Output to stdout by default
      When I run "cidx generate github"
      Then the output should be printed to stdout

    Scenario: Output to file with -o flag
      When I run "cidx generate github -o .github/workflows/cidx.yml"
      Then the file ".github/workflows/cidx.yml" should be created

  Rule: Generate handles edge cases

    Scenario: No pipelines defined
      Given cidx.toml has no pipelines defined
      When I run "cidx generate github"
      Then the command should fail
      And I should see "no pipelines defined"

    # `cidx generate` only has the platforms it can produce as subcommands, so
    # an unknown one is refused by the CLI before any generator runs. The
    # wording is urfave/cli's, not ours: the scenario used to claim an
    # "unsupported platform" message that no code ever printed (issue #265).
    Scenario: Unknown platform
      When I run "cidx generate unknown"
      Then the command should fail
      And I should see "No help topic for 'unknown'"
