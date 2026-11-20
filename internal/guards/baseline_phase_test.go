package guards

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/presets"
)

// TestTheBaselinePhaseScansThenRegenerates pins what `cidx run baseline` is for:
// the one command that refreshes SECURITY-BASELINE.md from a scan of the
// catalogue, run by a tool in a working directory.
//
// The baseline is generated only when the scan succeeded, so an incomplete scan
// never reaches the generation: no false baseline is written. It still leaves the
// triage sheets of its complete images (`--incomplete`). The baseline reads the raw results of that very scan, shares its
// cache, runs the repository's own code, and fits in 30 minutes. No pipeline
// lists the phase.
func TestTheBaselinePhaseScansThenRegenerates(t *testing.T) {
	cfg, err := config.Load(filepath.Join(projectRoot, "cidx.toml"))
	if err != nil {
		t.Fatalf("failed to load cidx.toml: %v", err)
	}

	phase, ok := cfg.Phases["baseline"]
	if !ok {
		t.Fatal("cidx.toml declares no [baseline] phase, so `cidx run baseline` is an unknown target")
	}
	if !slices.Equal(phase.Containers, []string{"catalogue-baseline"}) {
		t.Errorf("baseline runs %v, want [catalogue-baseline]", phase.Containers)
	}

	for name, pipeline := range cfg.Pipelines {
		if slices.Contains(pipeline.Phases, "baseline") {
			t.Errorf("pipeline %q lists baseline: it is run on demand, never from CI", name)
		}
	}

	preset, overrides, err := presets.Lookup("catalogue-baseline", cfg.Overrides["catalogue-baseline"])
	if err != nil {
		t.Fatalf("the baseline container does not resolve: %v", err)
	}
	merged := preset.MergeWith(overrides)

	scan := strings.Index(merged.Command, "go run ./cmd/cidx security scan")
	baseline := strings.Index(merged.Command, "go run ./cmd/cidx security baseline")
	if scan < 0 || baseline < 0 || scan > baseline {
		t.Fatalf("the baseline phase runs %q, want the repository's `security scan` and then `security baseline`", merged.Command)
	}
	// The baseline file is written only after a scan that succeeded; a failed one
	// still gets the triage sheets of its complete images, through --incomplete,
	// and the phase fails.
	if !strings.Contains(merged.Command, "if go run ./cmd/cidx security scan; then") {
		t.Errorf("the baseline is not conditioned on the scan having succeeded: %q", merged.Command)
	}
	if !strings.Contains(merged.Command, "security baseline --incomplete") || !strings.Contains(merged.Command, "exit 1") {
		t.Errorf("a failed scan does not write the triage of its complete images and fail: %q", merged.Command)
	}
	if !strings.Contains(merged.Command, "--results .cidx/scan/results") {
		t.Errorf("the baseline does not read the results of the scan (.cidx/scan/results): %q", merged.Command)
	}

	if got, _ := cfg.Overrides["catalogue-baseline"]["phase"].(string); got != "baseline" {
		t.Errorf("the container's phase is %q, want baseline", got)
	}
	if got, _ := cfg.Overrides["catalogue-baseline"]["timeout"].(string); got != "30m" {
		t.Errorf("the baseline timeout is %q, want 30m", got)
	}
	if !slices.Contains(merged.Volumes, "/var/run/docker.sock:/var/run/docker.sock") {
		t.Errorf("the baseline cannot start the scanners: volumes = %v", merged.Volumes)
	}
	// The cache is a host path that outlives the checkout (#cache-de-scan-par-checkout),
	// the same for the scan and the baseline; the results stay in the checkout.
	if !strings.HasSuffix(merged.Env["CIDX_SCAN_HOST_CACHE"], "/.cache/cidx/scan") {
		t.Errorf("the baseline does not use the shared scan cache (~/.cache/cidx/scan): CIDX_SCAN_HOST_CACHE = %q", merged.Env["CIDX_SCAN_HOST_CACHE"])
	}
	if strings.Contains(merged.Env["CIDX_SCAN_HOST_CACHE"], "/.cidx/") {
		t.Errorf("the scan cache lives in the checkout: %q", merged.Env["CIDX_SCAN_HOST_CACHE"])
	}
	if merged.Env["CIDX_SCAN_RESULTS"] != ".cidx/scan/results" {
		t.Errorf("the results are not written to the checkout's .cidx/scan/results: CIDX_SCAN_RESULTS = %q", merged.Env["CIDX_SCAN_RESULTS"])
	}
	if !slices.Contains(merged.Volumes, "${HOME}/.cache/cidx/scan:"+merged.Env["CIDX_SCAN_CACHE"]) {
		t.Errorf("the shared cache is not mounted where CIDX_SCAN_CACHE says (%q): volumes = %v", merged.Env["CIDX_SCAN_CACHE"], merged.Volumes)
	}
	for _, cache := range []string{":/tmp/go-mod-cache", ":/tmp/go-build-cache"} {
		if !slices.ContainsFunc(merged.Volumes, func(v string) bool {
			return strings.HasPrefix(v, "${WORKSPACE}/.cidx/") && strings.HasSuffix(v, cache)
		}) {
			t.Errorf("no volume keeps %s between runs", cache[1:])
		}
	}
}
