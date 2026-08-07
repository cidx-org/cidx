package commands

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/environment"
	"github.com/cidx-org/cidx/v3/pkg/executor"
	"github.com/cidx-org/cidx/v3/pkg/pipeline"
	"github.com/urfave/cli/v2"
)

// ResolveQuiet reports whether container output is buffered and dropped on
// success. CI runs quiet by default (issue #87), which is right for a lint that
// passes and wrong for a test job: the Test job printed `PHASE: TEST` then
// `go-test completed`, and a green run showing nothing cannot tell you whether
// the suite ran every scenario it has or none of them.
//
// `--verbose` was the only way out, and it buys the output at the price of
// logrus at Debug and the raw JSON of every image pull. `--stream` asks for the
// output and nothing else, so it wins over `--quiet` — one of the two was typed
// to see something, and the flag that was reached for last is the specific one.
//
// Exported because the scenarios describing it resolve against the real
// decision rather than a copy of it (issue #317).
func ResolveQuiet(quiet, stream, verbose, isCI bool) bool {
	if stream {
		return false
	}
	if quiet {
		return true
	}
	return isCI && !verbose
}

func runCommand() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Run a phase, tool, or all phases",
		ArgsUsage: "[flags] <phase|tool|all>",
		Description: `Execute containers by phase name, tool name, or all phases.

Examples:
  cidx run security                    # Run security phase
  cidx run trivy                       # Run single tool
  cidx run --parallel security         # Parallel execution (local only)
  cidx run -p -j 4 security            # Parallel with 4 concurrent
  cidx run --dry-run ci                # Dry-run full pipeline`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "dry-run",
				Aliases: []string{"n"},
				Usage:   "Show what would be executed without running",
			},
			&cli.StringFlag{
				Name:    "backend",
				Aliases: []string{"b"},
				Usage:   "Executor backend: auto, docker, podman (default: auto)",
				Value:   "auto",
			},
			&cli.BoolFlag{
				Name:    "parallel",
				Aliases: []string{"p"},
				Usage:   "Run containers in parallel within each phase (local only)",
			},
			&cli.IntFlag{
				Name:    "concurrency",
				Aliases: []string{"j"},
				Usage:   "Max concurrent containers when --parallel is enabled (default: 2)",
				Value:   2,
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "Suppress output and only show logs on failure",
			},
			&cli.BoolFlag{
				Name:  "stream",
				Usage: "Stream container output live, at the normal log level (CI runs are quiet by default)",
			},
		},
		Action: func(c *cli.Context) error {
			if c.NArg() != 1 {
				return fmt.Errorf("requires exactly one argument: phase, tool, or 'all'")
			}

			target := c.Args().First()
			configPath := c.String("config")
			dryRun := c.Bool("dry-run")
			verbose := c.Bool("verbose")
			quiet := ResolveQuiet(c.Bool("quiet"), c.Bool("stream"), verbose, environment.Detect().IsCI)

			backend := executor.ParseBackendType(c.String("backend"))
			parallel := c.Bool("parallel")
			concurrency := c.Int("concurrency")

			// Load config
			if configPath == "" {
				found, err := config.FindConfig()
				if err != nil {
					return err
				}
				configPath = found
			}

			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Validate required version
			if err := config.CheckVersion(cfg, Version); err != nil {
				return err
			}

			// Create executor selector
			selector, err := executor.NewSelector(dryRun, verbose, quiet)
			if err != nil {
				return fmt.Errorf("failed to create executor selector: %w", err)
			}
			defer func() {
				if closeErr := selector.Close(); closeErr != nil {
					_, _ = fmt.Fprintf(c.App.ErrWriter, "Warning: failed to close executor: %v\n", closeErr)
				}
			}()

			// Create runner options
			opts := pipeline.RunnerOptions{
				Backend:     backend,
				Parallel:    parallel,
				Concurrency: concurrency,
			}

			// Create runner with selector
			runner := pipeline.NewRunnerWithOptions(cfg, selector, opts)

			ctx := context.Background()

			// Run target (phase, tool, or all)
			return runner.Run(ctx, target)
		},
	}
}
