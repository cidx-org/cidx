package main

import (
	"context"
	"fmt"
	"os"

	"github.com/arcker/cidx/pkg/actions"
	"github.com/arcker/cidx/pkg/remote/github"
	"github.com/arcker/cidx/pkg/vcs"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/urfave/cli/v2"
)

// getGitHubToken retrieves GitHub token from env var or gh CLI auth
func getGitHubToken() (string, error) {
	// 1. Try environment variable first
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	// 2. Fallback to gh CLI auth
	token, _ := auth.TokenForHost("github.com")
	if token == "" {
		return "", fmt.Errorf("no GitHub token found: set GITHUB_TOKEN or run 'gh auth login'")
	}
	return token, nil
}

func actionCommand() *cli.Command {
	return &cli.Command{
		Name:  "action",
		Usage: "Run automated actions (commit-push-watch, release, etc.)",
		Subcommands: []*cli.Command{
			{
				Name:  "commit-push-watch",
				Usage: "Commit changes, push, and watch remote workflow",
				Aliases: []string{"cpw"},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "message",
						Aliases:  []string{"m"},
						Usage:    "Commit message",
						Required: true,
					},
				},
				Action: commitPushWatchAction,
			},
		},
	}
}

func commitPushWatchAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get remote info (owner/repo)
	owner, repoName, err := repo.GetRemoteInfo()
	if err != nil {
		return fmt.Errorf("failed to get remote info: %w", err)
	}

	// Get GitHub token (from env var or gh CLI auth)
	token, err := getGitHubToken()
	if err != nil {
		return err
	}

	// Create GitHub provider
	provider := github.NewClient(token, owner, repoName)

	// Create and execute action
	action := actions.NewCommitPushWatch(
		repo,
		provider,
		c.String("message"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}
