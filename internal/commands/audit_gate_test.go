package commands

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/presets"
	"github.com/urfave/cli/v2"
)

func writeFirstSeen(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "first-seen.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestReadFirstSeen pins the file the age comes from: both date spellings, a
// repository that holds slashes, an identifier read case-insensitively, and
// every malformed input refused rather than read as "nothing seen" — an empty
// map would grant every finding a fresh window.
func TestReadFirstSeen(t *testing.T) {
	seen, err := readFirstSeen(writeFirstSeen(t, `{
		"rust/cve-2026-0001": "2026-09-23T12:03:00Z",
		"ghcr.io/ansible/community-ansible-dev-tools/GHSA-aaaa-bbbb-cccc": "2026-09-01"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := seen[presets.AlertKey("rust", "CVE-2026-0001")]; got.IsZero() || got.Day() != 23 {
		t.Errorf("rust/CVE-2026-0001 = %v, want 2026-09-23 whatever the case of the ID", got)
	}
	if got := seen[presets.AlertKey("ghcr.io/ansible/community-ansible-dev-tools", "GHSA-AAAA-BBBB-CCCC")]; got.Month() != 9 || got.Day() != 1 {
		t.Errorf("a repository with slashes was split wrongly: %v", seen)
	}

	for name, body := range map[string]string{
		"not json":     `not json`,
		"a list":       `["rust/CVE-1"]`,
		"a bad date":   `{"rust/CVE-1": "yesterday"}`,
		"no repo part": `{"CVE-1": "2026-09-01"}`,
	} {
		if _, err := readFirstSeen(writeFirstSeen(t, body)); err == nil {
			t.Errorf("%s must be refused, not read as an empty source", name)
		}
	}
	if _, err := readFirstSeen(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing file must be refused")
	}
}

func graceContext(t *testing.T, args ...string) *cli.Context {
	t.Helper()
	set := flag.NewFlagSet("t", flag.ContinueOnError)
	set.Int("grace-days", 0, "")
	set.String("first-seen", "", "")
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	return cli.NewContext(cli.NewApp(), set, nil)
}

// TestGraceFromFlags: neither flag is the gate as it was; either alone is
// refused, because a window with no ages cannot be applied and ages with no
// window would do nothing (#322).
func TestGraceFromFlags(t *testing.T) {
	file := writeFirstSeen(t, `{}`)

	if g, err := graceFromFlags(graceContext(t)); g != nil || err != nil {
		t.Errorf("no flags must leave the gate as it was, got %v, %v", g, err)
	}
	if _, err := graceFromFlags(graceContext(t, "--grace-days", "7")); err == nil || !strings.Contains(err.Error(), "--first-seen") {
		t.Errorf("a window with no source must be refused naming --first-seen, got %v", err)
	}
	if _, err := graceFromFlags(graceContext(t, "--first-seen", file)); err == nil || !strings.Contains(err.Error(), "--grace-days") {
		t.Errorf("dates with no window must be refused naming --grace-days, got %v", err)
	}
	if _, err := graceFromFlags(graceContext(t, "--grace-days", "-1", "--first-seen", file)); err == nil {
		t.Error("a negative window must be refused")
	}
	g, err := graceFromFlags(graceContext(t, "--grace-days", "7", "--first-seen", file))
	if err != nil || g == nil || g.Days != 7 || g.FirstSeen == nil {
		t.Fatalf("a window and its source must give a Grace with an answered source, got %+v, %v", g, err)
	}
	if zero, err := graceFromFlags(graceContext(t, "--grace-days", "0", "--first-seen", file)); err != nil || zero == nil || zero.Days != 0 {
		t.Errorf("an explicit zero-day window is legitimate, got %+v, %v", zero, err)
	}
}

// TestUnansweredFindingsAreThePopulationTheCountSays: the list the gate reads
// ages against must be the population the page counts, or the page would say
// one number and the gate judge another (#436).
func TestUnansweredFindingsAreThePopulationTheCountSays(t *testing.T) {
	finding := func(id, pkg string, kev bool) presets.Finding {
		return presets.Finding{ID: id, Severity: "HIGH", Package: pkg, PackageType: "deb", KEV: kev}
	}
	left := map[string][]presets.Finding{
		"rust:1.97@sha256:aa":        {finding("CVE-2026-1", "libfoo", false), finding("CVE-2026-2", "libbar", true)},
		"rust:1.97-slim@sha256:bb":   {finding("CVE-2026-1", "libfoo", false)},
		"pyfound/black:26@sha256:cc": {finding("CVE-2026-3", "libbaz", false)},
		"exempt:1@sha256:dd":         {{ID: "CVE-2026-4", Severity: "HIGH", Package: "linux-libc-dev", PackageType: "deb"}}, // kernel headers: exempt
		"fixed:1@sha256:ee":          {{ID: "CVE-2026-5", Severity: "HIGH", Package: "libqux", PackageType: "deb", FixedIn: "1.2"}},
	}
	got := unansweredFindings(left)
	if want := triageCatalogue(left).Actionable; len(got) != want {
		t.Fatalf("%d findings named, but the page counts %d: %+v", len(got), want, got)
	}
	var kev []string
	for _, f := range got {
		if f.KEV {
			kev = append(kev, f.Repository+"/"+f.ID)
		}
	}
	if len(kev) != 1 || kev[0] != "rust/CVE-2026-2" {
		t.Errorf("the known-exploited finding must be carried through, got %v", kev)
	}
}
