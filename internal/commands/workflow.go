package commands

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
	"github.com/urfave/cli/v2"
)

func workflowCommand() *cli.Command {
	return &cli.Command{
		Name:  "workflow",
		Usage: "GitHub Actions workflow commands",
		Subcommands: []*cli.Command{
			{
				Name:      "list",
				Usage:     "List workflow runs, for one workflow or for a branch",
				ArgsUsage: "[options] [workflow-name]",
				Description: `Lists recent runs, most recent first.

With a workflow name it lists that workflow's runs. With no name it lists the
runs of every workflow on the current branch -- which is what you want when a
check has just failed and you don't yet know which workflow owns it.

--branch names another branch; --branch with a workflow name narrows to that
workflow's runs on that branch.

The 'id' column is the identifier 'workflow watch', 'workflow rerun' and
'artifact download' take. The '#' column is the number the web UI shows, and the
two are not interchangeable (issue #291).

Examples:
  cidx workflow list                       # every workflow, current branch
  cidx workflow list ci                    # the ci workflow, every branch
  cidx workflow list --branch main         # every workflow, main
  cidx workflow list -n 5 security-audit`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "branch",
						Aliases: []string{"b"},
						Usage:   "Branch to list runs for (defaults to the current branch when no workflow is named)",
					},
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
				Action: workflowListAction,
			},
			{
				Name:      "rerun",
				Usage:     "Restart a run, or only the jobs of it that failed",
				ArgsUsage: "[options] [run-id]",
				Description: `Restarts a workflow run.

--failed restarts only the jobs that failed, which is the recovery path when a
job dies on an infrastructure flake -- a registry resetting the connection while
an image is pulled -- rather than on the change.

The run defaults to the most recent one on the current branch. The identifier is
the 'id' column of 'cidx repo workflow list', not the '#' run number.

The rerun is not watched: it starts a new attempt of the same run and the API
reports the previous one as completed for a few seconds, so a watch chained onto
it would report the failure it was asked to clear. The command to watch it is
printed instead.

Examples:
  cidx workflow rerun --failed             # latest run on the current branch
  cidx workflow rerun --failed 18234567890
  cidx workflow rerun 18234567890          # every job of the run`,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "failed",
						Usage: "Restart only the jobs that failed",
					},
					&cli.StringFlag{
						Name:  "run",
						Usage: "Run to restart, by ID (same as the positional argument)",
					},
				},
				Action: workflowRerunAction,
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

// workflowListAction resolves what to list: a named workflow, or -- with no
// name -- every workflow on a branch, defaulting to the current one.
//
// The default only applies when no workflow is named. `cidx workflow list ci`
// keeps listing that workflow across every branch, which is what it has always
// done and what someone checking a workflow's history is asking for (issue #342).
func workflowListAction(c *cli.Context) error {
	workflow := c.Args().First()

	branchName := c.String("branch")
	if workflow == "" && branchName == "" {
		var err error
		branchName, err = branch.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewWorkflowList(provider, workflow, branchName, c.Int("limit"), c.Bool("verbose"))
		return action.Execute(context.Background())
	})
}

// workflowRerunAction resolves the run to restart -- by ID, or the latest run on
// the current branch -- and delegates to actions.WorkflowRerunAction.
func workflowRerunAction(c *cli.Context) error {
	runID := c.String("run")
	if runID == "" {
		runID = c.Args().First()
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		if runID == "" {
			current, err := branch.GetCurrentBranch()
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}
			run, err := provider.GetLatestRunForBranch(context.Background(), current)
			if err != nil {
				return fmt.Errorf("no workflow run found for branch %q, so there is nothing to rerun: %w", current, err)
			}
			runID = run.ID
		}

		action := actions.NewWorkflowRerun(provider, runID, c.Bool("failed"))
		return action.Execute(context.Background())
	})
}

// workflowRunAction resolves the ref (defaulting to the current branch) and
// the inputs, then delegates to actions.WorkflowRunAction.
func workflowRunAction(c *cli.Context) error {
	workflow := c.Args().First()
	if workflow == "" {
		return fmt.Errorf("workflow name is required: cidx workflow run [options] <workflow>")
	}

	// Options placed after the name were caught right here (#266); the check now
	// covers the whole tree, installed from NewApp (issue #268).
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
