package main

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/actions"
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
		},
	}
}
