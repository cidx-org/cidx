package commands

import (
	"fmt"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/validator"
	"github.com/urfave/cli/v2"
)

func validateCommand() *cli.Command {
	return &cli.Command{
		Name:  "validate",
		Usage: "Validate configuration file and the cidx invocations in CI workflows",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "workflow-dir",
				Aliases: []string{"w"},
				Value:   remote.GitHubWorkflowDir,
				Usage:   "Directory containing GitHub Actions workflow files",
			},
		},
		Action: func(c *cli.Context) error {
			configPath := c.String("config")

			// Find config
			if configPath == "" {
				found, err := config.FindConfig()
				if err != nil {
					return err
				}
				configPath = found
			}

			// Load config
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Validate
			result := config.Validate(cfg)

			fmt.Printf("Validating: %s\n\n", configPath)

			if len(result.Errors) > 0 {
				fmt.Println("Errors:")
				for _, err := range result.Errors {
					fmt.Printf("  ✗ %s\n", err)
				}
				fmt.Println()
			}

			if len(result.Warnings) > 0 {
				fmt.Println("Warnings:")
				for _, warn := range result.Warnings {
					fmt.Printf("  ⚠ %s\n", warn)
				}
				fmt.Println()
			}

			if !result.Valid {
				return fmt.Errorf("configuration validation failed")
			}
			fmt.Println("✓ Configuration is valid")

			// A perfectly valid config does not stop a workflow from calling a
			// subcommand that no longer exists — that went unnoticed for 8 days
			// (issue #239). The command tree comes from the running app, so it
			// can never drift from the CLI it describes.
			return validateWorkflowInvocations(c.App, c.String("workflow-dir"))
		},
	}
}

// validateWorkflowInvocations reports the cidx invocations in the CI workflows
// that no longer resolve against the command tree (issue #239).
func validateWorkflowInvocations(app *cli.App, workflowDir string) error {
	invocations, err := validator.WorkflowInvocations(workflowDir)
	if err != nil {
		return err
	}
	if len(invocations) == 0 {
		return nil
	}

	var stale []validator.StaleInvocation
	for _, inv := range invocations {
		if reason := validator.Resolve(app, inv.Args); reason != "" {
			stale = append(stale, validator.StaleInvocation{Invocation: inv, Reason: reason})
		}
	}

	if len(stale) == 0 {
		fmt.Printf("✓ %d cidx invocation(s) in %s resolve\n", len(invocations), workflowDir)
		return nil
	}

	fmt.Println("\nStale cidx invocations:")
	for _, s := range stale {
		fmt.Printf("  ✗ %s:%d — %s\n", s.File, s.Line, s.Reason)
		fmt.Printf("      %s\n", s.Invocation)
	}
	fmt.Println()

	return fmt.Errorf("%d stale cidx invocation(s) in %s", len(stale), workflowDir)
}
