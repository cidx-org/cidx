package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v2/pkg/presets"
)

// stubRegistry replaces the two network calls scan-targets makes for the
// duration of a test: the tag listing and the digest resolution. Nothing here
// reaches a registry.
func stubRegistry(t *testing.T, latestTag string, published time.Time, tagErr error) {
	t.Helper()

	originalLatest, originalDigest := latestTagFunc, resolveDigestFunc
	t.Cleanup(func() {
		latestTagFunc, resolveDigestFunc = originalLatest, originalDigest
	})

	latestTagFunc = func(image, currentTag string, now time.Time) (tagUpdate, error) {
		return tagUpdate{Latest: latestTag, Published: published}, tagErr
	}
	resolveDigestFunc = func(image, tag string) (string, error) {
		return "sha256:" + zeroDigest, nil
	}
}

// stubFrozenVariant stands in a registry that has stopped publishing the
// variant family the catalogue pins and moved to another one (#252).
func stubFrozenVariant(t *testing.T, currentTag, superseding string) {
	t.Helper()

	original := latestTagFunc
	t.Cleanup(func() { latestTagFunc = original })
	latestTagFunc = func(image, tag string, now time.Time) (tagUpdate, error) {
		return tagUpdate{Latest: currentTag, Superseding: superseding}, nil
	}
}

// stubDigestResolution replaces only the digest resolution, for the tests that
// turn on what the registry says about a reference the catalogue already runs.
// Apply it after stubRegistry, which sets a resolving default.
func stubDigestResolution(t *testing.T, resolve func(image, reference string) (string, error)) {
	t.Helper()

	original := resolveDigestFunc
	t.Cleanup(func() { resolveDigestFunc = original })
	resolveDigestFunc = resolve
}

const testNow = "2026-07-28T09:00:00Z"

func scanNow(t *testing.T) time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, testNow)
	if err != nil {
		t.Fatalf("bad test clock: %v", err)
	}
	return now
}

func onlyTarget(t *testing.T, targets []scanTarget) scanTarget {
	t.Helper()
	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}
	return targets[0]
}

// TestBuildScanTargetsPromotesAfterTheCooldown: an aged candidate becomes the
// image the workflow scans and promotes.
func TestBuildScanTargetsPromotesAfterTheCooldown(t *testing.T) {
	now := scanNow(t)
	stubRegistry(t, "3.7.0", now.AddDate(0, 0, -20), nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{"tmknom/prettier:3.6.2@sha256:" + zeroDigest: {"prettier"}},
		nil, now))

	if !target.IsUpdate {
		t.Fatalf("a 20-day-old candidate should be promoted, got: %s", target.PolicyReason)
	}
	want := "tmknom/prettier:3.7.0@sha256:" + zeroDigest
	if target.ScanImage != want {
		t.Errorf("ScanImage = %q, want the pinned candidate %q", target.ScanImage, want)
	}
	if target.CandidateImage != want {
		t.Errorf("CandidateImage = %q, want %q", target.CandidateImage, want)
	}
	if target.AgeDays == nil || *target.AgeDays != 20 {
		t.Errorf("AgeDays = %v, want 20", target.AgeDays)
	}
	if target.PublishedAt != "2026-07-08T09:00:00Z" {
		t.Errorf("PublishedAt = %q, want the candidate's publication date", target.PublishedAt)
	}
	if len(target.CVEWaiver) != 0 {
		t.Errorf("CVEWaiver = %v, want none", target.CVEWaiver)
	}
}

// TestBuildScanTargetsHoldsAFreshCandidate: the held candidate stays reported,
// but the workflow keeps scanning — and running — the current image.
func TestBuildScanTargetsHoldsAFreshCandidate(t *testing.T) {
	now := scanNow(t)
	current := "tmknom/prettier:3.6.2@sha256:" + zeroDigest
	stubRegistry(t, "3.7.0", now.AddDate(0, 0, -3), nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"prettier"}}, nil, now))

	if target.IsUpdate {
		t.Fatal("a 3-day-old candidate should not be promoted")
	}
	if target.ScanImage != current {
		t.Errorf("ScanImage = %q, want the current image %q", target.ScanImage, current)
	}
	if target.CandidateImage == "" {
		t.Error("CandidateImage is empty: a held candidate must stay visible, not be dropped")
	}
	if target.PolicyReason == "" {
		t.Error("PolicyReason is empty: a hold has to say why")
	}
	if target.AgeDays == nil || *target.AgeDays != 3 {
		t.Errorf("AgeDays = %v, want 3", target.AgeDays)
	}
}

// TestBuildScanTargetsHoldsAnUndatableCandidate is the fail-closed case: the
// same posture as an unresolvable digest in rule 1.
func TestBuildScanTargetsHoldsAnUndatableCandidate(t *testing.T) {
	now := scanNow(t)
	current := "tmknom/prettier:3.6.2@sha256:" + zeroDigest
	stubRegistry(t, "3.7.0", time.Time{}, nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"prettier"}}, nil, now))

	if target.IsUpdate {
		t.Fatal("a candidate with no publication date should not be promoted")
	}
	if target.ScanImage != current {
		t.Errorf("ScanImage = %q, want the current image %q", target.ScanImage, current)
	}
	if target.PublishedAt != "" {
		t.Errorf("PublishedAt = %q, want empty when the registry gave no date", target.PublishedAt)
	}
	if target.AgeDays != nil {
		t.Errorf("AgeDays = %d, want unreported", *target.AgeDays)
	}
	if target.CandidateImage == "" || target.PolicyReason == "" {
		t.Error("an undatable candidate must be reported, not silently discarded")
	}
}

// TestBuildScanTargetsWaivesTheCooldownForAffectingCVEs: rule 3, keyed on the
// exceptions already recorded for the running image.
func TestBuildScanTargetsWaivesTheCooldownForAffectingCVEs(t *testing.T) {
	now := scanNow(t)
	current := "tmknom/prettier:3.6.2@sha256:" + zeroDigest
	stubRegistry(t, "3.7.0", now.AddDate(0, 0, -3), nil)

	// Keyed by repository, exactly as known-vulnerabilities.toml records it
	// (#238): neither the tag nor the digest the catalogue is pinned with is
	// part of the lookup.
	affecting := map[string][]string{
		"tmknom/prettier": {"CVE-2025-27209"},
	}

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"prettier"}}, affecting, now))

	if !target.IsUpdate {
		t.Fatalf("a fix for a vulnerability affecting us should be promoted, got: %s", target.PolicyReason)
	}
	if len(target.CVEWaiver) != 1 || target.CVEWaiver[0] != "CVE-2025-27209" {
		t.Errorf("CVEWaiver = %v, want [CVE-2025-27209]: the PR states which CVE bought the waiver", target.CVEWaiver)
	}
}

// TestBuildScanTargetsReportsAnUndatableVersionRatherThanAnEternalCandidate is
// the trap #245 had to avoid: ghcr.io and dhi.io list their tags but date none
// of them, and the cooldown is fail-closed. Offering a candidate there would
// hold it in every weekly run for ever, so the newer version is reported as
// something to pin by hand instead.
func TestBuildScanTargetsReportsAnUndatableVersionRatherThanAnEternalCandidate(t *testing.T) {
	now := scanNow(t)
	current := "ghcr.io/astral-sh/ruff:0.8.2@sha256:" + zeroDigest
	stubRegistry(t, "0.16.0", time.Time{}, nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"ruff"}}, nil, now))

	if target.NewerVersion != "0.16.0" {
		t.Errorf("NewerVersion = %q, want 0.16.0: the version has to stay visible", target.NewerVersion)
	}
	if target.CandidateImage != "" {
		t.Errorf("CandidateImage = %q, want none: an undatable version can never clear the cooldown", target.CandidateImage)
	}
	if target.IsUpdate || target.ScanImage != current {
		t.Errorf("ScanImage = %q, IsUpdate = %v; want the current image and no promotion", target.ScanImage, target.IsUpdate)
	}
	if target.AgeDays != nil || target.PublishedAt != "" {
		t.Error("an undatable version must claim neither an age nor a publication date")
	}
	if !strings.Contains(target.PolicyReason, "ghcr.io") {
		t.Errorf("PolicyReason = %q, want it to name the registry that publishes no date", target.PolicyReason)
	}

	// container-monitor.yml selects this state with jq, so the field name is
	// part of the contract.
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to decode scan target: %v", err)
	}
	if _, ok := decoded["newer_version"]; !ok {
		t.Error("field \"newer_version\" missing from scan-targets JSON: container-monitor.yml reads it")
	}
	if _, ok := decoded["candidate_image"]; ok {
		t.Error("candidate_image is set although no promotable candidate exists")
	}
}

// TestBuildScanTargetsFlagsAPinnedImageThatIsGone: the reference the catalogue
// runs no longer exists. #244 found two catalogue images deleted upstream, and
// nothing had noticed until the presets failed to start.
func TestBuildScanTargetsFlagsAPinnedImageThatIsGone(t *testing.T) {
	now := scanNow(t)
	current := "dhi.io/alpine-base:3.21@sha256:" + zeroDigest
	stubRegistry(t, "3.21", time.Time{}, nil) // nothing newer on offer
	stubDigestResolution(t, func(image, reference string) (string, error) {
		return "", fmt.Errorf("%s: %w", image+"@"+reference, errImageNotFound)
	})

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"test-hot-reload"}}, nil, now))

	if !target.Missing {
		t.Fatal("a pinned image the registry answers 404 for must be reported missing")
	}
	if !strings.Contains(target.Error, "gone") {
		t.Errorf("Error = %q, want it to say the image is gone", target.Error)
	}

	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}
	if !strings.Contains(string(encoded), `"missing":true`) {
		t.Errorf("scan target JSON = %s, want a missing flag container-monitor.yml can select", encoded)
	}
}

// TestBuildScanTargetsDoesNotCallAnUnverifiableImageMissing: a registry we hold
// no credentials for answers 401, not 404. Calling that a deleted image would
// make the loudest signal the command has a false alarm.
func TestBuildScanTargetsDoesNotCallAnUnverifiableImageMissing(t *testing.T) {
	now := scanNow(t)
	current := "dhi.io/trivy:0.68@sha256:" + zeroDigest
	stubRegistry(t, "0.68", time.Time{}, nil)
	stubDigestResolution(t, func(_, _ string) (string, error) {
		return "", fmt.Errorf("registry dhi.io returned HTTP 401")
	})

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"trivy"}}, nil, now))

	if target.Missing {
		t.Error("an unauthenticated lookup is not a deletion")
	}
	if target.Error == "" {
		t.Error("the failed verification must still be reported")
	}
}

// TestBuildScanTargetsReportsAFrozenVariantLine: the pinned variant family has
// no successor and never will, because the repository stopped publishing it.
// Within the family that reads as up to date, which is how the catalogue sat on
// an abandoned line without anything noticing (#252).
func TestBuildScanTargetsReportsAFrozenVariantLine(t *testing.T) {
	now := scanNow(t)
	current := "dhi.io/golang:1.23-alpine3.21-dev@sha256:" + zeroDigest
	stubRegistry(t, "1.23-alpine3.21-dev", time.Time{}, nil)
	stubFrozenVariant(t, "1.23-alpine3.21-dev", "-alpine3.24-dev")

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"golang-build"}}, nil, now))

	if target.FrozenVariant != "-alpine3.24-dev" {
		t.Errorf("FrozenVariant = %q, want the family that replaced the pinned one", target.FrozenVariant)
	}
	if target.CandidateImage != "" || target.NewerVersion != "" {
		t.Error("a frozen line offers no candidate: repinning across variant families changes the base image")
	}
	if target.IsUpdate || target.ScanImage != current {
		t.Errorf("ScanImage = %q, IsUpdate = %v; want the current image and no promotion", target.ScanImage, target.IsUpdate)
	}
	if target.Missing {
		t.Error("a frozen line still pulls: it is abandoned, not deleted")
	}
	if !strings.Contains(target.PolicyReason, "-alpine3.24-dev") {
		t.Errorf("PolicyReason = %q, want it to name the family upstream moved to", target.PolicyReason)
	}

	// container-monitor.yml selects this state with jq, so the field name is
	// part of the contract.
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}
	if !strings.Contains(string(encoded), `"frozen_variant":"-alpine3.24-dev"`) {
		t.Errorf("scan target JSON = %s, want a frozen_variant field container-monitor.yml can select", encoded)
	}
}

// TestBuildScanTargetsLeavesAHealthyLineAlone: an image whose family is simply
// current must carry no frozen verdict, or the summary fills with a line that
// never resolves.
func TestBuildScanTargetsLeavesAHealthyLineAlone(t *testing.T) {
	now := scanNow(t)
	current := "dhi.io/trivy:0.68@sha256:" + zeroDigest
	stubRegistry(t, "0.68", time.Time{}, nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"trivy"}}, nil, now))

	if target.FrozenVariant != "" || target.PolicyReason != "" {
		t.Errorf("FrozenVariant = %q, PolicyReason = %q; want an up-to-date image to claim neither",
			target.FrozenVariant, target.PolicyReason)
	}
}

// TestBuildScanTargetsReportsATagThatCarriesNoVersion: the third state #328
// names. `buildpack-deps:trixie-curl` and `koalaman/shellcheck:stable` are
// names, not versions, so no tag a registry lists can be newer than them and
// the promotion path — which compares versions end to end — will never have
// anything to say. Their updates arrive as rebuilds of the same tag under a new
// digest, which nothing here detects.
//
// Saying nothing would file them under "Current (no updates)" beside the images
// that are genuinely current, which is the shape of rot #244 and #252 were both
// about: an image nothing is watching, reported as fine.
func TestBuildScanTargetsReportsATagThatCarriesNoVersion(t *testing.T) {
	now := scanNow(t)
	current := "buildpack-deps:trixie-curl@sha256:" + zeroDigest
	stubRegistry(t, "trixie-curl", time.Time{}, nil)

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"cargo-audit"}}, nil, now))

	if !target.UnversionedTag {
		t.Fatal("a pinned tag carrying no version must be reported, not filed as up to date")
	}
	if target.CandidateImage != "" || target.NewerVersion != "" || target.FrozenVariant != "" {
		t.Error("a tag that is a name offers no candidate: nothing can be shown to be newer than it")
	}
	if target.IsUpdate || target.ScanImage != current {
		t.Errorf("ScanImage = %q, IsUpdate = %v; want the current image and no promotion", target.ScanImage, target.IsUpdate)
	}
	if target.Missing {
		t.Error("the image pulls: it is unversioned, not deleted")
	}
	if !strings.Contains(target.PolicyReason, "rebuild") {
		t.Errorf("PolicyReason = %q, want it to name the rebuilds this path cannot see", target.PolicyReason)
	}

	// container-monitor.yml selects this state with jq, so the field name is
	// part of the contract.
	encoded, err := json.Marshal(target)
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}
	if !strings.Contains(string(encoded), `"unversioned_tag":true`) {
		t.Errorf("scan target JSON = %s, want an unversioned_tag field container-monitor.yml can select", encoded)
	}
}

// TestNewestOfHoldsEveryRegistryToTheSameRule is the regression #328 is about.
//
// Docker Hub and Quay.io used to run their own selection: a bare semver regex
// over a listing ordered by push date, first hit wins, with a hardcoded list of
// seven known variant suffixes standing in for the variant family rule. Against
// the listing below that picked `26.10` — an Ubuntu development branch — as the
// successor to a Debian `trixie-curl`, because `-curl` was not on the list and
// `trixie` is not a number.
//
// Every registry now reads its listing through the same presets.NewerTag, so
// this test stands the real Docker Hub answer in front of that one path.
func TestNewestOfHoldsEveryRegistryToTheSameRule(t *testing.T) {
	now := scanNow(t)
	buildpackDeps := tagListing{Tags: []string{
		"latest", "stable", "stable-curl", "curl", "trixie", "trixie-curl",
		"bookworm", "bookworm-curl", "sid", "sid-curl", "testing", "testing-curl",
		"stonking", "stonking-curl", "26.10", "26.10-curl",
		"resolute", "resolute-curl", "26.04", "26.04-curl",
		"noble", "noble-curl", "24.04", "24.04-curl",
	}}

	// The reported case: the pinned tag is a Debian suite name, so nothing in
	// the listing is comparable to it — least of all an Ubuntu release number.
	if got := newestOf("trixie-curl", buildpackDeps, now); got.Latest != "trixie-curl" {
		t.Errorf("newestOf(\"trixie-curl\") = %q, want the pinned tag back: a suite name has no successor", got.Latest)
	}
	if got := newestOf("trixie", buildpackDeps, now); got.Latest != "trixie" {
		t.Errorf("newestOf(\"trixie\") = %q, want the pinned tag back", got.Latest)
	}

	// A listing that does resolve still carries the date the cooldown will be
	// measured against — the Docker Hub path used to return it alongside the
	// tag, and it has to survive going through the shared selection.
	published, err := time.Parse(time.RFC3339, "2026-07-28T12:00:00Z")
	if err != nil {
		t.Fatalf("bad test date: %v", err)
	}
	prettier := tagListing{
		Tags:      []string{"latest", "3.9.4", "3.9.5", "3.9.6", "3.9.6-alpine"},
		Published: map[string]time.Time{"3.9.6": published},
	}

	got := newestOf("3.9.4", prettier, now)
	if got.Latest != "3.9.6" {
		t.Fatalf("newestOf(\"3.9.4\") = %q, want 3.9.6", got.Latest)
	}
	if !got.Published.Equal(published) {
		t.Errorf("Published = %v, want %v: without it the cooldown cannot be measured", got.Published, published)
	}
}

// TestCatalogueImagesIgnoresProjectPresets: `cidx preset scan-targets` governs
// the built-in catalogue. A project's own .cidx/presets.toml is nobody else's
// business, and letting one in had the promote job run a `sed` against
// pkg/presets/presets.toml for an image that is not in it (#248).
func TestCatalogueImagesIgnoresProjectPresets(t *testing.T) {
	// presets.Catalogue reads the catalogue file relative to the working
	// directory, so staging one is enough to stand in for the real thing.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pkg", "presets"), 0o750); err != nil {
		t.Fatalf("failed to stage the catalogue directory: %v", err)
	}
	catalogue := `
[presets.trivy]
name = "trivy"
image = "dhi.io/trivy:0.68@sha256:` + zeroDigest + `"
phase = "security"
`
	if err := os.WriteFile(filepath.Join(root, "pkg", "presets", "presets.toml"), []byte(catalogue), 0o600); err != nil {
		t.Fatalf("failed to stage the catalogue: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, ".cidx"), 0o750); err != nil {
		t.Fatalf("failed to stage the project directory: %v", err)
	}
	project := `
[presets.dogfood-check]
name = "dogfood-check"
image = "alpine:latest"
phase = "test"
`
	if err := os.WriteFile(filepath.Join(root, ".cidx", "presets.toml"), []byte(project), 0o600); err != nil {
		t.Fatalf("failed to stage the project presets: %v", err)
	}

	t.Chdir(root)

	images, err := catalogueImages()
	if err != nil {
		t.Fatalf("catalogueImages() error = %v", err)
	}

	if _, ok := images["alpine:latest"]; ok {
		t.Error("a project preset reached the scan targets: the policy governs the catalogue only")
	}
	if len(images) != 1 {
		t.Errorf("catalogueImages() = %v, want the catalogue image alone", images)
	}
	if got := images["dhi.io/trivy:0.68@sha256:"+zeroDigest]; len(got) != 1 || got[0] != "trivy" {
		t.Errorf("presets for the catalogue image = %v, want [trivy]", got)
	}
}

// TestRegistryImagesVersusCatalogue: `cidx preset images` feeds the daily
// security audit, which governs the images presets.toml ships. Reading the
// resolved registry made the audit scan a checked-out project's own presets too
// -- the same leak scan-targets had in #248, one file further along. The
// default stays the resolved registry, like `preset list` and `preset scan`;
// --catalogue is what the workflow asks for.
func TestRegistryImagesVersusCatalogue(t *testing.T) {
	const name, image = "cidx-test-project-preset", "example.invalid/project:1.0"

	presets.GlobalRegistry[name] = presets.Preset{Name: name, Image: image, Phase: "test"}
	t.Cleanup(func() { delete(presets.GlobalRegistry, name) })

	merged := registryImages()
	if got := merged[image]; len(got) != 1 || got[0] != name {
		t.Errorf("the default listing must keep a project preset, got %v", got)
	}

	catalogue, err := catalogueImages()
	if err != nil {
		t.Fatalf("catalogueImages() error = %v", err)
	}
	if _, ok := catalogue[image]; ok {
		t.Error("a project preset reached --catalogue: the audit governs the catalogue only")
	}
	if len(catalogue) >= len(merged) {
		t.Errorf("expected --catalogue to be a strict subset, got %d images against %d", len(catalogue), len(merged))
	}
}

// TestRegistryDatesTags pins which registries can serve the cooldown at all —
// the fact the whole undatable path turns on (#245).
func TestRegistryDatesTags(t *testing.T) {
	tests := map[string]bool{
		"tmknom/prettier":                true,
		"alpine":                         true,
		"quay.io/team/tool":              true,
		"gcr.io/kaniko-project/executor": true,
		"ghcr.io/astral-sh/ruff":         false,
		"dhi.io/trivy":                   false,
	}

	for image, want := range tests {
		if got := registryDatesTags(image); got != want {
			t.Errorf("registryDatesTags(%q) = %v, want %v", image, got, want)
		}
	}
}

// TestBuildScanTargetsKeepsCurrentImageOnLookupError: an unreachable registry
// yields no candidate, and the current image is what gets scanned.
func TestBuildScanTargetsKeepsCurrentImageOnLookupError(t *testing.T) {
	now := scanNow(t)
	current := "ghcr.io/astral-sh/ruff:0.8.2@sha256:" + zeroDigest
	stubRegistry(t, "", time.Time{}, fmt.Errorf("registry ghcr.io returned HTTP 500 listing tags"))

	target := onlyTarget(t, buildScanTargets(
		map[string][]string{current: {"ruff"}}, nil, now))

	if target.IsUpdate || target.ScanImage != current {
		t.Errorf("ScanImage = %q, IsUpdate = %v; want the current image and no promotion", target.ScanImage, target.IsUpdate)
	}
	if target.Error == "" {
		t.Error("the lookup failure must be reported")
	}
	if target.CandidateImage != "" {
		t.Errorf("CandidateImage = %q, want none: no candidate was found", target.CandidateImage)
	}
}

// TestScanTargetJSONContract pins the field names container-monitor.yml reads
// with jq. Renaming one here silently breaks the workflow, not the build.
func TestScanTargetJSONContract(t *testing.T) {
	now := scanNow(t)
	stubRegistry(t, "3.7.0", now.AddDate(0, 0, -3), nil)

	targets := buildScanTargets(
		map[string][]string{"tmknom/prettier:3.6.2@sha256:" + zeroDigest: {"prettier"}},
		map[string][]string{"tmknom/prettier": {"CVE-2025-27209"}},
		now)

	encoded, err := json.Marshal(targets[0])
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to decode scan target: %v", err)
	}

	for _, field := range []string{
		"current_image", "scan_image", "is_update", "presets",
		"candidate_image", "published_at", "age_days", "policy_reason", "cve_waiver",
	} {
		if _, ok := decoded[field]; !ok {
			t.Errorf("field %q missing from scan-targets JSON: container-monitor.yml reads it", field)
		}
	}

	if got := decoded["age_days"]; got != float64(3) {
		t.Errorf("age_days = %v, want 3", got)
	}
	if got := decoded["is_update"]; got != true {
		t.Errorf("is_update = %v, want true", got)
	}
}

// TestScanTargetJSONOmitsPolicyFieldsWithoutCandidate: an image with nothing
// newer must not carry an empty policy verdict, or the workflow summary fills
// with noise.
func TestScanTargetJSONOmitsPolicyFieldsWithoutCandidate(t *testing.T) {
	now := scanNow(t)
	stubRegistry(t, "3.6.2", now, nil) // same tag as the current image

	targets := buildScanTargets(
		map[string][]string{"tmknom/prettier:3.6.2@sha256:" + zeroDigest: {"prettier"}}, nil, now)

	encoded, err := json.Marshal(targets[0])
	if err != nil {
		t.Fatalf("failed to encode scan target: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("failed to decode scan target: %v", err)
	}

	for _, field := range []string{"candidate_image", "published_at", "age_days", "policy_reason", "cve_waiver"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %q present although no candidate exists", field)
		}
	}
}

// TestKnownHighSeverityCVEsIndexesByRepository: rule 3 reads the exceptions the
// security audit already produced, keyed the way that file records them — by
// repository, so a promotion inside it keeps the waiver alive (#238).
func TestKnownHighSeverityCVEsIndexesByRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	content := `
[[vulnerabilities]]
cve = "CVE-2025-0002"
repository = "tmknom/prettier"
severity = "HIGH"

[[vulnerabilities]]
cve = "CVE-2025-0001"
repository = "tmknom/prettier"
severity = "CRITICAL"

[[vulnerabilities]]
cve = "CVE-2025-0001"
repository = "tmknom/prettier"
severity = "CRITICAL"

[[vulnerabilities]]
cve = "CVE-2025-0003"
repository = "tmknom/prettier"
severity = "MEDIUM"

[[vulnerabilities]]
cve = "CVE-2025-0004"
repository = "securego/gosec"
severity = "HIGH"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to stage vulnerability file: %v", err)
	}

	byImage := knownHighSeverityCVEs(path)

	got := byImage["tmknom/prettier"]
	want := []string{"CVE-2025-0001", "CVE-2025-0002"}
	if len(got) != len(want) {
		t.Fatalf("prettier CVEs = %v, want %v (MEDIUM excluded, duplicate collapsed)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("prettier CVEs = %v, want %v", got, want)
			break
		}
	}
	if len(byImage["securego/gosec"]) != 1 {
		t.Errorf("gosec CVEs = %v, want one entry", byImage["securego/gosec"])
	}
}

// TestKnownHighSeverityCVEsWithoutFileGrantsNoWaiver: no evidence means the
// cooldown stands, never the other way round.
func TestKnownHighSeverityCVEsWithoutFileGrantsNoWaiver(t *testing.T) {
	if got := knownHighSeverityCVEs(filepath.Join(t.TempDir(), "absent.toml")); len(got) != 0 {
		t.Errorf("knownHighSeverityCVEs on a missing file = %v, want none", got)
	}
}
