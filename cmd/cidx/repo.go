package main

import "github.com/urfave/cli/v2"

func repoCommand() *cli.Command {
	return &cli.Command{
		Name:  "repo",
		Usage: "Repository workflow helpers (pr, branch, artifacts)",
		Subcommands: []*cli.Command{
			prCommand(),
			cpwCommand(),
			branchCommand(),
			workflowCommand(),
			artifactCommand(),
			cleanupCommand(),
		},
	}
}

func artifactCommand() *cli.Command {
	return &cli.Command{
		Name:  "artifact",
		Usage: "GitHub Actions artifact management",
		Subcommands: []*cli.Command{
			{
				Name:    "tui",
				Usage:   "Interactive artifact manager (TUI)",
				Aliases: []string{"ui"},
				Action:  artifactTUIAction,
			},
			{
				Name:  "list",
				Usage: "List all artifacts with storage statistics",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show detailed artifact information",
					},
				},
				Action: artifactListAction,
			},
			{
				Name:   "stats",
				Usage:  "Show artifact storage statistics",
				Action: artifactStatsAction,
			},
			{
				Name:  "cleanup",
				Usage: "Delete artifacts to free storage space",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "all",
						Usage: "Delete all artifacts",
					},
					&cli.BoolFlag{
						Name:  "expired",
						Usage: "Delete only expired artifacts",
					},
					&cli.IntFlag{
						Name:    "older-than",
						Aliases: []string{"d"},
						Usage:   "Delete artifacts older than N days",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be deleted without making changes",
					},
				},
				Action: artifactCleanupAction,
			},
		},
	}
}
