package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/validator"
	"github.com/urfave/cli/v2"
)

// runResolveConfigPath drives the global --config flag through the real command
// tree, so the test also covers the flag actually reaching the subcommand.
func runResolveConfigPath(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var (
		got    string
		gotErr error
	)
	app := &cli.App{
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}},
		},
		Commands: []*cli.Command{{
			Name: "probe",
			Action: func(c *cli.Context) error {
				got, gotErr = resolveConfigPath(c)
				return nil
			},
		}},
	}

	if err := app.Run(append([]string{"cidx"}, append(args, "probe")...)); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	return got, gotErr
}

// TestResolveConfigPath_HonoursConfigFlag covers #230: `check workflow`
// hardcoded cidx.toml, so a project whose config is named differently could
// not check its workflows even though `run` accepted --config.
func TestResolveConfigPath_HonoursConfigFlag(t *testing.T) {
	got, err := runResolveConfigPath(t, "--config", "ci/pipeline.toml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ci/pipeline.toml" {
		t.Errorf("expected the --config path, got %q", got)
	}

	// Same through the short alias.
	if got, err := runResolveConfigPath(t, "-c", "other.toml"); err != nil || got != "other.toml" {
		t.Errorf("expected the -c path, got %q (err: %v)", got, err)
	}
}

// TestResolveConfigPath_FallsBackToDiscovery keeps the flagless behaviour:
// auto-detect the conventional file.
func TestResolveConfigPath_FallsBackToDiscovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cidx.toml"), []byte("[security]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	got, err := runResolveConfigPath(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "cidx.toml" {
		t.Errorf("expected the auto-detected config, got %q", got)
	}
}

// TestResolveConfigPath_NoConfigReportsError: without a flag and without a
// conventional file, the caller gets an error instead of a silent cidx.toml.
func TestResolveConfigPath_NoConfigReportsError(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := runResolveConfigPath(t); err == nil {
		t.Error("expected an error when no config exists")
	}
}

// projectWithConfig writes a minimal config under a name that is deliberately
// not cidx.toml, and makes it the working directory. A command that ignores
// --config finds nothing there and fails.
func projectWithConfig(t *testing.T, name string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	config := "[security]\ncontainers = [\"trivy\"]\n\n[pipelines.ci]\nphases = [\"security\"]\n"
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

// TestGenerateHonoursConfigFlag covers the `generate` half of #230: both
// subcommands hardcoded cidx.toml, so a project whose config is named
// differently could not generate its workflow at all.
func TestGenerateHonoursConfigFlag(t *testing.T) {
	for _, platform := range []string{"github", "gitlab"} {
		t.Run(platform, func(t *testing.T) {
			dir := projectWithConfig(t, "ci/pipeline.toml")
			out := filepath.Join(dir, "generated.yml")

			if err := newApp().Run([]string{"cidx", "--config", "ci/pipeline.toml", "generate", platform, "-o", out}); err != nil {
				t.Fatalf("generate %s: %v", platform, err)
			}
			if _, err := os.Stat(out); err != nil {
				t.Fatalf("expected the workflow to be written: %v", err)
			}
		})
	}
}

// TestGenerateRegenerationHintNamesTheOutputPath covers #229 on the GitLab
// side: the header always announced `-o .gitlab-ci.yml`, so following it from a
// file generated elsewhere silently wrote a second configuration.
func TestGenerateRegenerationHintNamesTheOutputPath(t *testing.T) {
	for _, tc := range []struct{ platform, out, defaultPath string }{
		{"github", "ci/custom.yml", ".github/workflows/cidx.yml"},
		{"gitlab", "ci/custom.yml", ".gitlab-ci.yml"},
	} {
		t.Run(tc.platform, func(t *testing.T) {
			dir := projectWithConfig(t, "cidx.toml")
			out := filepath.Join(dir, tc.out)

			if err := newApp().Run([]string{"cidx", "generate", tc.platform, "-o", out}); err != nil {
				t.Fatalf("generate %s: %v", tc.platform, err)
			}

			generated, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			want := "# Regenerate with: cidx generate " + tc.platform + " -o " + out
			if !strings.Contains(string(generated), want) {
				t.Errorf("expected the header to name %q", out)
			}
			if strings.Contains(string(generated), "-o "+tc.defaultPath) {
				t.Errorf("the header still points at the default path %q", tc.defaultPath)
			}
		})
	}
}

// TestCheckWorkflow_RealCIWorkflowKeepsItsPhases is the standing guard for the
// second half of #233: `check workflow` extracted phases by matching the
// substring "cidx run ", so `./bin/cidx --verbose run test` — the form ci.yml
// has used since #271 — lost the test phase entirely. The command then reported
// it missing while `check drift` reported it in sync, on the same file. It
// reads this repository's own files only: no network, no workflow run.
func TestCheckWorkflow_RealCIWorkflowKeepsItsPhases(t *testing.T) {
	const ciWorkflow = repoWorkflowDir + "/ci.yml"

	workflow, err := validator.ParseWorkflow(newApp(), ciWorkflow)
	if err != nil {
		t.Fatalf("ParseWorkflow: %v", err)
	}
	if !slices.Contains(workflow.Phases, "test") {
		t.Errorf("the test phase is run by %s but was extracted as %v", ciWorkflow, workflow.Phases)
	}

	// And the whole comparison agrees: this is `cidx check workflow ci`.
	cfg, err := config.Load("../../cidx.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	result, err := validator.ValidateWorkflow(newApp(), cfg, "ci", ciWorkflow)
	if err != nil {
		t.Fatalf("ValidateWorkflow: %v", err)
	}
	if !result.Success {
		t.Errorf("check workflow ci is out of sync: cidx.toml %v, ci.yml %v (missing in workflow: %v, missing in config: %v, order differs: %v)",
			result.LocalOrder, result.GitHubOrder, result.MissingInGH, result.MissingInLocal, result.OrderMismatch)
	}
}

// TestCheckDriftHonoursConfigFlag covers the `check drift` half of #230: it
// loaded cidx.toml outright, so a project whose config is named differently
// could not compare it against its workflow.
func TestCheckDriftHonoursConfigFlag(t *testing.T) {
	dir := projectWithConfig(t, "ci/pipeline.toml")
	workflow := filepath.Join(dir, "ci.yml")

	if err := newApp().Run([]string{"cidx", "--config", "ci/pipeline.toml", "generate", "github", "-o", workflow}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The workflow was generated from that very config, so there is no drift.
	// Before the fix this failed on "failed to load cidx.toml" instead.
	if err := newApp().Run([]string{"cidx", "--config", "ci/pipeline.toml", "check", "drift", "--file", workflow}); err != nil {
		t.Fatalf("check drift: %v", err)
	}
}

// TestCheckWorkflowSummaryStatesWhatWasChecked covers #318: one summary line
// served both paths, so `cidx check workflow ci` — one pipeline compared —
// signed off with "All workflows are in sync with pipelines" and read as a clean
// bill for the whole repository. Both modes are pinned here, in sync and not,
// because the defect was that they were indistinguishable.
func TestCheckWorkflowSummaryStatesWhatWasChecked(t *testing.T) {
	for _, tc := range []struct {
		name             string
		pipeline         string
		checked          int
		inSync           bool
		wants, wantsNone []string
	}{
		{
			name: "one pipeline, in sync", pipeline: "ci", checked: 1, inSync: true,
			wants: []string{"✅", "Pipeline 'ci'", "in sync"},
			// The whole point: a targeted check may not speak for the sweep.
			wantsNone: []string{"All workflows", "All 1"},
		},
		{
			name: "one pipeline, out of sync", pipeline: "ci", checked: 1, inSync: false,
			wants:     []string{"⚠️", "Pipeline 'ci'", "differences"},
			wantsNone: []string{"Some workflows"},
		},
		{
			name: "every pipeline, in sync", pipeline: "", checked: 3, inSync: true,
			wants:     []string{"✅", "All 3 workflow(s)", "in sync"},
			wantsNone: []string{"Pipeline '"},
		},
		{
			name: "every pipeline, out of sync", pipeline: "", checked: 3, inSync: false,
			wants:     []string{"⚠️", "3 checked workflow(s)", "differences"},
			wantsNone: []string{"Pipeline '"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkWorkflowSummary(tc.pipeline, tc.checked, tc.inSync)
			for _, want := range tc.wants {
				if !strings.Contains(got, want) {
					t.Errorf("summary %q does not mention %q", got, want)
				}
			}
			for _, unwanted := range tc.wantsNone {
				if strings.Contains(got, unwanted) {
					t.Errorf("summary %q overstates its scope with %q", got, unwanted)
				}
			}
		})
	}
}

// TestCheckWorkflowNamesThePipelineItChecked drives the real command tree, so
// the wording above is the wording a user reads. `cidx check workflow ci` on
// this very repository is the invocation of #318.
func TestCheckWorkflowNamesThePipelineItChecked(t *testing.T) {
	out := captureStdout(t, func() {
		if err := newApp().Run([]string{
			"cidx", "--config", "../../cidx.toml",
			"check", "workflow", "--workflow-dir", repoWorkflowDir, "ci",
		}); err != nil {
			t.Fatalf("check workflow ci: %v", err)
		}
	})

	if !strings.Contains(out, "Pipeline 'ci' is in sync with its workflow") {
		t.Errorf("the summary does not name the pipeline it checked:\n%s", out)
	}
	if strings.Contains(out, "All workflows are in sync") {
		t.Errorf("checking one pipeline still claims to have checked all of them:\n%s", out)
	}
}
