package presets

import (
	"encoding/json"
	"strings"
	"testing"
)

const scanTestImage = "dhi.io/golang:1.26.5-alpine-dev@sha256:0be40e8408fe1fa9fcfce328f13e76b0a39bc0268c564a1da50e90eb699a9709"

func scanTestLog(t *testing.T, scans []ImageScan, gaps []ScanGap) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(ScanSARIF(scans, gaps))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded
}

func scanTestResults(t *testing.T, log map[string]any) []map[string]any {
	t.Helper()
	run := log["runs"].([]any)[0].(map[string]any)
	results, ok := run["results"].([]any)
	if !ok {
		t.Fatalf("results is not an array: %v", run["results"])
	}
	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, r.(map[string]any))
	}
	return out
}

// TestScanReportsOneResultPerImagePackageAndVulnerability: Trivy and Grype
// seeing the same finding make one result, and the scanners that saw it travel
// in it. A package hit by two CVEs, or one CVE in two packages, is two results.
func TestScanReportsOneResultPerImagePackageAndVulnerability(t *testing.T) {
	scans := []ImageScan{{
		Image:  scanTestImage,
		UsedBy: []string{"go-test", "go-build"},
		Line:   42,
		Findings: []Finding{
			{ID: "CVE-2026-1", Severity: "HIGH", Package: "libssl3", FixedIn: "3.5.1", Scanner: "Trivy"},
			{ID: "cve-2026-1", Severity: "HIGH", Package: "libssl3", FixedIn: "3.5.1", Scanner: "Grype"},
			{ID: "CVE-2026-1", Severity: "HIGH", Package: "libcrypto3", FixedIn: "3.5.1", Scanner: "Trivy"},
			{ID: "CVE-2026-2", Severity: "CRITICAL", Package: "libssl3", Scanner: "Grype"},
		},
	}}

	results := scanTestResults(t, scanTestLog(t, scans, nil))
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3: %v", len(results), results)
	}

	byKey := map[string]map[string]any{}
	for _, r := range results {
		props := r["properties"].(map[string]any)
		byKey[r["ruleId"].(string)+" "+props["package"].(string)] = r
	}

	merged := byKey["CVE-2026-1 libssl3"]
	if merged == nil {
		t.Fatalf("no result for CVE-2026-1 in libssl3: %v", byKey)
	}
	props := merged["properties"].(map[string]any)
	if got := props["scanners"].([]any); len(got) != 2 {
		t.Errorf("scanners = %v, want both", got)
	}
	if props["image"] != scanTestImage {
		t.Errorf("image = %v, want the pinned reference", props["image"])
	}
	if props["fixedVersion"] != "3.5.1" || props["fixAvailable"] != true {
		t.Errorf("the fix is not reported: %v", props)
	}
	if !strings.Contains(merged["message"].(map[string]any)["text"].(string), "fixed in 3.5.1") {
		t.Errorf("the message does not say which version fixes it: %v", merged["message"])
	}

	unfixed := byKey["CVE-2026-2 libssl3"]["properties"].(map[string]any)
	if unfixed["fixAvailable"] != false {
		t.Errorf("a finding with no fix says one exists: %v", unfixed)
	}
	if _, has := unfixed["fixedVersion"]; has {
		t.Errorf("a finding with no fix carries a fixed version: %v", unfixed)
	}
	if unfixed["severity"] != "CRITICAL" {
		t.Errorf("severity = %v, want CRITICAL", unfixed["severity"])
	}
}

// TestScanKeepsTheFindingsTheTriageLeavesOut is behaviours 2 and 3: the audit's
// view publishes only what waits for a judgement, and this report is whole. A
// finding with a fix upstream is in it, and so is one a Go standard library
// exemption would have dropped from the tab.
func TestScanKeepsTheFindingsTheTriageLeavesOut(t *testing.T) {
	findings := []Finding{
		{ID: "CVE-2026-10", Severity: "HIGH", Package: "curl", PackageType: "apk", FixedIn: "8.9.0", Scanner: "Trivy"},
		{ID: "CVE-2026-11", Severity: "HIGH", Package: "stdlib", PackageType: "gobinary", Scanner: "Trivy"},
		{ID: "CVE-2026-12", Severity: "HIGH", Package: "busybox", PackageType: "apk", Scanner: "Grype"},
	}

	if len(TriageAlerts(scanTestImage, nil, findings, 1)) == len(findings) {
		t.Fatal("the audit's triage keeps everything, so this test proves nothing")
	}

	results := scanTestResults(t, scanTestLog(t, []ImageScan{{Image: scanTestImage, Findings: findings}}, nil))
	if len(results) != len(findings) {
		t.Errorf("got %d results for %d findings: nothing may be filtered", len(results), len(findings))
	}
}

// TestScanNamesTheImagesItCouldNotScan is behaviour 4: no result is not a clean
// image, and the file says so where a reader of the file will see it.
func TestScanNamesTheImagesItCouldNotScan(t *testing.T) {
	partial := scanTestLog(t, []ImageScan{{Image: scanTestImage, Missing: []MissingScanner{{Scanner: "Trivy", Reason: "cache locked"}}}}, nil)
	partialRun := partial["runs"].([]any)[0].(map[string]any)
	partialInvocation := partialRun["invocations"].([]any)[0].(map[string]any)
	if partialInvocation["executionSuccessful"] != false {
		t.Error("a partly scanned image leaves executionSuccessful true")
	}
	notes := partialInvocation["toolExecutionNotifications"].([]any)
	if len(notes) != 1 || !strings.Contains(notes[0].(map[string]any)["message"].(map[string]any)["text"].(string), "Trivy did not answer (cache locked)") {
		t.Errorf("notifications = %v, want one naming Trivy and its reason", notes)
	}

	log := scanTestLog(t, nil, []ScanGap{{Image: "dhi.io/missing:1@sha256:abc", Reason: "Trivy: exit 1"}})

	run := log["runs"].([]any)[0].(map[string]any)
	invocation := run["invocations"].([]any)[0].(map[string]any)
	if invocation["executionSuccessful"] != false {
		t.Errorf("a scan that missed an image reports success: %v", invocation)
	}
	gapNotes := invocation["toolExecutionNotifications"].([]any)
	if len(gapNotes) != 1 {
		t.Fatalf("got %d notifications, want 1", len(gapNotes))
	}
	text := gapNotes[0].(map[string]any)["message"].(map[string]any)["text"].(string)
	if !strings.Contains(text, "dhi.io/missing:1@sha256:abc") || !strings.Contains(text, "Trivy: exit 1") {
		t.Errorf("the notification names neither the image nor the reason: %s", text)
	}
}

// TestScanOfACleanCatalogueIsAnEmptyArray: code scanning and every other reader
// refuse a run whose results is null.
func TestScanOfACleanCatalogueIsAnEmptyArray(t *testing.T) {
	log := scanTestLog(t, []ImageScan{{Image: scanTestImage}}, nil)
	if results := scanTestResults(t, log); len(results) != 0 {
		t.Errorf("got %d results for an image with no finding", len(results))
	}

	run := log["runs"].([]any)[0].(map[string]any)
	if run["invocations"].([]any)[0].(map[string]any)["executionSuccessful"] != true {
		t.Errorf("a complete scan does not report success")
	}
}

// TestScanFingerprintIdentifiesTheFinding: the same finding on the same
// image keeps its identity from one run to the next, and a different image or
// package does not share it.
func TestScanFingerprintIdentifiesTheFinding(t *testing.T) {
	finding := Finding{ID: "CVE-2026-1", Severity: "HIGH", Package: "libssl3", Scanner: "Trivy"}

	first := ScanAlerts([]ImageScan{{Image: scanTestImage, Findings: []Finding{finding}}})
	again := ScanAlerts([]ImageScan{{Image: scanTestImage, Findings: []Finding{finding}}})
	other := ScanAlerts([]ImageScan{{Image: "dhi.io/other:1@sha256:abc", Findings: []Finding{finding}}})

	if first[0].Fingerprint != again[0].Fingerprint {
		t.Error("the fingerprint of one finding changes between runs")
	}
	if first[0].Fingerprint == other[0].Fingerprint {
		t.Error("the same CVE on two images shares a fingerprint")
	}
}
