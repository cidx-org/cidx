package presets

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// pinnedImagePattern describes the only reference form the catalogue accepts:
// `repo:tag@sha256:<64 hex>`. The tag stays readable, the digest makes the
// reference immutable.
var pinnedImagePattern = regexp.MustCompile(`^[^@\s]+:[^@:/\s]+@sha256:[0-9a-f]{64}$`)

// TestCatalogueImagesArePinnedByDigest is the standing guard behind rule 1 of
// the image supply-chain policy (issue #242, docs/core-concepts/security.md):
// every built-in preset image is pinned `image:tag@sha256:...`.
//
// A tag alone is mutable — `commitizen:4.15.1` can point at different content
// tomorrow with nothing in presets.toml changing — and no scanner catches that,
// because a backdoor has no CVE until someone finds it. This test is what keeps
// the rule true over time: a preset added or promoted with a bare tag fails
// here, including inside container-monitor.yml's promote job, which runs
// `go test ./pkg/...` before it opens a PR.
//
// It reads the catalogue directly rather than GlobalRegistry: the policy
// governs CIDX's own presets, not the ones a user or project layers on top via
// ~/.config/cidx/presets.toml or .cidx/presets.toml.
func TestCatalogueImagesArePinnedByDigest(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}
	if len(catalogue) == 0 {
		t.Fatal("catalogue is empty — the guard would pass vacuously")
	}

	var unpinned []string
	for name, preset := range catalogue {
		if preset.Image == "" {
			t.Errorf("preset %q has no image", name)
			continue
		}
		if !pinnedImagePattern.MatchString(preset.Image) {
			unpinned = append(unpinned, name+" -> "+preset.Image)
		}
	}

	if len(unpinned) > 0 {
		sort.Strings(unpinned)
		t.Errorf("%d catalogue image(s) are not pinned by digest:\n  %s\n\n"+
			"Every built-in image must be written image:tag@sha256:... (issue #242).\n"+
			"Resolve the digest of the multi-arch index with:\n"+
			"  docker buildx imagetools inspect --format '{{.Manifest.Digest}}' <image:tag>",
			len(unpinned), strings.Join(unpinned, "\n  "))
	}
}

// TestCatalogueImageDigestsAreConsistent: several presets share one image
// (seven of them run community-ansible-dev-tools). The same `repo:tag` must
// always carry the same digest, otherwise "pinned" would mean two different
// things depending on which preset you ran.
func TestCatalogueImageDigestsAreConsistent(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}

	digestByRef := make(map[string]string)
	presetByRef := make(map[string]string)

	for name, preset := range catalogue {
		ref, digest, found := strings.Cut(preset.Image, "@")
		if !found {
			continue // already reported by TestCatalogueImagesArePinnedByDigest
		}
		if seen, ok := digestByRef[ref]; ok && seen != digest {
			t.Errorf("%s is pinned to two different digests:\n  %s (%s)\n  %s (%s)",
				ref, seen, presetByRef[ref], digest, name)
			continue
		}
		digestByRef[ref] = digest
		presetByRef[ref] = name
	}
}

// TestCataloguePinsOneReferencePerRepository is what makes the repository-keyed
// alert fingerprint unambiguous (#313, docs/core-concepts/security.md).
//
// A triage alert is identified by its repository and its CVE, so that a repin
// does not close it and reopen it. The price of dropping the reference from the
// identity is that two references of one repository, pinned at the same time,
// would produce two alerts sharing an identity — GitHub would show one, and the
// second repin would never be surfaced. The catalogue pins one reference per
// repository today, and this is where that stops being an assumption.
//
// If it ever has to pin two — as it did while `rust:1.97.0` and
// `rust:1.97.0-slim` were both in (#287) — the alerts need a disambiguator that
// survives a repin, and that is a decision to take rather than a line to change
// here.
func TestCataloguePinsOneReferencePerRepository(t *testing.T) {
	catalogue, err := loadBasePresets()
	if err != nil {
		t.Fatalf("loadBasePresets() error = %v", err)
	}

	refByRepository := make(map[string]string)
	for _, preset := range catalogue {
		if preset.Image == "" {
			continue // already reported by TestCatalogueImagesArePinnedByDigest
		}
		repository := repositoryOf(preset.Image)
		if seen, ok := refByRepository[repository]; ok && seen != preset.Image {
			t.Errorf("%s is pinned twice at once:\n  %s\n  %s\n\n"+
				"Triage alerts are keyed on the repository so they survive a repin (#313),\n"+
				"so these two would publish alerts sharing an identity and only one would be shown.",
				repository, seen, preset.Image)
			continue
		}
		refByRepository[repository] = preset.Image
	}
}
