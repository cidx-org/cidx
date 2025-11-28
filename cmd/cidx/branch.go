package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/branch"
	"github.com/cidx-org/cidx/pkg/config"
	"github.com/urfave/cli/v2"
)

func branchCommand() *cli.Command {
	return &cli.Command{
		Name:  "branch",
		Usage: "Manage Git branches",
		Subcommands: []*cli.Command{
			branchListCommand(),
		},
	}
}

func branchListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List branches with status information",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Include remote-only branches",
			},
			&cli.BoolFlag{
				Name:  "mine",
				Usage: "Only show branches by current user",
			},
			&cli.BoolFlag{
				Name:  "stale",
				Usage: "Only show stale branches",
			},
			&cli.BoolFlag{
				Name:  "merged",
				Usage: "Only show merged branches",
			},
			&cli.BoolFlag{
				Name:  "orphan",
				Usage: "Only show orphan branches (PR closed without merge)",
			},
			&cli.StringFlag{
				Name:  "author",
				Usage: "Filter by author email/name",
			},
			&cli.IntFlag{
				Name:  "stale-days",
				Usage: "Days threshold for stale branches",
				Value: 30,
			},
			&cli.BoolFlag{
				Name:  "json",
				Usage: "Output as JSON",
			},
		},
		Action: func(c *cli.Context) error {
			// Load config for branch settings
			cfg, _ := config.Load("cidx.toml")

			// Build branch config from loaded config or defaults
			branchCfg := branch.Config{
				StaleDays:     c.Int("stale-days"),
				Protected:     []string{"main", "master", "develop"},
			}

			// Override with config file if present
			if cfg != nil {
				if cfg.Branch.StaleDays > 0 {
					branchCfg.StaleDays = cfg.Branch.StaleDays
				}
				if len(cfg.Branch.Protected) > 0 {
					branchCfg.Protected = cfg.Branch.Protected
				}
				branchCfg.NamingPattern = cfg.Branch.NamingPattern
				branchCfg.AutoCleanup = cfg.Branch.AutoCleanup
			}

			// Create manager
			manager := branch.NewManager(branchCfg)

			// Build list options
			opts := branch.ListOptions{
				All:       c.Bool("all"),
				Mine:      c.Bool("mine"),
				Stale:     c.Bool("stale"),
				Merged:    c.Bool("merged"),
				Orphan:    c.Bool("orphan"),
				Author:    c.String("author"),
				StaleDays: branchCfg.StaleDays,
				JSON:      c.Bool("json"),
			}

			// List branches
			result, err := manager.List(opts)
			if err != nil {
				return fmt.Errorf("failed to list branches: %w", err)
			}

			// Format and print output
			output := branch.FormatList(result, opts.JSON)
			fmt.Print(output)

			return nil
		},
	}
}
