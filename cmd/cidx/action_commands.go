package main

import "github.com/urfave/cli/v2"

// cpwCommand returns the commit-push-watch command definition.
func cpwCommand() *cli.Command {
	return &cli.Command{
		Name:    "commit-push-watch",
		Usage:   "Commit changes, push, and watch remote workflow",
		Aliases: []string{"cpw"},
		Description: `Runs the ` + "`code`" + ` phase before committing, so a formatting slip or a
lint issue is caught here instead of costing a full CI cycle plus a second
commit. Same contract as git's pre-commit hook, including --no-verify.

The phase is skipped, with a message, when there is nothing to run (no code
phase configured), nothing to run it in (no container runtime), or a pre-commit
hook that already runs it on the commit.`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "message",
				Aliases:  []string{"m"},
				Usage:    "Commit message",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "no-verify",
				Usage: "Push without running the code phase first",
			},
		},
		Action: commitPushWatchAction,
	}
}

// prCommand returns the PR subcommand tree.
func prCommand() *cli.Command {
	return &cli.Command{
		Name:  "pr",
		Usage: "Pull request workflow commands",
		Subcommands: []*cli.Command{
			{
				Name:      "create",
				Usage:     "Create a new draft PR with feature branch",
				ArgsUsage: "[title]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "issue",
						Aliases: []string{"i"},
						Usage:   "Link to existing issue number",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: prCreateAction,
			},
			{
				Name:  "ready",
				Usage: "Mark the current draft PR as ready for review",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: prReadyAction,
			},
			{
				Name:  "edit",
				Usage: "Update title and/or body of the current branch's PR",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "title",
						Aliases: []string{"t"},
						Usage:   "New PR title",
					},
					&cli.StringFlag{
						Name:    "body",
						Aliases: []string{"b"},
						Usage:   "New PR body",
					},
				},
				Action: prEditAction,
			},
			{
				Name:  "merge",
				Usage: "Merge the current PR (non-interactive) and optionally watch post-merge workflow",
				Description: `Merges as soon as the required checks are green. This command is
non-interactive by design -- it asks for no confirmation, so it behaves the same
when driven by a human or by a script.

Use --dry-run to see what would be merged without merging anything.

The green-checks gate is the safety net: --skip-checks removes it (not
recommended).`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "method",
						Aliases: []string{"m"},
						Usage:   "Merge method: merge, squash, or rebase",
						Value:   "squash",
					},
					&cli.BoolFlag{
						Name:    "watch",
						Aliases: []string{"w"},
						Usage:   "Watch post-merge workflow",
					},
					&cli.BoolFlag{
						Name:  "skip-checks",
						Usage: "Skip pre-merge checks validation (not recommended)",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be done without making changes",
					},
				},
				Action: prMergeAction,
			},
			{
				Name:      "status",
				Usage:     "Show PR status for current branch",
				ArgsUsage: "[branch]",
				Action:    prStatusAction,
			},
			{
				Name:      "watch",
				Usage:     "Watch CI checks until they complete",
				Aliases:   []string{"w"},
				ArgsUsage: "[branch]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "quiet",
						Aliases: []string{"q"},
						Usage:   "Minimal output (no spinner animation, CI-friendly)",
					},
				},
				Action: prWatchAction,
			},
			{
				Name:   "open",
				Usage:  "Open PR in browser",
				Action: prOpenAction,
			},
			{
				Name:    "tui",
				Usage:   "Interactive PR merge interface (TUI)",
				Aliases: []string{"ui"},
				Action:  prTUIAction,
			},
		},
	}
}
