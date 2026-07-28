Feature: Catalogue Image Update Detection
  As the maintainer of the built-in preset catalogue
  I want the newest version picked out of the tag list a registry publishes
  So that the images on registries that only list tag names stop being invisible

  Rule: A newer version is only offered inside the same variant family

    Scenario: A variant tag is offered its own variant, never the bare version
      Given the registry lists the tags "1.24, 1.25, 1.24-alpine3.21-dev, 1.25-alpine3.24-dev"
      When I look for a version newer than "1.23-alpine3.21-dev"
      Then the newest version offered should be "1.24-alpine3.21-dev"

    Scenario: A variant whose family has moved on is offered nothing
      Given the registry lists the tags "1.24, 1.25, 1.25-alpine3.24-dev"
      When I look for a version newer than "1.23-alpine3.21-dev"
      Then no newer version should be offered

    Scenario: A bare version is not offered a variant
      Given the registry lists the tags "0.69-alpine, 0.69-fips"
      When I look for a version newer than "0.68"
      Then no newer version should be offered

  Rule: A version is offered at the precision the catalogue pinned

    Scenario: A two-component pin is offered a two-component version
      Given the registry lists the tags "0.71, 0.71.2, 0.72, 0.72.1"
      When I look for a version newer than "0.68"
      Then the newest version offered should be "0.72"

    Scenario: A three-component pin is offered a three-component version
      Given the registry lists the tags "0.71, 0.71.2, 0.72, 0.72.1"
      When I look for a version newer than "0.68.1"
      Then the newest version offered should be "0.72.1"

  Rule: Versions compare as numbers, because a tag listing carries no order

    Scenario: A two-digit minor ranks above a one-digit minor
      Given the registry lists the tags "v1.7.0, v1.9.2, v1.24.1, v1.24.0"
      When I look for a version newer than "v1.24.0"
      Then the newest version offered should be "v1.24.1"

    Scenario: The v prefix is part of the family
      Given the registry lists the tags "1.25.0"
      When I look for a version newer than "v1.24.0"
      Then no newer version should be offered

  Rule: Only a strictly newer version is offered

    Scenario: The running version is not offered back to itself
      Given the registry lists the tags "0.8.0, 0.8.1, 0.8.2"
      When I look for a version newer than "0.8.2"
      Then no newer version should be offered

    Scenario: Tags that carry no version are ignored
      Given the registry lists the tags "latest, initial-push, nightly, sha-1b2c3d4, 0.8.3"
      When I look for a version newer than "0.8.2"
      Then the newest version offered should be "0.8.3"

    Scenario: A running tag that carries no version can be compared to nothing
      Given the registry lists the tags "3.21, 3.22"
      When I look for a version newer than "latest"
      Then no newer version should be offered

  Rule: A variant line the registry stopped publishing is not up to date

    Scenario: A frozen variant line names the family that replaced it
      Given the registry lists the tags "1.25-alpine3.23-dev, 1.25-alpine3.24-dev, 1.26-alpine3.24-dev"
      When I look for the family that superseded "1.23-alpine3.21-dev"
      Then the superseding variant family should be "-alpine3.24-dev"

    Scenario: A family still published is not frozen, even at its own head
      Given the registry lists the tags "1.25-alpine3.23-dev, 1.26-alpine3.23-dev, 1.26-alpine3.24-dev"
      When I look for the family that superseded "1.26-alpine3.23-dev"
      Then no superseding variant family should be reported

    Scenario: A variant carrying no version names no line to lose
      Given the registry lists the tags "30-cli-dev, 31-alpine3.24"
      When I look for the family that superseded "29-cli"
      Then no superseding variant family should be reported
