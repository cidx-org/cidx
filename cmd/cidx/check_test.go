package main

import (
	"os"
	"path/filepath"
	"testing"

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
