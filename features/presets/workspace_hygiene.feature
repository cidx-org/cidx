Feature: Presets Leave The Workspace As They Found It
  As a developer running a check from the catalogue
  I want the container to inspect my workspace without writing into it
  So that nothing I did not ask for — and may not be able to delete — is left
  behind in the bind mount

  # cidx mounts the workspace into every container, so whatever a tool writes to
  # its working directory lands in the user's repository carrying whatever
  # ownership the container ran with. #279 (ansible-lint's .ansible/) and #295
  # (go-mod-tidy's .bak copies) are one defect twice; the sweep they prompted
  # found two more caches doing it and three presets that could not run at all.

  Rule: A check that only inspects the workspace writes nothing into it

    Scenario: go-mod-tidy compares without making copies
      When I resolve the preset "go-mod-tidy" without overrides
      Then the resolved command should be "go mod tidy -diff"

      # It used to `cp go.mod go.mod.bak`, diff against the copy and never
      # remove it: two untracked files no .gitignore covers, left in the
      # workspace by a check that only reads it (#295).

    Scenario: ansible-lint removes the cache directory it creates
      When I resolve the preset "ansible-lint" without overrides
      Then the resolved command should contain "rmdir .ansible/* .ansible"

      # ansible_compat builds <project>/.ansible unconditionally and ignores
      # ANSIBLE_HOME while doing it, so the directory has to be cleared after
      # the run rather than pointed elsewhere. `rmdir`, not `rm -rf`: a cache
      # that actually holds collections refuses to be removed (#279).

    Scenario: ruff lints without leaving a cache directory
      When I resolve the preset "ruff" without overrides
      Then the resolved command should contain "--no-cache"

    Scenario: pytest keeps its cache and its bytecode out of the mount
      When I resolve the preset "pytest" without overrides
      Then the resolved environment should set "PYTHONDONTWRITEBYTECODE" to "1"
      And the resolved environment should set "PYTEST_ADDOPTS" to "-o cache_dir=/tmp/.pytest_cache"

  Rule: A preset whose image entrypoint is the tool passes arguments only

    # Naming the tool again makes it the tool's own first argument, and the
    # image answers `Unknown argument: commitlint` (#278), `unknown command
    # "goreleaser" for "goreleaser release"`, `docker: unknown command: docker
    # sh`. All three are presets that could never run, in the part of the
    # catalogue this repo's cidx.toml puts in no phase — so nothing dogfoods
    # them and nothing noticed. A preset that genuinely needs a shell around
    # such an image clears the entrypoint on purpose, the way gh-release and
    # commitizen do; these do not.

    # The range is shown resolved, which is what the container is handed: the
    # preset declares FROM and TO as defaults, and since #384 a scenario sees
    # the same expansion the executor performs.
    Scenario: commitlint receives a commit range, not its own name
      When I resolve the preset "commitlint" without overrides
      Then the resolved command should be "--default-config --from origin/main --to HEAD"

    Scenario: goreleaser receives a subcommand, not its own name
      When I resolve the preset "goreleaser" without overrides
      Then the resolved command should be "release --clean"

    Scenario: docker-buildx receives docker arguments, not a shell
      When I resolve the preset "docker-buildx" without overrides
      Then the resolved command should contain "buildx build"
      And the resolved command should not contain "sh -c"

      # Clearing the entrypoint would not have rescued the shell form either:
      # this DHI variant ships no shell to exec.
