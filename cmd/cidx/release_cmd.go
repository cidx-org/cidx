package main

import "github.com/urfave/cli/v2"

func releaseCommand() *cli.Command {
	return &cli.Command{
		Name:  "release",
		Usage: "Release and tag management",
		Subcommands: []*cli.Command{
			{
				Name:    "tui",
				Usage:   "Interactive release creator (TUI)",
				Aliases: []string{"ui"},
				Action:  releaseTUIAction,
			},
			{
				Name:  "prepare",
				Usage: "Prepare release notes for human review (fetches PRs, commits, opens editor)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be generated without saving",
					},
				},
				Action: releasePrepareAction,
			},
			{
				Name:   "preview",
				Usage:  "Preview what will happen during release (version bump, changelog, workflow)",
				Action: releasePreviewAction,
			},
			{
				Name:  "commit",
				Usage: "Commit prepared release notes (shortcut for git add/commit)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: releaseCommitAction,
			},
			{
				Name:  "create",
				Usage: "Create a new release (bump version, tag, push, watch workflow)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: releaseCreateAction,
			},
			releaseTagCommand(),
		},
	}
}

func releaseTagCommand() *cli.Command {
	return &cli.Command{
		Name:  "tag",
		Usage: "Tag management commands",
		Subcommands: []*cli.Command{
			{
				Name:    "tui",
				Usage:   "Interactive tag creator (TUI)",
				Aliases: []string{"ui"},
				Action:  tagTUIAction,
			},
			{
				Name:  "prepare",
				Usage: "Prepare a tag version and message for review",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be generated without saving",
					},
				},
				Action: tagPrepareAction,
			},
			{
				Name:   "preview",
				Usage:  "Preview what will happen during tag creation",
				Action: tagPreviewAction,
			},
			{
				Name:  "create",
				Usage: "Create and optionally push a git tag",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: tagCreateAction,
			},
			{
				Name:      "delete",
				Usage:     "Delete a git tag locally and optionally from remote",
				ArgsUsage: "<tag-name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "remote",
						Aliases: []string{"r"},
						Usage:   "Also delete from remote",
					},
					&cli.BoolFlag{
						Name:    "force",
						Aliases: []string{"f"},
						Usage:   "Force deletion of protected tags",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: tagDeleteAction,
			},
			{
				Name:  "list",
				Usage: "List git tags with optional filtering",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "limit",
						Aliases: []string{"n"},
						Usage:   "Limit number of tags shown",
						Value:   20,
					},
					&cli.StringFlag{
						Name:    "pattern",
						Aliases: []string{"p"},
						Usage:   "Filter tags by pattern (e.g., 'v1.*')",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show detailed tag information",
					},
				},
				Action: tagListAction,
			},
		},
	}
}
