package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/presets"
)

const limitCause = "Trivy: TOOMANYREQUESTS: You have reached your unauthenticated pull rate limit"

const failedImage = "dhi.io/missing:1@sha256:" + zeroDigest

// TestAnImageThatFailedAndACompleteOneGiveTheSheetOfTheSecondAndTheIndexOfBoth
// is behaviours 1, 2 and 4: the baseline file is left alone and the run fails,
// but the complete image has its sheet and the index names the missing one with
// its cause.
func TestAnImageThatFailedAndACompleteOneGiveTheSheetOfTheSecondAndTheIndexOfBoth(t *testing.T) {
	results := t.TempDir()
	writeTriageFixture(t, results, triageImage)
	imagePresets := map[string][]string{triageImage: {"trivy"}, failedImage: {"other"}}
	unscanned := map[string]string{failedImage: limitCause}
	if err := writeUnscanned(results, unscanned); err != nil {
		t.Fatal(err)
	}

	accepted := triageFixtureAccepted()
	carried, _ := carriedFindings(imagePresets, results, accepted)

	read, err := readUnscanned(results, imagePresets)
	if err != nil || len(read) != 1 || read[failedImage] != limitCause {
		t.Fatalf("readUnscanned = %v, %v", read, err)
	}

	dir := filepath.Join(t.TempDir(), "triage")
	output := filepath.Join(t.TempDir(), "SECURITY-BASELINE.md")
	err = incompleteBaseline(dir, output, carried, accepted, read)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("err = %v, want the run to fail as incomplete", err)
	}

	if _, err := os.Stat(output); err == nil {
		t.Error("the baseline was written on partial results")
	}
	if _, err := os.Stat(filepath.Join(dir, triageSheetFile(triageImage))); err != nil {
		t.Errorf("the complete image has no sheet: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, triageSheetFile(failedImage))); err == nil {
		t.Error("the unscanned image has a sheet")
	}

	index, err := os.ReadFile(filepath.Join(dir, triageIndexFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(index), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("index = %q, want a header and one line per image", lines)
	}
	if !strings.HasPrefix(lines[1], failedImage+"\t") || !strings.HasSuffix(lines[1], "\t"+limitCause) {
		t.Errorf("the unscanned image is not listed with its cause: %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], triageImage+"\t4\t") {
		t.Errorf("the scanned image is not summarised: %q", lines[2])
	}
}

// TestAnImageOnlyOneScannerAnsweredForGetsNoSheet: its findings are half of what
// there is, so it is listed as unscanned with the scanner that is missing.
func TestAnImageOnlyOneScannerAnsweredForGetsNoSheet(t *testing.T) {
	scans := []presets.ImageScan{{
		Image:   scanTestImage,
		Missing: []presets.MissingScanner{{Scanner: "Grype", Reason: "exit 1"}},
	}, {Image: triageImage}}
	got := unscannedImages(scans, []presets.ScanGap{{Image: failedImage, Reason: "both failed"}})

	if len(got) != 2 || !strings.Contains(got[scanTestImage], "Grype did not answer: exit 1") || got[failedImage] != "both failed" {
		t.Errorf("unscannedImages = %v", got)
	}
}

// TestACompleteScanLeavesNoUnscannedFile: the file exists only to say what is
// missing, and the next complete scan removes it.
func TestACompleteScanLeavesNoUnscannedFile(t *testing.T) {
	dir := t.TempDir()
	if err := writeUnscanned(dir, map[string]string{failedImage: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := writeUnscanned(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, unscannedFile)); err == nil {
		t.Error("the file of an earlier incomplete scan survived a complete one")
	}
}

func cacheWithDatabases(t *testing.T, built time.Time) string {
	t.Helper()
	cache := t.TempDir()
	for _, f := range []string{filepath.Join("trivy", "db", "trivy.db"), filepath.Join("grype", "db", "vulnerability.db")} {
		path := filepath.Join(cache, f)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("db"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, built, built); err != nil {
			t.Fatal(err)
		}
	}
	return cache
}

func leaveResults(t *testing.T, results string, at time.Time) {
	t.Helper()
	for _, scanner := range []string{"trivy", "grype"} {
		path := filepath.Join(results, scanFileName(scanner, scanTestImage))
		if err := os.WriteFile(path, []byte(`{"matches":[]}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
}

// TestAResultCachedAtTheSameDigestIsNotPulledAgain is behaviour 3: the results of
// a digest-pinned image newer than the advisory databases answer for it, and no
// scanner container runs.
func TestAResultCachedAtTheSameDigestIsNotPulledAgain(t *testing.T) {
	now := time.Now()
	cache := cacheWithDatabases(t, now.Add(-2*time.Hour))
	results := t.TempDir()
	leaveResults(t, results, now.Add(-time.Hour))

	exec := &recordingExecutor{}
	rig := scanRig{exec: exec, cache: cache, workspace: t.TempDir(), hostCache: "/host/cache", timeout: "5m"}
	if failed := rig.scanImage(context.Background(), 0, scanTestImage, results); len(failed) != 0 {
		t.Fatalf("failures = %v", failed)
	}
	if len(exec.ran) != 0 {
		t.Errorf("ran %d container(s) for an image whose results are current", len(exec.ran))
	}
}

// TestAResultOlderThanTheDatabaseIsScannedAgain: the same image can carry new
// findings once the advisories change, so only a result built on today's
// database is kept.
func TestAResultOlderThanTheDatabaseIsScannedAgain(t *testing.T) {
	now := time.Now()
	cache := cacheWithDatabases(t, now.Add(-time.Hour))
	results := t.TempDir()
	leaveResults(t, results, now.Add(-2*time.Hour))

	exec := &recordingExecutor{}
	rig := scanRig{exec: exec, cache: cache, workspace: t.TempDir(), hostCache: "/host/cache", timeout: "5m"}
	rig.scanImage(context.Background(), 0, scanTestImage, results)
	if len(exec.ran) != 2 {
		t.Errorf("ran %d container(s), want both scanners", len(exec.ran))
	}
}

// TestAnImageWithoutADigestIsNeverServedFromTheCache: a tag moves.
func TestAnImageWithoutADigestIsNeverServedFromTheCache(t *testing.T) {
	now := time.Now()
	cache := cacheWithDatabases(t, now.Add(-2*time.Hour))
	results := t.TempDir()
	image := "dhi.io/golang:1.26"
	for _, scanner := range []string{"trivy", "grype"} {
		if err := os.WriteFile(filepath.Join(results, scanFileName(scanner, image)), []byte(`{}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rig := scanRig{cache: cache}
	if rig.cachedResult("trivy", image, results) || rig.cachedResult("grype", image, results) {
		t.Error("a result for a moving tag was reused")
	}
}
