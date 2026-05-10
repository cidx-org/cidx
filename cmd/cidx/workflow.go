package main

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/actions"
	"github.com/cidx-org/cidx/pkg/branch"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/vcs"
	"github.com/urfave/cli/v2"
)

func workflowCommand() *cli.Command {
	return &cli.Command{
		Name:  "workflow",
		Usage: "GitHub Actions workflow commands",
		Subcommands: []*cli.Command{
			{
				Name:      "list",
				Usage:     "List runs for a GitHub Actions workflow",
				ArgsUsage: "<workflow-name>",
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:    "limit",
						Aliases: []string{"n"},
						Usage:   "Limit number of runs shown",
						Value:   10,
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show detailed run information",
					},
				},
				Action: func(c *cli.Context) error {
					workflow := c.Args().First()
					if workflow == "" {
						return fmt.Errorf("workflow name is required: cidx workflow list <workflow-name>")
					}

					action := actions.NewWorkflowList(
						workflow,
						c.Int("limit"),
						c.Bool("verbose"),
					)

					return action.Execute(context.Background())
				},
			},
			{
				Name:      "watch",
				Usage:     "Watch a workflow run until it completes (works for any branch, no PR required)",
				Aliases:   []string{"w"},
				ArgsUsage: "[run-id]",
				Description: `Watches the most recent workflow run on the current branch by default.

Unlike 'cidx pr watch', this command does not require an open pull request, so
it works for direct pushes to main and any other non-PR branch.

Examples:
  cidx workflow watch                    # latest run on current branch
  cidx workflow watch --branch main      # latest run on main
  cidx workflow watch 12345678           # specific run by ID
  cidx workflow watch --run 12345678     # same, via flag`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "branch",
						Aliases: []string{"b"},
						Usage:   "Branch to watch (defaults to current branch)",
					},
					&cli.StringFlag{
						Name:  "run",
						Usage: "Watch a specific workflow run by ID",
					},
					&cli.BoolFlag{
						Name:    "quiet",
						Aliases: []string{"q"},
						Usage:   "Minimal output (CI-friendly)",
					},
				},
				Action: workflowWatchAction,
			},
		},
	}
}

// workflowWatchAction resolves the run to watch (by ID, by --branch, or by
// current branch) and delegates to actions.WorkflowWatchAction.
func workflowWatchAction(c *cli.Context) error {
	runID := c.String("run")
	if runID == "" && c.Args().Len() > 0 {
		runID = c.Args().First()
	}

	branchName := c.String("branch")

	// If neither a run ID nor an explicit branch is provided, default to the
	// current git branch -- the common case from the user's terminal.
	if runID == "" && branchName == "" {
		var err error
		branchName, err = branch.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewWorkflowWatch(provider, branchName, runID, c.Bool("quiet"))
		return action.Execute(context.Background())
	})
}
