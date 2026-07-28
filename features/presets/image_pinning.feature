Feature: Catalogue Image Pinning
  As the maintainer of the built-in preset catalogue
  I want every catalogue image referenced by digest as well as by tag
  So that a tag republished with different content cannot silently change what runs

  Rule: A catalogue image is referenced as image:tag@sha256:...

    Scenario Outline: A built-in preset resolves to an immutable reference
      When I resolve the preset "<preset>" without overrides
      Then the resolved image should be pinned by digest

      Examples: One preset per registry the catalogue pulls from
        | preset      |
        | trivy       |
        | gitleaks    |
        | ansible-lint|
        | kaniko      |
        | probatum    |

    Scenario: The version stays readable next to the digest
      When I resolve the preset "gitleaks" without overrides
      Then the resolved image tag should be "v8.30.1"
      And the resolved image should be pinned by digest

  Rule: One image means one digest, whichever preset asks for it

    Scenario: Presets sharing an image share its digest
      Then the presets "ansible-lint" and "yamllint" should resolve the same image
