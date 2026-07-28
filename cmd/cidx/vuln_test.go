package main

import "testing"

// TestStaleVulnerabilities: exceptions are keyed by `repo:tag`, so a promotion
// leaves the ones recorded against the version it replaced behind. They stop
// matching — correctly — and then nothing says so, which is how the file
// accumulated records for tags the catalogue passed long ago (#248).
func TestStaleVulnerabilities(t *testing.T) {
	running := map[string]bool{
		"golangci/golangci-lint:v2.12.2": true,
		"tmknom/prettier:3.6.2":          true,
	}

	entries := []Vulnerability{
		{CVE: "CVE-2025-0001", Image: "golangci/golangci-lint:v2.6.2"},
		{CVE: "CVE-2025-0002", Image: "golangci/golangci-lint:v2.12.2"},
		{CVE: "CVE-2025-0003", Image: "tmknom/prettier:3.6.2"},
		{CVE: "CVE-2025-0004", Image: "docker:29.0.4"},
	}

	stale := staleVulnerabilities(entries, running)

	want := []string{"CVE-2025-0001", "CVE-2025-0004"}
	if len(stale) != len(want) {
		t.Fatalf("stale entries = %v, want %v", stale, want)
	}
	for i, cve := range want {
		if stale[i].CVE != cve {
			t.Errorf("stale[%d].CVE = %q, want %q", i, stale[i].CVE, cve)
		}
	}
}

// TestStaleVulnerabilitiesIgnoresTheDigest: the catalogue is pinned
// `image:tag@sha256:...` (#242) while exceptions are recorded without the
// digest, so re-pinning a tag must not make every entry for it read as dead.
func TestStaleVulnerabilitiesIgnoresTheDigest(t *testing.T) {
	running := map[string]bool{"tmknom/prettier:3.6.2": true}

	entries := []Vulnerability{
		{CVE: "CVE-2025-0003", Image: "tmknom/prettier:3.6.2@sha256:" + zeroDigest},
	}

	if stale := staleVulnerabilities(entries, running); len(stale) != 0 {
		t.Errorf("stale entries = %v, want none: the digest is not part of the key", stale)
	}
}
