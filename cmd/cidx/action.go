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

func prEditAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewPREdit(repo, provider, c.String("title"), c.String("body"))
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

