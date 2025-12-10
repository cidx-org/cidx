Feature: Executor Backend Selection
  As a DevOps engineer
  I want CIDX to automatically select the best container runtime
  So that pipelines run regardless of which runtime is installed

  Background:
    Given I have a valid cidx.toml configuration

  Rule: CIDX auto-detects available container runtimes

    Scenario: Docker is available and selected automatically
      Given Docker daemon is running
      When I run "cidx run security"
      Then the backend should be "docker"
      And I should see "Backend: Docker (auto-detected)"

    Scenario: Podman is selected when Docker is unavailable
      Given Docker daemon is NOT running
      And Podman is available
      When I run "cidx run security"
      Then the backend should be "podman"
      And I should see "Backend: Podman (auto-detected)"

    Scenario: Error when no container runtime is available
      Given Docker daemon is NOT running
      And Podman is NOT available
      When I run "cidx run security"
      Then the command should fail
      And I should see "No container runtime available"
      And I should see suggestions to start Docker or Podman

  Rule: Users can force a specific backend

    Scenario: Force Docker backend
      Given Docker daemon is running
      When I run "cidx run security --backend docker"
      Then the backend should be "docker"
      And I should see "Backend: docker (forced)"

    Scenario: Force Podman backend
      Given Podman is available
      When I run "cidx run security --backend podman"
      Then the backend should be "podman"
      And I should see "Backend: podman (forced)"

    Scenario: Error when forcing unavailable backend
      Given Docker daemon is NOT running
      When I run "cidx run security --backend docker"
      Then the command should fail
      And I should see "Docker daemon is not running"

  Rule: Backend flag accepts multiple formats

    Scenario Outline: Backend flag variations
      Given Docker daemon is running
      When I run "cidx run security <flag>"
      Then the backend should be "docker"

      Examples:
        | flag              |
        | --backend docker  |
        | --backend=docker  |
        | -b docker         |

  Rule: Executor interface is consistent across backends

    Scenario: Docker executor implements Executor interface
      Given Docker daemon is running
      When I execute a tool via Docker
      Then the executor should have method "Run"
      And the executor should have method "Available"
      And the executor should have method "Name"
      And the executor should have method "Close"

    Scenario: All backends use same ContainerConfig
      Given any container runtime is available
      When I run a tool
      Then the ContainerConfig should contain:
        | field      |
        | Name       |
        | Phase      |
        | Image      |
        | Command    |
        | Workdir    |
        | Volumes    |
        | Env        |

  Rule: Dry-run works with backend selection

    Scenario: Dry-run shows selected backend
      Given Docker daemon is running
      When I run "cidx run security --dry-run"
      Then I should see "Backend: Docker (auto-detected)"
      And I should see "Would execute:"
      And no container should actually run

    Scenario: Dry-run works even when no runtime available
      Given Docker daemon is NOT running
      And Podman is NOT available
      When I run "cidx run security --dry-run"
      Then the command should fail
      And I should see "No container runtime available"
