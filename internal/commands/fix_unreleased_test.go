package commands

import (
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// TestFixUnreleasedHasItsOwnRule: the general rule (never an exception for a
// fixed vulnerability) stands; the narrow category — the fix exists, but in no
// released image of the pinned tool — is held to a rule of its own.
func TestFixUnreleasedHasItsOwnRule(t *testing.T) {
	good := "CVE-2026-1 in brotli-libs of x/y:1, installed 1.2.0-r1. The fix, 1.2.1-r0, is not yet in any released image of y."

	cases := []struct {
		name  string
		cve   string
		notes string
		days  int
		ok    bool
	}{
		{"named, reasoned, monthly", "CVE-2026-1", good, 30, true},
		{"shorter is fine", "CVE-2026-1", good, 7, true},
		{"expiry beyond 30 days", "CVE-2026-1", good, 31, false},
		{"no expiry", "CVE-2026-1", good, 0, false},
		{"no reason", "CVE-2026-1", "", 30, false},
		{"reason names another advisory", "CVE-2026-2", good, 30, false},
		{"reason does not say what it waits for", "CVE-2026-1", "CVE-2026-1 is accepted.", 30, false},
	}
	for _, tc := range cases {
		err := checkFixUnreleased(tc.cve, tc.notes, tc.days)
		if tc.ok && err != nil {
			t.Errorf("%s: refused: %v", tc.name, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("%s: accepted, want refused", tc.name)
		}
	}
}

// TestFixUnreleasedDoesNotWeakenTheGeneralRule: the other statuses still meet
// the refusal; only the named category is routed around it.
func TestFixUnreleasedDoesNotWeakenTheGeneralRule(t *testing.T) {
	dir := t.TempDir()
	image := "ghcr.io/ansible/dev-tools:v26.7.1@sha256:" + zeroDigest
	writeTrivyResultAs(t, dir, "trivy-"+imageFileName.Replace(image)+".json", nil)
	writeJSON(t, dir+"/grype-"+imageFileName.Replace(image)+".json", map[string]any{
		"matches": []any{map[string]any{
			"vulnerability": map[string]any{
				"id": "CVE-2025-52881", "severity": "High",
				"fix": map[string]any{"versions": []any{"1.2.8"}},
			},
			"artifact": map[string]any{"name": "github.com/opencontainers/runc", "type": "go-module"},
		}},
	})

	if err := refuseIfFixedUpstream("CVE-2025-52881", image, dir); err == nil {
		t.Fatal("a fixed vulnerability was accepted without the fix-unreleased category")
	}
}

// TestPruneDoesNotSendAFixUnreleasedEntryToARepin: it has a fix by definition and
// nothing to repin to, so the FIXED UPSTREAM list leaves it out — and still
// names every other entry with a fix.
func TestPruneDoesNotSendAFixUnreleasedEntryToARepin(t *testing.T) {
	entries := []prunedEntry{
		{
			Vulnerability:    Vulnerability{CVE: "CVE-1", Repository: "a", Status: StatusFixUnreleased},
			ExceptionVerdict: presets.ExceptionVerdict{FixedIn: "1.2"},
		},
		{
			Vulnerability:    Vulnerability{CVE: "CVE-2", Repository: "b", Status: "accepted-risk"},
			ExceptionVerdict: presets.ExceptionVerdict{FixedIn: "3.4"},
		},
	}

	got := entriesFixedUpstream(entries)
	if len(got) != 1 || got[0].CVE != "CVE-2" {
		t.Errorf("entriesFixedUpstream = %+v, want only CVE-2", got)
	}
}

// retiredImages are the pins that carried the 28 blocking findings of build
// run-cidx-2fb814e. None may come back into the catalogue.
var retiredImages = []string{
	"jauderho/prettier:3.9.4",
	"ghcr.io/ansible/community-ansible-dev-tools:v26.7.1",
	"commitizen/commitizen:4.16.5",
	"maniator/gh:v2.95@",
	"buildpack-deps:trixie-curl@sha256:ef554489e8f8b0245ef3cb2742b4c6b6e53c43bd64366b13f624982c854af2eb",
	"dhi.io/docker:29-cli@sha256:b20e9ecc2dfb63c505d4442dc82b93fa36ccfb568a607d70ce0528f64ff678ca",
	"dhi.io/golang:1.26.5-alpine-dev",
	"ghcr.io/securego/gosec:2.28.0",
	"goreleaser/goreleaser:v2.17.0",
	"golangci/golangci-lint:v2.12.2-alpine",
	"commitlint/commitlint:21.2.1",
}

// TestTheCatalogueNoLongerPinsTheImagesWithBlockingFindings: the twelve images
// the Security page counted are re-pinned, and no preset still names the old pin.
func TestTheCatalogueNoLongerPinsTheImagesWithBlockingFindings(t *testing.T) {
	catalogue, err := presets.Catalogue()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	for name, p := range catalogue {
		for _, retired := range retiredImages {
			if strings.HasPrefix(p.Image, retired) {
				t.Errorf("preset %s still pins %s, which carried a blocking finding", name, p.Image)
			}
		}
	}
}

// TestEveryFindingLeftOnTheToolImagesIsANamedDatedException: where no released
// image of a tool is clean, the finding is on file as fix-unreleased, dated, with
// a reason that meets the category's rule, for a repository the catalogue runs.
func TestEveryFindingLeftOnTheToolImagesIsANamedDatedException(t *testing.T) {
	file, err := loadVulnerabilities("../../known-vulnerabilities.toml")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	catalogue, err := presets.Catalogue()
	if err != nil {
		t.Fatalf("catalogue: %v", err)
	}
	running := map[string]bool{}
	for _, p := range catalogue {
		running[imageRepository(p.Image)] = true
	}

	want := map[string]bool{
		"CVE-2026-123158|ghcr.io/securego/gosec": false,
		"GO-2026-6888|goreleaser/goreleaser":     false,
		"CVE-2026-111497|golangci/golangci-lint": false,
		"CVE-2026-111497|commitlint/commitlint":  false,
		"CVE-2026-123158|zricethezav/gitleaks":   false,
		"GO-2026-6888|zricethezav/gitleaks":      false,
	}

	for _, v := range file.Vulnerabilities {
		if v.Status != StatusFixUnreleased {
			continue
		}
		key := v.CVE + "|" + v.Repository
		if _, expected := want[key]; expected {
			want[key] = true
		}
		if !running[v.Repository] {
			t.Errorf("%s: the catalogue runs no image from %s", key, v.Repository)
		}
		added, err := time.Parse(time.DateOnly, v.Added)
		if err != nil {
			t.Errorf("%s: added %q: %v", key, v.Added, err)
			continue
		}
		expires, err := time.Parse(time.DateOnly, v.Expires)
		if err != nil {
			t.Errorf("%s: expires %q: %v", key, v.Expires, err)
			continue
		}
		if expires.After(added.AddDate(0, 0, fixUnreleasedMaxDays)) {
			t.Errorf("%s: expires %s, more than %d days after %s", key, v.Expires, fixUnreleasedMaxDays, v.Added)
		}
		if err := checkFixUnreleased(v.CVE, v.Notes, 1); err != nil {
			t.Errorf("%s: its reason fails the category's rule: %v", key, err)
		}
	}

	for key, found := range want {
		if !found {
			t.Errorf("%s: no fix-unreleased exception on file", key)
		}
	}
}
