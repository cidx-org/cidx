package main

import (
	"fmt"

	"github.com/cidx-org/cidx/pkg/branch"
	"github.com/cidx-org/cidx/pkg/config"
	"github.com/urfave/cli/v2"
)

// getPRManager creates a branch manager and resolves the current branch.
func getPRManager(c *cli.Context) (*branch.Manager, string, error) {
	branchName := c.Args().First()
	if branchName == "" {
		var err error
		branchName, err = branch.GetCurrentBranch()
		if err != nil {
			return nil, "", fmt.Errorf("failed to get current branch: %w", err)
		}
	}

	cfg, _ := config.Load("cidx.toml")
	branchCfg := branch.Config{
		Protected: []string{"main", "master", "develop"},
	}
	if cfg != nil && len(cfg.Branch.Protected) > 0 {
		branchCfg.Protected = cfg.Branch.Protected
	}

	manager := branch.NewManager(branchCfg)
	return manager, branchName, nil
}

func prStatusAction(c *cli.Context) error {
	manager, branchName, err := getPRManager(c)
	if err != nil {
		return err
	}

	info, err := manager.GetPRInfo(branchName)
	if err != nil {
		return fmt.Errorf("no PR found for branch '%s': %w", branchName, err)
	}

	output := branch.FormatPRInfo(info)
	fmt.Print(output)
	return nil
}

func prWatchAction(c *cli.Context) error {
	manager, branchName, err := getPRManager(c)
	if err != nil {
		return err
	}

	info, err := manager.GetPRInfo(branchName)
	if err != nil {
		return fmt.Errorf("no PR found for branch '%s': %w", branchName, err)
	}

	return watchPRChecks(manager, branchName, info, c.Bool("quiet"))
}

func prOpenAction(c *cli.Context) error {
	manager, branchName, err := getPRManager(c)
	if err != nil {
		return err
	}

	info, err := manager.GetPRInfo(branchName)
	if err != nil {
		return fmt.Errorf("no PR found for branch '%s': %w", branchName, err)
	}

	if err := openBrowser(info.URL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
		fmt.Printf("URL: %s\n", info.URL)
	}
	return nil
}
