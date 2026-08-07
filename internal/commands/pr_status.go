package commands

import (
	"errors"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cidx-org/cidx/v3/pkg/config"
	"github.com/cidx-org/cidx/v3/pkg/remote"
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

// resolvePR finds the pull request of a branch, and tells the two ways there
// can be none apart.
//
// Sitting on the trunk with no PR open is not a failure, it is where every
// session starts and ends. `cidx pr status` answered `Error: no PR found for
// branch 'main'` and exit 1 there, which printed a fault for a healthy
// repository and made the command unusable in any `&&` chain or prompt helper
// (issue #362). It reports the absence and exits 0 now -- the same distinction
// `check drift` draws between "nothing to report" and "something is wrong".
//
// On a branch that is not protected, having no PR stays an error: that is a
// branch someone made to open one from, and saying so is the useful answer.
//
// Only a genuine absence takes the quiet path. remote.ErrNoPullRequest is the
// provider saying "there is none"; an expired token or a broken network is a
// different error and still fails, because exiting 0 on those would report a
// healthy repository for an unreachable one.
func resolvePR(c *cli.Context) (*branch.PRInfo, error) {
	manager, branchName, err := getPRManager(c)
	if err != nil {
		return nil, err
	}

	info, err := manager.GetPRInfo(branchName)
	if err == nil {
		return info, nil
	}

	if prAbsenceIsNormal(err, manager.IsProtected(branchName)) {
		fmt.Printf("No pull request for '%s', which is where a branch is opened from rather than one that has a PR.\n", branchName)
		fmt.Println("Start one with: cidx pr create \"your title\"")
		return nil, nil
	}

	return nil, fmt.Errorf("no PR found for branch '%s': %w", branchName, err)
}

// prAbsenceIsNormal reports whether "this branch has no pull request" is the
// expected state rather than something to report as a fault.
//
// Both halves matter. A protected branch is one PRs are opened *from*, so
// having none there is normal; anywhere else it is worth exit 1. And only
// remote.ErrNoPullRequest counts as an absence -- an expired token answers "no
// PR" just as convincingly and means the opposite, so treating the two alike
// would report a healthy repository for an unreachable one (issue #362).
func prAbsenceIsNormal(err error, protected bool) bool {
	return errors.Is(err, remote.ErrNoPullRequest) && protected
}

func prStatusAction(c *cli.Context) error {
	info, err := resolvePR(c)
	if err != nil || info == nil {
		return err
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

	info, err := resolvePR(c)
	if err != nil || info == nil {
		return err
	}

	return watchPRChecks(manager, branchName, info, c.Bool("quiet"))
}

func prOpenAction(c *cli.Context) error {
	info, err := resolvePR(c)
	if err != nil || info == nil {
		return err
	}

	if err := openBrowser(info.URL); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
		fmt.Printf("URL: %s\n", info.URL)
	}
	return nil
}
