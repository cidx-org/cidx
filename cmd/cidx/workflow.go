package main

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v2/pkg/actions"
	"github.com/cidx-org/cidx/v2/pkg/branch"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
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
				Name:      "run",
				Usage:     "Trigger a workflow and watch the run it starts",
				ArgsUsage: "[options] <workflow>",
				Description: `Triggers a workflow on a ref and follows the run it creates.

The workflow is named by its file: 'ci.yml', or 'ci' for short. It must declare
the 'workflow_dispatch' trigger on the default branch, otherwise GitHub refuses
the dispatch.

--ref defaults to the current branch, which is the point of the command: trying
a workflow change on your own branch before merging it. The branch has to be
pushed.

The run is watched until it completes; --no-watch prints its URL and returns.

Options go before the workflow name -- anything after it is not parsed as a
flag.

Examples:
  cidx workflow run ci                                # current branch
  cidx workflow run --ref main release.yml
  cidx workflow run --input dry_run=true container-monitor.yml
  cidx workflow run --no-watch ci`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "ref",
						Aliases: []string{"r"},
						Usage:   "Branch or tag to run on (defaults to current branch)",
					},
					&cli.StringSliceFlag{
						Name:    "input",
						Aliases: []string{"i"},
						Usage:   "Workflow input as key=value (repeatable)",
					},
					&cli.BoolFlag{
						Name:  "no-watch",
						Usage: "Print the run URL and return instead of watching it",
					},
				},
				Action: workflowRunAction,
			},
			{
				Name:      "watch",
				Usage:     "Watch a workflow run until it completes (works for any branch, no PR required)",
				Aliases:   []string{"w"},
				ArgsUsage: "[run-id]",
				Description: `Watches the most recent workflow run on the current branch by default.

Unlike 'cidx pr watch', this command does not require an open pull request, so
it works for direct pushes to main and any other non-PR branch.

--tag watches the run a tag push triggered (typically the release workflow),
which is a different run from the CI one on the same commit.

Examples:
  cidx workflow watch                    # latest run on current branch
  cidx workflow watch --branch main      # latest run on main
  cidx workflow watch --tag v2.1.4       # run triggered by the v2.1.4 tag push
  cidx workflow watch 12345678           # specific run by ID
  cidx workflow watch --run 12345678     # same, via flag`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "branch",
						Aliases: []string{"b"},
						Usage:   "Branch to watch (defaults to current branch)",
					},
					&cli.StringFlag{
						Name:    "tag",
						Aliases: []string{"t"},
						Usage:   "Watch the run triggered by a tag push (e.g. v2.1.4)",
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

// workflowRunAction resolves the ref (defaulting to the current branch) and
// the inputs, then delegates to actions.WorkflowRunAction.
func workflowRunAction(c *cli.Context) error {
	workflow := c.Args().First()
	if workflow == "" {
		return fmt.Errorf("workflow name is required: cidx workflow run [options] <workflow>")
	}

	// Options placed after the name were caught right here (#266); the check now
	// covers the whole tree, installed from newApp (issue #268).
	inputs, err := actions.ParseWorkflowInputs(c.StringSlice("input"))
	if err != nil {
		return err
	}

	// Validating a change to a workflow means running it on the branch that
	// carries the change, so the current branch is the default.
	ref := c.String("ref")
	if ref == "" {
		ref, err = branch.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewWorkflowRun(provider, workflow, ref, inputs, !c.Bool("no-watch"))
		return action.Execute(context.Background())
	})
}

// workflowWatchAction resolves the run to watch (by ID, by --tag, by --branch,
// or by current branch) and delegates to actions.WorkflowWatchAction.
func workflowWatchAction(c *cli.Context) error {
	runID := c.String("run")
	if runID == "" && c.Args().Len() > 0 {
		runID = c.Args().First()
	}

	branchName := c.String("branch")
	tagName := c.String("tag")

	// With no explicit selector, default to the current git branch -- the
	// common case from the user's terminal.
	if runID == "" && branchName == "" && tagName == "" {
		var err error
		branchName, err = branch.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewWorkflowWatch(provider, branchName, tagName, runID, c.Bool("quiet"))
		return action.Execute(context.Background())
	})
}
