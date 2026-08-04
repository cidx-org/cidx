package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/drift"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/validator"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// checkCommand is built on demand like every other command. It used to be a
// package-level value, which made it the one branch of the tree shared between
// two newApp() calls -- urfave appends a `help` subcommand to the tree it runs,
// so the second call got a tree the first one had mutated (issue #268).
func checkCommand() *cli.Command {
	return &cli.Command{
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
}

// resolveConfigPath returns the config file to read: the global --config when
// one is given, otherwise the auto-detected one. `run` has always honoured the
// flag; `check workflow` hardcoded cidx.toml, so a project whose config is
// named differently could not check its workflows (issue #230).
func resolveConfigPath(c *cli.Context) (string, error) {
	if path := c.String("config"); path != "" {
		return path, nil
	}
	return config.FindConfig()
}

// loadResolvedConfig loads the config resolveConfigPath points at. Every
// command that reads the project config goes through it, so none can drift back
// to a hardcoded cidx.toml the way `generate` and `check drift` had (issue #240
// follow-up to #230).
func loadResolvedConfig(c *cli.Context) (*config.Config, error) {
	configPath, err := resolveConfigPath(c)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load %s: %w", configPath, err)
	}
	return cfg, nil
}

func checkWorkflowAction(c *cli.Context) error {
	// Load configuration
	cfg, err := loadResolvedConfig(c)
	if err != nil {
		return err
	}

	workflowDir := c.String("workflow-dir")
	verbose := c.Bool("verbose")
	pipelineName := c.Args().First()

	// Validate specific pipeline or all pipelines
	var results []*validator.ValidationResult

	if pipelineName != "" {
		// Validate specific pipeline
		pipeline, exists := cfg.Pipelines[pipelineName]
		if !exists {
			return fmt.Errorf("pipeline '%s' not found in configuration", pipelineName)
		}

		// A pipeline can state that no workflow implements it. Asking for it by
		// name then has a real answer — nothing to compare — rather than a
		// comparison with a file that only shares its name (issue #233).
		workflowName := pipeline.WorkflowFile(pipelineName)
		if workflowName == "" {
			fmt.Printf("ℹ️  Pipeline '%s' declares workflow = %q — no workflow implements it, nothing to compare\n", pipelineName, config.NoWorkflow)
			return nil
		}

		workflowFile := filepath.Join(workflowDir, workflowName)
		if _, err := os.Stat(workflowFile); os.IsNotExist(err) {
			logrus.Errorf("Workflow file not found: %s", workflowFile)
			return fmt.Errorf("workflow file not found: %s", workflowFile)
		}

		result, err := validator.ValidateWorkflow(c.App, cfg, pipelineName, workflowFile)
		if err != nil {
			logrus.Errorf("Validation failed: %v", err)
			return err
		}
		results = []*validator.ValidationResult{result}
	} else {
		// Validate all workflows
		results, err = validator.ValidateAllWorkflows(c.App, cfg, workflowDir)
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
	summary := checkWorkflowSummary(pipelineName, len(results), allSuccess)
	if allSuccess {
		logrus.Info(summary)
		fmt.Println(summary)
		return nil
	}

	logrus.Warn(summary)
	fmt.Println(summary)
	return cli.Exit("", 1)
}

// checkWorkflowSummary states what was actually checked.
//
// One line used to serve both paths, so `cidx check workflow ci` — which
// compares exactly one pipeline — signed off with "All workflows are in sync
// with pipelines" and read as a clean bill for the repository. It cost a wrong
// conclusion on a repository whose release.yml was out of sync at the time
// (issue #318, the same family as #259). A summary may only claim the ground it
// covered: the pipeline named on the command line, or the sweep and its size.
func checkWorkflowSummary(pipelineName string, checked int, inSync bool) string {
	if pipelineName != "" {
		if inSync {
			return fmt.Sprintf("✅ Pipeline '%s' is in sync with its workflow", pipelineName)
		}
		return fmt.Sprintf("⚠️  Pipeline '%s' has differences with its workflow", pipelineName)
	}

	if inSync {
		return fmt.Sprintf("✅ All %d workflow(s) are in sync with pipelines", checked)
	}
	return fmt.Sprintf("⚠️  Some of the %d checked workflow(s) have differences with pipelines", checked)
}

func checkDriftAction(c *cli.Context) error {
	cfg, err := loadResolvedConfig(c)
	if err != nil {
		return err
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
		fmt.Println(drift.Summary(result))
		return nil
	}

	return errors.New(drift.Summary(result))
}
