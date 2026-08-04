package commands

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
				Name:      "download",
				Usage:     "Download a run's artifacts into a flat directory",
				ArgsUsage: "[options] [name-pattern...]",
				Description: `Downloads the artifacts a workflow run produced.

Every artifact of the run by default; name patterns select a subset, and globs
are honoured ('trivy-*'). The files land flat in one directory -- no
subdirectory per artifact -- because that is what the commands that read them
expect: 'cidx security vuln prune --results DIR' and 'cidx security baseline
--results DIR' join DIR with a file name.

--output defaults to 'scan-results', the same default those commands read, so
the pair works with no path repeated:

  cidx repo artifact download --run 18234567890
  cidx security vuln prune

--run defaults to the most recent run on the current branch; the identifier is
the 'id' column of 'cidx repo workflow list'. The repository comes from the git
remote of the working directory, so the artifacts are always this repository's.

A file name two artifacts share is not an error: identical content is skipped
and differing content keeps the first copy, naming both artifacts.

Examples:
  cidx repo artifact download --run 18234567890
  cidx repo artifact download --run 18234567890 'trivy-*' 'grype-*'
  cidx repo artifact download --run 18234567890 -o /tmp/audit`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "run",
						Usage: "Run whose artifacts to download (defaults to the latest run on the current branch)",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Directory to write the files into",
						Value:   defaultResultsDir,
					},
				},
				Action: artifactDownloadAction,
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
