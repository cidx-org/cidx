package commands

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/branch"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cidx-org/cidx/v3/pkg/remote/github"
	"github.com/cidx-org/cidx/v3/pkg/vcs"
	log "github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// commitPushWatchAction resolves the remote before anything happens, and that
// is a decision rather than an oversight (issue #358).
//
// The eager-provider sweeps of #227, #350 and #356 moved every other command to
// withRepoAndLazyProvider: a command whose local-only steps never reach the
// remote must not fail on an unusable one. cpw looks like the last of that
// family and is deliberately not treated as one, because deferring here would
// not move construction — it would change what an unusable remote *means* for a
// command that has already mutated the repository.
//
// cpw's contract is commit, push and watch. Lazily, an unparseable origin or an
// expired token would leave a commit made and a push attempted, then fail; the
// user would be somewhere they did not ask to be, after the side effects rather
// than before. Failing first leaves the working tree exactly as it was, with
// nothing to amend or reset — the same posture #307 chose for the code phase,
// which runs before the commit for that reason.
//
// TestCPWRefusesBeforeTouchingTheRepository holds the line.
func commitPushWatchAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		action := actions.NewCommitPushWatch(repo, provider, c.String("message"), !c.Bool("no-verify"))
		return action.Execute(context.Background())
	})
}

func releaseCreateAction(c *cli.Context) error {
	return withRepoAndLazyProvider(func(repo *vcs.Repository, resolveProvider remote.ProviderFunc) error {
		action := actions.NewRelease(repo, resolveProvider, loadReleaseConfig(), "release-create", c.Bool("dry-run"))
		return action.Execute(context.Background())
	})
}

func prCreateAction(c *cli.Context) error {
	title := c.Args().First()
	if title == "" {
		return fmt.Errorf("PR title is required: cidx pr create \"Your PR title\"")
	}

	return withRepoAndLazyProvider(func(repo *vcs.Repository, resolveProvider remote.ProviderFunc) error {
		action := actions.NewPR(repo, resolveProvider, title, c.String("issue"), c.Bool("dry-run"), false)
		return action.Execute(context.Background())
	})
}

func prReadyAction(c *cli.Context) error {
	return withRepoAndLazyProvider(func(repo *vcs.Repository, resolveProvider remote.ProviderFunc) error {
		action := actions.NewPR(repo, resolveProvider, "", "", c.Bool("dry-run"), true)
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
	return withRepo(func(repo *vcs.Repository) error {
		action := actions.NewReleasePrepare(repo, loadReleaseConfig(), c.Bool("dry-run"))
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
		return fmt.Errorf("tag name is required: cidx release tag delete <tag-name>")
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

// artifactDownloadAction resolves the run whose artifacts to fetch -- the one
// --run names, or the latest on the current branch -- and delegates to
// actions.ArtifactDownloadAction.
//
// The run is pinned before anything is downloaded. Artifacts only mean something
// taken from one run: the readers of these files compare a whole catalogue at a
// point in time, and mixing two runs answers a different question without saying
// so (issue #285).
func artifactDownloadAction(c *cli.Context) error {
	return withRepoAndProvider(func(repo *vcs.Repository, provider remote.Provider) error {
		// Said out loud, because the failure it guards against is a silent one:
		// `gh run download <id>` resolves the repository from gh's own notion of
		// where you are -- a default, an environment variable, the last thing it
		// knew -- and hands over another repository's artifacts without a word
		// (#327). cidx reads the git remote of this directory, and now names it.
		if owner, name, err := repo.GetRemoteInfo(); err == nil {
			log.Infof("📦 %s/%s, from the git remote of the working directory", owner, name)
		}

		runID := c.String("run")
		if runID == "" {
			current, err := branch.GetCurrentBranch()
			if err != nil {
				return fmt.Errorf("failed to get current branch: %w", err)
			}
			run, err := provider.GetLatestRunForBranch(context.Background(), current)
			if err != nil {
				return fmt.Errorf("no workflow run found for branch %q, so there are no artifacts to download: %w", current, err)
			}
			runID = run.ID
		}

		action := actions.NewArtifactDownload(provider, runID, c.Args().Slice(), c.String("output"))
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
