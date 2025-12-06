package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cidx-org/cidx/pkg/actions"
	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/remote/github"
	"github.com/cidx-org/cidx/pkg/vcs"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/urfave/cli/v2"
)

// loadReleaseConfig loads the release configuration from cidx.toml or returns defaults
func loadReleaseConfig() config.ReleaseConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		// Return defaults if no config file
		return config.DefaultReleaseConfig()
	}
	// Apply defaults for unset values
	if cfg.Release.MainBranch == "" {
		cfg.Release.MainBranch = "main"
	}
	// AutoCleanup defaults to true (zero value is false, so we need special handling)
	// This is already handled by the config loader or we use the default
	return cfg.Release
}

// loadTagConfig loads the tag configuration from cidx.toml or returns defaults
func loadTagConfig() config.TagConfig {
	cfg, err := config.Load("cidx.toml")
	if err != nil {
		// Return defaults if no config file
		return config.DefaultTagConfig()
	}
	return cfg.Tag
}

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
								Name:  "skip-checks",
								Usage: "Skip pre-merge checks validation (not recommended)",
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
				Name:  "tag",
				Usage: "Tag management commands",
				Subcommands: []*cli.Command{
					{
						Name:  "prepare",
						Usage: "Prepare a tag version and message for review",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be generated without saving",
							},
						},
						Action: tagPrepareAction,
					},
					{
						Name:  "preview",
						Usage: "Preview what will happen during tag creation",
						Action: tagPreviewAction,
					},
					{
						Name:  "create",
						Usage: "Create and optionally push a git tag",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: tagCreateAction,
					},
					{
						Name:      "delete",
						Usage:     "Delete a git tag locally and optionally from remote",
						ArgsUsage: "<tag-name>",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "remote",
								Aliases: []string{"r"},
								Usage:   "Also delete from remote",
							},
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Force deletion of protected tags",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: tagDeleteAction,
					},
					{
						Name:  "list",
						Usage: "List git tags with optional filtering",
						Flags: []cli.Flag{
							&cli.IntFlag{
								Name:    "limit",
								Aliases: []string{"n"},
								Usage:   "Limit number of tags shown",
								Value:   20,
							},
							&cli.StringFlag{
								Name:    "pattern",
								Aliases: []string{"p"},
								Usage:   "Filter tags by pattern (e.g., 'v1.*')",
							},
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Show detailed tag information",
							},
						},
						Action: tagListAction,
					},
				},
			},
			{
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
						Action: workflowListAction,
					},
				},
			},
			{
				Name:  "release",
				Usage: "Release management commands",
				Subcommands: []*cli.Command{
					{
						Name:  "prepare",
						Usage: "Prepare release notes for human review (fetches PRs, commits, opens editor)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be generated without saving",
							},
						},
						Action: releasePrepareAction,
					},
					{
						Name:  "preview",
						Usage: "Preview what will happen during release (version bump, changelog, workflow)",
						Action: releasePreviewAction,
					},
					{
						Name:  "commit",
						Usage: "Commit prepared release notes (shortcut for git add/commit)",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be done without making changes",
							},
						},
						Action: releaseCommitAction,
					},
					{
						Name:  "create",
						Usage: "Create a new release (bump version, tag, push, watch workflow)",
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

	// Load release config
	releaseConfig := loadReleaseConfig()

	// Create and execute release action
	action := actions.NewRelease(
		repo,
		provider,
		releaseConfig,
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
		c.Bool("skip-checks"),
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func releasePrepareAction(c *cli.Context) error {
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

	// Load release config
	releaseConfig := loadReleaseConfig()

	// Create and execute release prepare action
	action := actions.NewReleasePrepare(
		repo,
		provider,
		releaseConfig,
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func releasePreviewAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load release config
	releaseConfig := loadReleaseConfig()

	// Create and execute release preview action
	action := actions.NewReleasePreview(
		repo,
		releaseConfig,
		false, // preview is always "dry-run" style
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func releaseCommitAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Create and execute release commit action
	action := actions.NewReleaseCommit(
		repo,
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func tagPrepareAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load tag config
	tagConfig := loadTagConfig()

	// Create and execute tag prepare action
	action := actions.NewTagPrepare(
		repo,
		tagConfig,
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func tagPreviewAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load tag config
	tagConfig := loadTagConfig()

	// Create and execute tag preview action
	action := actions.NewTagPreview(
		repo,
		tagConfig,
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func tagCreateAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load tag config
	tagConfig := loadTagConfig()

	// Create and execute tag create action
	action := actions.NewTagCreate(
		repo,
		tagConfig,
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func tagDeleteAction(c *cli.Context) error {
	// Get tag name from args
	tagName := c.Args().First()
	if tagName == "" {
		return fmt.Errorf("tag name is required: cidx action tag delete <tag-name>")
	}

	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load tag config
	tagConfig := loadTagConfig()

	// Create and execute tag delete action
	action := actions.NewTagDelete(
		repo,
		tagConfig,
		tagName,
		c.Bool("remote"),
		c.Bool("force"),
		c.Bool("dry-run"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func tagListAction(c *cli.Context) error {
	// Open repository
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Load tag config
	tagConfig := loadTagConfig()

	// Create and execute tag list action
	action := actions.NewTagList(
		repo,
		tagConfig,
		c.Int("limit"),
		c.String("pattern"),
		c.Bool("verbose"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}

func workflowListAction(c *cli.Context) error {
	// Get workflow name from args
	workflow := c.Args().First()
	if workflow == "" {
		return fmt.Errorf("workflow name is required: cidx action workflow list <workflow-name>")
	}

	// Create and execute workflow list action
	action := actions.NewWorkflowList(
		workflow,
		c.Int("limit"),
		c.Bool("verbose"),
	)

	ctx := context.Background()
	return action.Execute(ctx)
}
