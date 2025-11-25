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
			{
				Name:  "pr",
				Usage: "Pull request workflow commands",
				Subcommands: []*cli.Command{
					{
						Name:      "create",
						Usage:     "Create a new draft PR with feature branch (like GitLab workflow)",
						ArgsUsage: "[title]",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "issue",
								Aliases: []string{"i"},
								Usage:   "Link to existing issue number",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: prCreateAction,
					},
					{
						Name:   "ready",
						Usage:  "Mark the current draft PR as ready for review",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: prReadyAction,
					},
					{
						Name:  "merge",
						Usage: "Merge the current PR and optionally watch post-merge workflow",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "method",
								Aliases: []string{"m"},
								Usage:   "Merge method: merge, squash, or rebase",
								Value:   "squash",
							},
							&cli.BoolFlag{
								Name:    "watch",
								Aliases: []string{"w"},
								Usage:   "Watch post-merge workflow",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: prMergeAction,
					},
				},
			},
			{
				Name:  "release",
				Usage: "Release management commands",
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create a new release with commitizen (bump version, tag, push, watch workflow)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: releaseCreateAction,
					},
				},
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

func releaseCreateAction(c *cli.Context) error {
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

	// Create and execute release action
	action := actions.NewRelease(
		repo,
		provider,
		"release-create", // Action name from cidx.toml
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func prCreateAction(c *cli.Context) error {
	// Get PR title from args or prompt
	title := c.Args().First()
	if title == "" {
		return fmt.Errorf("PR title is required: cidx action pr create \"Your PR title\"")
	}

	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get remote info
	owner, repoName, err := repo.GetRemoteInfo()
	if err != nil {
		return fmt.Errorf("failed to get remote info: %w", err)
	}

	// Get GitHub token
	token, err := getGitHubToken()
	if err != nil {
		return err
	}

	// Create GitHub provider
	provider := github.NewClient(token, owner, repoName)

	// Create and execute PR action
	action := actions.NewPR(
		repo,
		provider,
		title,
		c.String("issue"),
		c.Bool("dry-run"),
		false, // not ready mode
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func prReadyAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get remote info
	owner, repoName, err := repo.GetRemoteInfo()
	if err != nil {
		return fmt.Errorf("failed to get remote info: %w", err)
	}

	// Get GitHub token
	token, err := getGitHubToken()
	if err != nil {
		return err
	}

	// Create GitHub provider
	provider := github.NewClient(token, owner, repoName)

	// Create and execute PR ready action
	action := actions.NewPR(
		repo,
		provider,
		"",    // no title needed for ready
		"",    // no issue needed for ready
		c.Bool("dry-run"),
		true, // ready mode
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func prMergeAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get remote info
	owner, repoName, err := repo.GetRemoteInfo()
	if err != nil {
		return fmt.Errorf("failed to get remote info: %w", err)
	}

	// Get GitHub token
	token, err := getGitHubToken()
	if err != nil {
		return err
	}

	// Create GitHub provider
	provider := github.NewClient(token, owner, repoName)

	// Create and execute PR merge action
	action := actions.NewPRMerge(
		repo,
		provider,
		c.String("method"),
		c.Bool("watch"),
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}
