package guards

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// TestTheCatalogueScanTargetIsDeclared pins what `cidx run catalogue-scan` is
// for: the target Drassana runs after the tests and reads security.sarif from.
//
// Three things must hold together. The target exists under a name of its own
// and leaves the `security` phase — the repository's own gate, which Drassana
// uses as a test group — exactly as it was. No pipeline lists it, because a scan
// of 21 images is not something every push pays for and its findings are not a
// gate. And the report is never committed.
func TestTheCatalogueScanTargetIsDeclared(t *testing.T) {
	cfg, err := config.Load(filepath.Join(projectRoot, "cidx.toml"))
	if err != nil {
		t.Fatalf("failed to load cidx.toml: %v", err)
	}

	phase, ok := cfg.Phases["catalogue-scan"]
	if !ok {
		t.Fatal("cidx.toml declares no [catalogue-scan] phase, so `cidx run catalogue-scan` is an unknown target")
	}
	if !slices.Equal(phase.Containers, []string{"catalogue-scanner"}) {
		t.Errorf("catalogue-scan runs %v, want [catalogue-scanner]", phase.Containers)
	}

	if got := cfg.Phases["security"].Containers; !slices.Equal(got, []string{"trivy", "gitleaks"}) {
		t.Errorf("the security phase is %v: the catalogue scan must not change it", got)
	}

	for name, pipeline := range cfg.Pipelines {
		if slices.Contains(pipeline.Phases, "catalogue-scan") {
			t.Errorf("pipeline %q lists catalogue-scan: it is run on demand, never from CI", name)
		}
	}

	preset, overrides, err := presets.Lookup("catalogue-scanner", cfg.Overrides["catalogue-scanner"])
	if err != nil {
		t.Fatalf("the scanner container does not resolve: %v", err)
	}
	merged := preset.MergeWith(overrides)

	if !strings.Contains(merged.Command, "security scan") {
		t.Errorf("the scanner runs %q, not `cidx security scan`", merged.Command)
	}
	if got, _ := cfg.Overrides["catalogue-scanner"]["phase"].(string); got != "catalogue-scan" {
		t.Errorf("the scanner's phase is %q, want catalogue-scan", got)
	}
	if !slices.Contains(merged.Volumes, "/var/run/docker.sock:/var/run/docker.sock") {
		t.Errorf("the scanner cannot start the scanners: volumes = %v", merged.Volumes)
	}
	if merged.Env["CIDX_SCAN_HOST_CACHE"] == "" {
		t.Error("the scanner does not say where its cache is on the host, so the scanners it starts would mount a path that does not exist there")
	}
	for _, volume := range merged.Volumes {
		if strings.Contains(volume, "known-vulnerabilities") {
			t.Errorf("volume %q hands the acceptances to the scan, which must report what they accept", volume)
		}
	}

	ignored, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore"))
	if err != nil {
		t.Fatalf("failed to read .gitignore: %v", err)
	}
	if !slices.Contains(strings.Split(string(ignored), "\n"), "security.sarif") {
		t.Error("security.sarif is not ignored: the report would be committed")
	}
}
