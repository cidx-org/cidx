package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	latestTagFunc = func(image, currentTag string) (string, time.Time, error) {
		return latestTag, published, tagErr
	}
	resolveDigestFunc = func(image, tag string) (string, error) {
		return "sha256:" + zeroDigest, nil
	}
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

	// Keyed by repo:tag, exactly as known-vulnerabilities.toml records it: the
	// digest the catalogue is pinned with must not be part of the lookup.
	affecting := map[string][]string{
		"tmknom/prettier:3.6.2": {"CVE-2025-27209"},
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

// TestBuildScanTargetsKeepsCurrentImageOnLookupError: an unreachable or
// unsupported registry yields no candidate, and the current image is what gets
// scanned (#245 — that is 9 of 21 catalogue images today).
func TestBuildScanTargetsKeepsCurrentImageOnLookupError(t *testing.T) {
	now := scanNow(t)
	current := "ghcr.io/astral-sh/ruff:0.8.2@sha256:" + zeroDigest
	stubRegistry(t, "", time.Time{}, fmt.Errorf("registry ghcr.io not supported yet"))

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
		map[string][]string{"tmknom/prettier:3.6.2": {"CVE-2025-27209"}},
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

// TestKnownHighSeverityCVEsIndexesByImage: rule 3 reads the exceptions the
// security audit already produced, keyed the way that file records them.
func TestKnownHighSeverityCVEsIndexesByImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-vulnerabilities.toml")
	content := `
[[vulnerabilities]]
cve = "CVE-2025-0002"
image = "tmknom/prettier:3.6.2"
severity = "HIGH"

[[vulnerabilities]]
cve = "CVE-2025-0001"
image = "tmknom/prettier:3.6.2"
severity = "CRITICAL"

[[vulnerabilities]]
cve = "CVE-2025-0001"
image = "tmknom/prettier:3.6.2"
severity = "CRITICAL"

[[vulnerabilities]]
cve = "CVE-2025-0003"
image = "tmknom/prettier:3.6.2"
severity = "MEDIUM"

[[vulnerabilities]]
cve = "CVE-2025-0004"
image = "securego/gosec:2.24.6"
severity = "HIGH"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to stage vulnerability file: %v", err)
	}

	byImage := knownHighSeverityCVEs(path)

	got := byImage["tmknom/prettier:3.6.2"]
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
	if len(byImage["securego/gosec:2.24.6"]) != 1 {
		t.Errorf("gosec CVEs = %v, want one entry", byImage["securego/gosec:2.24.6"])
	}
}

// TestKnownHighSeverityCVEsWithoutFileGrantsNoWaiver: no evidence means the
// cooldown stands, never the other way round.
func TestKnownHighSeverityCVEsWithoutFileGrantsNoWaiver(t *testing.T) {
	if got := knownHighSeverityCVEs(filepath.Join(t.TempDir(), "absent.toml")); len(got) != 0 {
		t.Errorf("knownHighSeverityCVEs on a missing file = %v, want none", got)
	}
}
