Feature: Catalogue reference page
  As someone deciding whether to trust the built-in presets
  I want one committed page stating what each preset runs and under which policy
  So that "which container, and why" is read from the repository, not reconstructed

  # docs/reference/tools.md was written by hand and rotted the way every
  # hand-written inventory does: 18 presets listed out of 42, six images wrong,
  # one preset that no longer exists. The remedy is the baseline's (#310): the
  # page is generated from the catalogue, committed, and a test regenerates it
  # offline — a preset added or repinned without regenerating the page is a red
  # diff. Everything on the page derives from the declarations; the only human
  # field is the preset's own description.

  Rule: The page states every preset, from the catalogue alone

    Scenario: Every catalogue preset appears on the page
      When the catalogue reference page is rendered
      Then every built-in preset appears exactly once on the page

    Scenario: A preset row states what it runs and what that implies
      When the catalogue reference page is rendered
      Then the row for "goreleaser" states image "goreleaser/goreleaser:v2.17.0", phase "release" and capabilities "docker-socket, publishing-credential"

    Scenario: The page is deterministic
      When the catalogue reference page is rendered twice
      Then both renderings are byte-identical

  Rule: The update policy is stated per image, not assumed

    # Rule 2's cooldown needs a publication date, and dhi.io and ghcr.io
    # publish none — so their images can never be auto-promoted, and every
    # move is a reviewed manual repin. That difference decides who has to
    # watch what, so the page says it instead of the reader inferring it.

    Scenario: An image on a dated registry reads as automatically promoted
      When the catalogue reference page is rendered
      Then the row for "commitizen" states update policy "automatic"

    Scenario: An image on an undated registry reads as manually repinned
      When the catalogue reference page is rendered
      Then the row for "trivy" states update policy "manual repin"
