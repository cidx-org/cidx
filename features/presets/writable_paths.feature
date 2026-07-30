Feature: Writable Paths For Non-Root Presets
  As a developer running a preset that installs its tool at runtime
  I want the install to land somewhere the container user can write
  So that the preset does not die on a permission error before doing any work

  # cidx runs every container as the invoking uid, never root (executor sets
  # User from os.Getuid). A preset whose command writes to a path only root owns
  # therefore fails on any image: #188 fixed cargo-audit's HOME/CARGO_HOME, and
  # the Python pack and go-mod-tidy carried the same defect (#238).

  Rule: A preset that installs its tool at runtime installs it under a writable HOME

    Scenario Outline: The Python pack has somewhere to install to
      When I resolve the preset "<preset>" without overrides
      Then the resolved environment should set "HOME" to "/tmp"

      # Without HOME, pip falls back to a user install under "/" and dies with
      # `Permission denied: '/.local'`.
      Examples: Every preset whose command is a pip install
        | preset       |
        | mypy         |
        | bandit       |
        | pip-audit    |
        | pytest       |
        | python-build |
        | twine        |

    Scenario Outline: The entry point pip wrote is on the path that invokes it
      When I resolve the preset "<preset>" without overrides
      Then the resolved command should contain "PATH=$PATH:/tmp/.local/bin"

      # pip puts console scripts in $HOME/.local/bin, which no image lists in
      # PATH; python-build is absent because `python -m build` runs a module.
      Examples: Every preset that invokes a console script
        | preset    |
        | mypy      |
        | bandit    |
        | pip-audit |
        | pytest    |
        | twine     |

  Rule: A Go preset downloads modules into a writable GOPATH

    Scenario Outline: The Go pack has somewhere to cache modules
      When I resolve the preset "<preset>" without overrides
      Then the resolved environment should set "GOPATH" to "/tmp/go"

      # The image default is /go, which a non-root container cannot create in:
      # `mkdir /go/pkg: permission denied`. go-mod-tidy was the one preset on
      # the shared golang image missing this, on any image, until #238.
      Examples: Every preset running the Go toolchain from the catalogue image
        | preset      |
        | go-mod-tidy |
        | go-test     |
        | go-build    |
        | godog       |
        | govulncheck |
