package commands

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
)

// recordingExecutor stands in for the container runtime: it records what it was
// asked to run and fails the containers whose name it was told to.
type recordingExecutor struct {
	ran  []*config.ContainerConfig
	fail map[string]bool
}

func (r *recordingExecutor) Run(_ context.Context, c *config.ContainerConfig) error {
	r.ran = append(r.ran, c)
	if r.fail[c.Name] {
		return errors.New("registry refused the credentials")
	}
	return nil
}

func (r *recordingExecutor) Available() bool { return true }
func (r *recordingExecutor) Name() string    { return "recording" }
func (r *recordingExecutor) Close() error    { return nil }

const scanTestImage = "dhi.io/golang:1.26.5-alpine-dev@sha256:0be40e8408fe1fa9fcfce328f13e76b0a39bc0268c564a1da50e90eb699a9709"

const scanTestTrivy = `{"Results":[{"Type":"alpine","Vulnerabilities":[
  {"VulnerabilityID":"CVE-2026-1","PkgName":"libssl3","Severity":"HIGH","FixedVersion":"3.5.1"},
  {"VulnerabilityID":"CVE-2026-9","PkgName":"zlib","Severity":"LOW","FixedVersion":"1.3"}
]}]}`

// TestScanImageRunsBothScannersWithoutAnIgnoreFile is behaviour 3: an acceptance
// removes nothing from the report because nothing is told about it.
func TestScanImageRunsBothScannersWithoutAnIgnoreFile(t *testing.T) {
	exec := &recordingExecutor{}
	rig := scanRig{exec: exec, workspace: t.TempDir(), hostCache: "/host/cache", dockerConfig: "/home/me/.docker", timeout: "5m"}

	if failed := rig.scanImage(context.Background(), 0, scanTestImage, t.TempDir()); len(failed) != 0 {
		t.Fatalf("unexpected failures: %v", failed)
	}
	if len(exec.ran) != 2 {
		t.Fatalf("ran %d containers, want one Trivy and one Grype", len(exec.ran))
	}

	for _, c := range exec.ran {
		joined := c.Command + " " + strings.Join(c.Volumes, " ")
		for _, forbidden := range []string{"ignore", ".trivyignore", ".grype.yaml", "known-vulnerabilities"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("%s is handed %q: the scan would filter what the acceptances cover", c.Name, forbidden)
			}
		}
		if !strings.Contains(c.Command, scanTestImage) {
			t.Errorf("%s does not scan the pinned reference: %s", c.Name, c.Command)
		}
		if !c.Ephemeral {
			t.Errorf("%s may share a container with another scan", c.Name)
		}
		if c.Env["DOCKER_CONFIG"] != "/docker-config" {
			t.Errorf("%s does not read the registry credentials: %v", c.Name, c.Env)
		}
	}
	if exec.ran[0].Image != scanTrivyImage || exec.ran[1].Image != scanGrypeImage {
		t.Errorf("scanners = %s, %s", exec.ran[0].Image, exec.ran[1].Image)
	}
	if !strings.Contains(exec.ran[0].Command, "HIGH,CRITICAL") {
		t.Errorf("Trivy is not asked for the severity band the policy acts on: %s", exec.ran[0].Command)
	}
}

// TestScannersWriteOnlyInsideTheMountedCache: whatever user the scanners run as,
// HOME, the cache and the temporary directory are all inside the directory
// mounted from .cidx/scan, the one place that user can write.
func TestScannersWriteOnlyInsideTheMountedCache(t *testing.T) {
	exec := &recordingExecutor{}
	rig := scanRig{exec: exec, cache: t.TempDir(), workspace: t.TempDir(), hostCache: "/host/cache", timeout: "5m"}
	if err := rig.refreshDatabases(context.Background()); err != nil {
		t.Fatal(err)
	}
	rig.scanImage(context.Background(), 0, scanTestImage, t.TempDir())

	want := map[string]map[string]string{
		"trivy": {"HOME": trivyMount, "XDG_CACHE_HOME": trivyMount, "TRIVY_CACHE_DIR": trivyMount, "TMPDIR": trivyMount + "/tmp"},
		"grype": {"HOME": grypeMount, "XDG_CACHE_HOME": grypeMount, "GRYPE_DB_CACHE_DIR": grypeMount + "/db", "TMPDIR": grypeMount + "/tmp"},
	}
	mount := map[string]string{"trivy": "/host/cache/trivy:" + trivyMount, "grype": "/host/cache/grype:" + grypeMount}

	if len(exec.ran) != 4 {
		t.Fatalf("ran %d containers, want two refreshes and two scans", len(exec.ran))
	}
	for _, c := range exec.ran {
		scanner := "trivy"
		if strings.Contains(c.Name, "grype") {
			scanner = "grype"
		}
		for k, v := range want[scanner] {
			if c.Env[k] != v {
				t.Errorf("%s: %s = %q, want %q", c.Name, k, c.Env[k], v)
			}
		}
		found := false
		for _, v := range c.Volumes {
			found = found || v == mount[scanner]
		}
		if !found {
			t.Errorf("%s does not mount the cache: %v", c.Name, c.Volumes)
		}
	}
	if exec.ran[3].Env["GRYPE_DB_AUTO_UPDATE"] != "false" {
		t.Errorf("Grype scan may update its database: %v", exec.ran[3].Env)
	}
}

// TestAColdCacheWithoutADatabaseStopsTheScan: a failed download with nothing
// cached ends the run with its cause instead of failing every scan.
func TestAColdCacheWithoutADatabaseStopsTheScan(t *testing.T) {
	exec := &recordingExecutor{fail: map[string]bool{"catalogue-scan-trivy-db": true}}
	rig := scanRig{exec: exec, cache: t.TempDir(), workspace: t.TempDir(), hostCache: "/host/cache", timeout: "5m"}

	err := rig.refreshDatabases(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Trivy") || !strings.Contains(err.Error(), "registry refused") {
		t.Fatalf("err = %v, want the Trivy download failure and its cause", err)
	}

	// With a database from an earlier run the refresh failure is only a warning.
	db := filepath.Join(rig.cache, "trivy", "db")
	if err := os.MkdirAll(db, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(db, "trivy.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rig.refreshDatabases(context.Background()); err != nil {
		t.Fatalf("a cached database should be used: %v", err)
	}
}

// TestAScanThatCannotStartLeavesAReadableReport: when no executor can be
// selected, security.sarif says nothing was scanned and why, instead of missing.
func TestAScanThatCannotStartLeavesAReadableReport(t *testing.T) {
	out := filepath.Join(t.TempDir(), "security.sarif")
	cause := errors.New("executor selection failed: Docker socket access was denied")
	if err := writeUnscannedReport(out, cause); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Docker socket access was denied", "not scanned", `"executionSuccessful": false`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("report lacks %q:\n%s", want, data)
		}
	}
}

// TestScanImageNamesTheScannerThatFailed is behaviour 4: the failure is kept,
// attributed, and the result file it may have left is not read as an answer.
func TestScanImageNamesTheScannerThatFailed(t *testing.T) {
	results := t.TempDir()
	stale := filepath.Join(results, scanFileName("grype", scanTestImage))
	if err := os.WriteFile(stale, []byte(`{"matches":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	exec := &recordingExecutor{fail: map[string]bool{"catalogue-scan-grype-01": true}}
	rig := scanRig{exec: exec, workspace: t.TempDir(), hostCache: "/host/cache", timeout: "5m"}

	failed := rig.scanImage(context.Background(), 0, scanTestImage, results)
	if len(failed) != 1 || !strings.HasPrefix(failed[0], "Grype: ") {
		t.Fatalf("failures = %v, want one attributed to Grype", failed)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("the result of an earlier run is left to answer for a scan that failed")
	}
}

func TestCollectScansSeparatesScannedPartialAndUnscanned(t *testing.T) {
	results := t.TempDir()
	missing := "dhi.io/missing:1@sha256:abc"
	if err := os.WriteFile(filepath.Join(results, scanFileName("trivy", scanTestImage)), []byte(scanTestTrivy), 0o644); err != nil {
		t.Fatal(err)
	}

	imagePresets := map[string][]string{scanTestImage: {"go-build"}, missing: {"other"}}
	scans, gaps, partial := collectScans(imagePresets, []string{missing, scanTestImage}, results,
		map[string][]string{missing: {"Trivy: exit 1", "Grype: exit 1"}})

	if len(scans) != 1 || scans[0].Image != scanTestImage {
		t.Fatalf("scans = %v", scans)
	}
	if len(scans[0].Findings) != 1 || scans[0].Findings[0].FixedIn != "3.5.1" {
		t.Errorf("findings = %v, want the HIGH one with its fixed version", scans[0].Findings)
	}
	if len(partial) != 1 || !strings.Contains(partial[0], "Trivy") {
		t.Errorf("partial = %v, want the image only Trivy answered for", partial)
	}
	if len(gaps) != 1 || gaps[0].Image != missing {
		t.Fatalf("gaps = %v", gaps)
	}
	if !strings.Contains(gaps[0].Reason, "Trivy: exit 1") || !strings.Contains(gaps[0].Reason, "Grype: exit 1") {
		t.Errorf("the reason loses what the scanners said: %q", gaps[0].Reason)
	}
}

// TestScanScannersAreTheAuditsScanners: the catalogue scan and the audit scan
// the same images with the same tools, so a Renovate bump of one has to reach
// the other.
func TestScanScannersAreTheAuditsScanners(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "security-audit.yml"))
	if err != nil {
		t.Fatalf("read the audit workflow: %v", err)
	}
	for name, image := range map[string]string{"TRIVY_IMAGE": scanTrivyImage, "GRYPE_IMAGE": scanGrypeImage} {
		if !strings.Contains(string(workflow), name+": \""+image+"\"") {
			t.Errorf("security-audit.yml pins %s to something other than %s", name, image)
		}
	}
}

func TestScanIsPartOfTheSecurityNamespace(t *testing.T) {
	app := NewApp()
	var found bool
	for _, command := range app.Commands {
		if command.Name != "security" {
			continue
		}
		for _, sub := range command.Subcommands {
			found = found || sub.Name == "scan"
		}
	}
	if !found {
		t.Error("`cidx security scan` is not in the command tree")
	}
}
