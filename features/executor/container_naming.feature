Feature: Container Naming Is Scoped Per Project
  As a developer running cidx on several repositories from one machine
  I want each project to own its containers
  So that two repos never fight over the same container name or build cache

  Rule: Container names are scoped to the workspace

    Scenario: A tool gets a readable, project-scoped container name
      When I compute the container name for tool "trivy" in workspace "/home/dev/myrepo"
      Then the container name should start with "cidx_myrepo-"
      And the container name should end with "_trivy"
      And the container name should be a valid Docker container name

    Scenario: Two repositories sharing a basename get distinct container names
      When I compute the container name for tool "trivy" in workspace "/home/dev/work/api"
      And I compute the container name for tool "trivy" in workspace "/home/dev/personal/api"
      Then the two container names should differ

    Scenario: The same project always gets the same container name so reuse still works
      When I compute the container name for tool "trivy" in workspace "/home/dev/myrepo"
      And I compute the container name for tool "trivy" in workspace "/home/dev/myrepo"
      Then the two container names should be identical

    Scenario: The same project running two tools gets two container names
      When I compute the container name for tool "trivy" in workspace "/home/dev/myrepo"
      And I compute the container name for tool "gitleaks" in workspace "/home/dev/myrepo"
      Then the two container names should differ

  Rule: Names Docker would reject are sanitized

    Scenario Outline: Workspace paths with unusual characters still yield valid names
      When I compute the container name for tool "trivy" in workspace "<workspace>"
      Then the container name should be a valid Docker container name

      Examples:
        | workspace              |
        | /home/dev/My Repo      |
        | /home/dev/répo (copie) |
        | /home/dev/@@@          |
        | /                      |
