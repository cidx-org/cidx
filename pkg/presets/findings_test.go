package presets

import "testing"

// TestGoStdlibIsExemptOnlyInAGoBinary: the exemption is the Go standard library
// compiled into a CLI, not anything called `stdlib`. Both scanners' spellings of
// the ecosystem count; a package of that name from anywhere else does not.
func TestGoStdlibIsExemptOnlyInAGoBinary(t *testing.T) {
	cases := []struct {
		name    string
		finding Finding
		want    string
	}{
		{"trivy", Finding{Package: "stdlib", PackageType: "gobinary"}, ExemptGoStdlib},
		{"grype", Finding{Package: "stdlib", PackageType: "go-module"}, ExemptGoStdlib},
		{"python package named stdlib", Finding{Package: "stdlib", PackageType: "python-pkg"}, ""},
		{"another go module", Finding{Package: "github.com/opencontainers/runc", PackageType: "gobinary"}, ""},
	}

	for _, tc := range cases {
		if got := tc.finding.Exempt(); got != tc.want {
			t.Errorf("%s: Exempt() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestKernelHeadersAreExemptByExactName: the kernel is the host's. The match is
// on the exact package name — a prefix match on "linux" would swallow
// `util-linux` and `linux-pam`, which do real work inside the container.
func TestKernelHeadersAreExemptByExactName(t *testing.T) {
	cases := []struct {
		pkg  string
		want string
	}{
		{"linux-libc-dev", ExemptKernelHeaders},
		{"linux-headers", ExemptKernelHeaders},
		{"util-linux", ""},
		{"linux-pam", ""},
		{"libc6", ""},
	}

	for _, tc := range cases {
		finding := Finding{Package: tc.pkg, PackageType: "debian"}
		if got := finding.Exempt(); got != tc.want {
			t.Errorf("%s: Exempt() = %q, want %q", tc.pkg, got, tc.want)
		}
	}
}

// TestTheRunAndContainerdFindingsStayInTheQueue: the findings the catalogue
// actually accepted on the Ansible image are `gobinary` too. An exemption keyed
// on the ecosystem alone would have swallowed a container escape.
func TestTheRuncAndContainerdFindingsStayInTheQueue(t *testing.T) {
	for _, pkg := range []string{"github.com/opencontainers/runc", "github.com/containerd/containerd"} {
		finding := Finding{ID: "CVE-2025-31133", Package: pkg, PackageType: "gobinary"}
		if got := finding.Exempt(); got != "" {
			t.Errorf("%s was exempted as %q, but it is a real finding", pkg, got)
		}
	}
}

// TestSummariseLabelsByClassBeforeFixability: a Go stdlib finding is unreachable
// whether or not the Go team has shipped the fix yet, and most of them have one.
// Labelling by fix first would report zero stdlib findings on a catalogue full of
// them. The four counts have to add up to the carried total either way, or the
// baseline states something that is not a partition.
func TestSummariseLabelsByClassBeforeFixability(t *testing.T) {
	triage := Summarise([]Finding{
		{ID: "CVE-1", Package: "openssl", PackageType: "debian", FixedIn: "3.0.1"},
		{ID: "CVE-2", Package: "stdlib", PackageType: "gobinary"},
		{ID: "CVE-3", Package: "linux-libc-dev", PackageType: "debian"},
		{ID: "CVE-4", Package: "libxml2", PackageType: "debian"},
		{ID: "CVE-5", Package: "stdlib", PackageType: "gobinary", FixedIn: "1.26.6"},
	})

	if triage.Carried != 5 {
		t.Fatalf("Carried = %d, want 5", triage.Carried)
	}
	if triage.Fixable != 1 {
		t.Errorf("Fixable = %d, want 1: only the non-exempt fixable finding", triage.Fixable)
	}
	if triage.GoStdlib != 2 || triage.KernelHeaders != 1 {
		t.Errorf("GoStdlib = %d, KernelHeaders = %d, want 2 and 1: a fixable stdlib finding is still stdlib",
			triage.GoStdlib, triage.KernelHeaders)
	}
	if triage.Actionable != 1 {
		t.Errorf("Actionable = %d, want 1", triage.Actionable)
	}
	if sum := triage.Fixable + triage.GoStdlib + triage.KernelHeaders + triage.Actionable; sum != triage.Carried {
		t.Errorf("the split sums to %d, not to the %d carried", sum, triage.Carried)
	}
}

// TestSummariseCountsAFindingOnce: both scanners report the same vulnerability,
// and "596 carried" has to mean 596 things rather than 596 result lines.
func TestSummariseCountsAFindingOnce(t *testing.T) {
	triage := Summarise([]Finding{
		{ID: "CVE-1", Package: "linux-libc-dev", PackageType: "debian"},
		{ID: "cve-1", Package: "linux-libc-dev", PackageType: "deb"},
	})

	if triage.Carried != 1 {
		t.Errorf("Carried = %d, want 1: the same identifier however it is spelled", triage.Carried)
	}
}

// TestDisagreeingScannersKeepTheFindingInTheQueue: an exemption that is too
// broad hides what matters. Trivy calling a CVE a kernel-header finding while
// Grype calls it an OpenSSL one is not an exemption — it is a reason to look.
func TestDisagreeingScannersKeepTheFindingInTheQueue(t *testing.T) {
	triage := Summarise([]Finding{
		{ID: "CVE-1", Package: "linux-libc-dev", PackageType: "debian"},
		{ID: "CVE-1", Package: "openssl", PackageType: "deb"},
	})

	if triage.Actionable != 1 {
		t.Errorf("Actionable = %d, want 1: only a finding every scanner calls exempt is exempt", triage.Actionable)
	}
}

// TestOneScannerKnowingTheFixIsEnough: only one of the two carries `fix`
// consistently, and a fix does not stop existing because the other missed it.
func TestOneScannerKnowingTheFixIsEnough(t *testing.T) {
	triage := Summarise([]Finding{
		{ID: "CVE-1", Package: "runc", PackageType: "gobinary"},
		{ID: "CVE-1", Package: "runc", PackageType: "go-module", FixedIn: "1.2.8"},
	})

	if triage.Fixable != 1 {
		t.Errorf("Fixable = %d, want 1", triage.Fixable)
	}
}

// TestAddCountsPerImage: the same CVE on five images is five things to look at
// and five repins. Collapsing them would understate what the catalogue carries,
// which is the number the baseline exists to publish.
func TestAddCountsPerImage(t *testing.T) {
	one := Summarise([]Finding{{ID: "CVE-1", Package: "libxml2", PackageType: "debian"}})

	var total Triage
	total.Add(one)
	total.Add(one)

	if total.Carried != 2 || total.Actionable != 2 {
		t.Errorf("Carried = %d, Actionable = %d, want 2 and 2", total.Carried, total.Actionable)
	}
}

// TestAddNamesAKEVEntryOnce: unlike the counts, a KEV identifier is worth
// reading once however many images carry it.
func TestAddNamesAKEVEntryOnce(t *testing.T) {
	one := Summarise([]Finding{{ID: "CVE-1", Package: "openssl", PackageType: "debian", KEV: true, EPSS: 0.4}})

	var total Triage
	total.Add(one)
	total.Add(one)

	if len(total.KEV) != 1 {
		t.Errorf("KEV = %v, want the identifier named once", total.KEV)
	}
	if total.TopEPSS != 0.4 {
		t.Errorf("TopEPSS = %v, want the highest seen, not a sum", total.TopEPSS)
	}
}

// TestKEVAndEPSSAreReportedNotThresholded: they answer question 1, and the
// answer belongs to a human. What the code owes them is that the data survives
// the parse.
func TestKEVAndEPSSAreReportedNotThresholded(t *testing.T) {
	triage := Summarise([]Finding{
		{ID: "CVE-1", Package: "openssl", PackageType: "debian", EPSS: 0.10, KEV: true},
		{ID: "CVE-2", Package: "openssl", PackageType: "debian", EPSS: 0.02},
	})

	if len(triage.KEV) != 1 || triage.KEV[0] != "CVE-1" {
		t.Errorf("KEV = %v, want the one identifier CISA lists", triage.KEV)
	}
	if triage.TopEPSS != 0.10 {
		t.Errorf("TopEPSS = %v, want 0.10", triage.TopEPSS)
	}
	if triage.Actionable != 2 {
		t.Errorf("Actionable = %d, want 2: neither score removes a finding from the queue", triage.Actionable)
	}
}

// TestFixVersionMatchesCaseInsensitively: the identifier on file was written by
// whichever scanner reported it first, and the other one spells it differently.
func TestFixVersionMatchesCaseInsensitively(t *testing.T) {
	findings := []Finding{{ID: "ghsa-cgrx-mc8f-2prm", FixedIn: "1.2.8"}}

	if got := FixVersion(findings, "GHSA-cgrx-mc8f-2prm"); got != "1.2.8" {
		t.Errorf("FixVersion = %q, want 1.2.8", got)
	}
	if got := FixVersion(findings, "CVE-2026-0001"); got != "" {
		t.Errorf("FixVersion = %q, want empty for an identifier nothing reported", got)
	}
}
