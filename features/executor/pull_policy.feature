Feature: Image Pull Policy
  As a developer
  I want to control when container images are pulled
  So that I don't waste bandwidth on images I already have locally

  Rule: Default pull policy depends on environment

    Scenario: Local environment defaults to if-not-present
      Given I am in local environment
      When I run a container with no pull_policy set
      Then the effective pull policy should be "if-not-present"

    Scenario: CI environment defaults to always
      Given I am in CI environment (github)
      When I run a container with no pull_policy set
      Then the effective pull policy should be "always"

  Rule: Pull policy can be overridden per container

    Scenario: Override pull policy in cidx.toml
      Given I am in local environment
      And container "megalinter" has pull_policy "always"
      When I run container "megalinter"
      Then the effective pull policy should be "always"

    Scenario: Never pull policy skips image pull
      Given I am in local environment
      And container "trivy" has pull_policy "never"
      When I run container "trivy"
      Then no image pull should occur

  Rule: if-not-present checks local images before pulling

    Scenario: Image exists locally - no pull
      Given I am in local environment
      And image "aquasec/trivy:0.68" exists locally
      When I run a container with pull_policy "if-not-present"
      Then no image pull should occur

    Scenario: Image missing locally - pull
      Given I am in local environment
      And image "aquasec/trivy:0.68" does NOT exist locally
      When I run a container with pull_policy "if-not-present"
      Then the image should be pulled
