package main

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/executor"
	"github.com/cidx-org/cidx/pkg/pipeline"
	"github.com/urfave/cli/v2"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:      "run",
		Usage:     "Run a phase, tool, or all phases",
		ArgsUsage: "<phase|tool|all>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "dry-run",
				Aliases: []string{"n"},
				Usage:   "Show what would be executed without running",
			},
			&cli.StringFlag{
				Name:    "backend",
				Aliases: []string{"b"},
				Usage:   "Executor backend: auto, docker, native (default: auto)",
				Value:   "auto",
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
			backend := executor.ParseBackendType(c.String("backend"))

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

			// Create executor selector
			selector, err := executor.NewSelector(dryRun, verbose)
			if err != nil {
				return fmt.Errorf("failed to create executor selector: %w", err)
			}
			defer func() {
				if closeErr := selector.Close(); closeErr != nil {
					_, _ = fmt.Fprintf(c.App.ErrWriter, "Warning: failed to close executor: %v\n", closeErr)
				}
			}()

			// Create runner with selector
			runner := pipeline.NewRunnerWithSelector(cfg, selector, backend)

			ctx := context.Background()

			// Run target (phase, tool, or all)
			return runner.Run(ctx, target)
		},
	}
}
