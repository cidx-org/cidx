package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/drift"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/validator"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var checkCommand = &cli.Command{
	Name:  "check",
	Usage: "Validate configuration and workflows",
	Subcommands: []*cli.Command{
		{
			Name:  "drift",
			Usage: "Compare cidx.toml with actual CI workflow and detect divergence",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "file",
					Aliases:     []string{"f"},
					Usage:       "Path to CI workflow file",
					DefaultText: "auto-detect: cidx.yml, ci.yml",
				},
			},
			Action: checkDriftAction,
		},
		{
			Name:      "workflow",
			Usage:     "Validate that cidx.toml pipelines match GitHub Actions workflows",
			ArgsUsage: "[pipeline-name]",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:    "workflow-dir",
					Aliases: []string{"w"},
					Value:   ".github/workflows",
					Usage:   "Directory containing GitHub Actions workflow files",
				},
				&cli.BoolFlag{
					Name:    "verbose",
					Aliases: []string{"v"},
					Value:   false,
					Usage:   "Show detailed validation information",
				},
			},
			Action: checkWorkflowAction,
		},
	},
}

func checkWorkflowAction(c *cli.Context) error {
	// Load configuration
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		logrus.Fatalf("Failed to load configuration: %v", err)
		return err
	}

	workflowDir := c.String("workflow-dir")
	verbose := c.Bool("verbose")
	pipelineName := c.Args().First()

	// Validate specific pipeline or all pipelines
	var results []*validator.ValidationResult

	if pipelineName != "" {
		// Validate specific pipeline
		workflowFile := filepath.Join(workflowDir, pipelineName+".yml")
		if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
			logrus.Errorf("Workflow file not found: %s", workflowFile)
			return fmt.Errorf("workflow file not found: %s", workflowFile)
		}

		result, err := validator.ValidateWorkflow(cfg, pipelineName, workflowFile)
		if err != nil {
			logrus.Errorf("Validation failed: %v", err)
			return err
		}
		results = []*validator.ValidationResult{result}
	} else {
		// Validate all workflows
		results, err = validator.ValidateAllWorkflows(cfg, workflowDir)
		if err != nil {
			logrus.Errorf("Validation failed: %v", err)
			return err
		}

		if len(results) == 0 {
			logrus.Warn("No workflows found to validate")
			fmt.Println("⚠️  No GitHub Actions workflows found in", workflowDir)
			return nil
		}
	}

	// Display results
	allSuccess := true
	for _, result := range results {
		output := validator.FormatResult(result)
		fmt.Print(output)

		if !result.Success {
			allSuccess = false
		}

		// Show verbose details if requested
		if verbose {
			fmt.Println()
		}
	}

	// Summary
	fmt.Println()
	if allSuccess {
		logrus.Info("✅ All workflows are in sync with pipelines")
		fmt.Println("✅ All workflows are in sync with pipelines")
	} else {
		logrus.Warn("⚠️  Some workflows have differences with pipelines")
		fmt.Println("⚠️  Some workflows have differences with pipelines")
		return cli.Exit("", 1)
	}

	return nil
}

func checkDriftAction(c *cli.Context) error {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		return fmt.Errorf("failed to load cidx.toml: %w", err)
	}

	// Explicit --file wins; otherwise probe the candidate workflow names
	// (cidx.yml as written by generate/init, then ci.yml) — issue #170.
	workflowFile := c.String("file")
	if workflowFile == "" {
		workflowFile, err = remote.ResolveWorkflowFile(remote.GitHubWorkflowDir)
		if err != nil {
			return fmt.Errorf("%w — use --file to specify the workflow path", err)
		}
	} else if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
		return fmt.Errorf("workflow file not found: %s (use --file to specify path)", workflowFile)
	}

	result, err := drift.Compare(cfg, workflowFile)
	if err != nil {
		return err
	}

	fmt.Print(drift.Format(result))
	fmt.Println()

	if !result.HasDrift() {
		fmt.Println("\033[32mNo drift detected.\033[0m")
		return nil
	}

	return fmt.Errorf("%d difference(s) found", result.DiffCount())
}
