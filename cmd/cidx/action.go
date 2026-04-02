package main

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/actions"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/remote/github"
	"github.com/cidx-org/cidx/pkg/vcs"
	"github.com/urfave/cli/v2"
)

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
					{
						Name:    "tui",
						Usage:   "Interactive PR merge interface (TUI)",
						Aliases: []string{"ui"},
						Action:  prTUIAction,
					},
				},
			},
			{
				Name:  "tag",
				Usage: "Tag management commands",
				Subcommands: []*cli.Command{
					{
						Name:    "tui",
						Usage:   "Interactive tag creator (TUI)",
						Aliases: []string{"ui"},
						Action:  tagTUIAction,
					},
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
				Name:  "artifact",
				Usage: "GitHub Actions artifact management",
				Subcommands: []*cli.Command{
					{
						Name:    "tui",
						Usage:   "Interactive artifact manager (TUI)",
						Aliases: []string{"ui"},
						Action:  artifactTUIAction,
					},
					{
						Name:  "list",
						Usage: "List all artifacts with storage statistics",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:    "verbose",
								Aliases: []string{"v"},
								Usage:   "Show detailed artifact information",
							},
						},
						Action: artifactListAction,
					},
					{
						Name:  "stats",
						Usage: "Show artifact storage statistics",
						Action: artifactStatsAction,
					},
					{
						Name:  "cleanup",
						Usage: "Delete artifacts to free storage space",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "all",
								Usage: "Delete all artifacts",
							},
							&cli.BoolFlag{
								Name:  "expired",
								Usage: "Delete only expired artifacts",
							},
							&cli.IntFlag{
								Name:    "older-than",
								Aliases: []string{"d"},
								Usage:   "Delete artifacts older than N days",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Show what would be deleted without making changes",
							},
						},
						Action: artifactCleanupAction,
					},
				},
			},
			{
				Name:  "release",
				Usage: "Release management commands",
				Subcommands: []*cli.Command{
					{
						Name:    "tui",
						Usage:   "Interactive release creator (TUI)",
						Aliases: []string{"ui"},
						Action:  releaseTUIAction,
					},
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
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewCommitPushWatch(repo, provider, c.String("message"))
		return action.Execute(context.Background())
	})
}

func releaseCreateAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewRelease(repo, provider, loadReleaseConfig(), "release-create", c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func prCreateAction(c *cli.Context) error {
	title := c.Args().First()
	if title == "" {
		return fmt.Errorf("PR title is required: cidx action pr create \"Your PR title\"")
	}

	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewPR(repo, provider, title, c.String("issue"), c.Bool("dry-run"), false)
		return action.Execute(context.Background())
	})
}

func prReadyAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewPR(repo, provider, "", "", c.Bool("dry-run"), true)
		return action.Execute(context.Background())
	})
}

func prMergeAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewPRMerge(repo, provider, c.String("method"), c.Bool("watch"), c.Bool("skip-checks"), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func releasePrepareAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewReleasePrepare(repo, provider, loadReleaseConfig(), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func releasePreviewAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewReleasePreview(repo, loadReleaseConfig(), false)
		return action.Execute(context.Background())
	})
}

func releaseCommitAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewReleaseCommit(repo, c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func tagPrepareAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewTagPrepare(repo, loadTagConfig(), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func tagPreviewAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewTagPreview(repo, loadTagConfig())
		return action.Execute(context.Background())
	})
}

func tagCreateAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewTagCreate(repo, loadTagConfig(), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func tagTUIAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		return runReleaseTUI(modeTag, repo, nil, loadTagConfig(), loadReleaseConfig())
	})
}

func releaseTUIAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		return runReleaseTUI(modeRelease, repo, provider, loadTagConfig(), loadReleaseConfig())
	})
}

func prTUIAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		branch, err := repo.GetCurrentBranch()
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}

		// Cast to GitHub client (TUI requires GitHub-specific methods)
		ghClient, ok := provider.(*github.Client)
		if !ok {
			return fmt.Errorf("PR TUI is only supported for GitHub repositories")
		}

		prNumber, _, err := provider.GetPullRequestByBranch(context.Background(), branch)
		if err != nil {
			return fmt.Errorf("no PR found for branch %s: %w", branch, err)
		}

		return runMergeTUI(ghClient, prNumber, loadPRConfig())
	})
}

func tagDeleteAction(c *cli.Context) error {
	tagName := c.Args().First()
	if tagName == "" {
		return fmt.Errorf("tag name is required: cidx action tag delete <tag-name>")
	}

	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewTagDelete(repo, loadTagConfig(), tagName, c.Bool("remote"), c.Bool("force"), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func tagListAction(c *cli.Context) error {
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewTagList(repo, loadTagConfig(), c.Int("limit"), c.String("pattern"), c.Bool("verbose"))
		return action.Execute(context.Background())
	})
}

func artifactListAction(c *cli.Context) error {
	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewArtifactList(provider, c.Bool("verbose"))
		return action.Execute(context.Background())
	})
}

func artifactStatsAction(c *cli.Context) error {
	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewArtifactStats(provider)
		return action.Execute(context.Background())
	})
}

func artifactCleanupAction(c *cli.Context) error {
	if !c.Bool("all") && !c.Bool("expired") && c.Int("older-than") == 0 {
		return fmt.Errorf("must specify --all, --expired, or --older-than <days>")
	}

	return withRepoAndProvider(func(_ *vcs.Repository, provider remote.Provider) error {
		action := actions.NewArtifactCleanup(provider, c.Bool("all"), c.Bool("expired"), c.Int("older-than"), c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func artifactTUIAction(c *cli.Context) error {
	ghClient, err := getGitHubClient()
	if err != nil {
		return err
	}

	return runArtifactTUI(ghClient)
}

