package features

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/actions"
	"github.com/cidx-org/cidx/v3/pkg/remote"
	"github.com/cucumber/godog"
)

type postMergeProvider struct {
	remote.Provider
	delayed bool
	oldOnly bool
	lookups int
}

func (p *postMergeProvider) ListRuns(_ context.Context, file, branch string, _ int) ([]remote.Workflow, error) {
	if branch != "trunk" {
		return nil, fmt.Errorf("expected merge target trunk, got %s", branch)
	}
	p.lookups++
	if file == "cidx.yml" || p.oldOnly || (p.delayed && p.lookups <= 2) {
		return []remote.Workflow{{ID: "old", Branch: branch, HeadSHA: "old-sha", Status: "completed", Conclusion: "success"}}, nil
	}
	return []remote.Workflow{
		{ID: "other-branch", Branch: "feature", HeadSHA: "merged-sha"},
		{ID: "merged-ci", Branch: branch, HeadSHA: "merged-sha"},
	}, nil
}

func RegisterPostMergeSteps(ctx *godog.ScenarioContext, tc *TestContext) {
	ctx.Given(`^a completed old cidx workflow and a CI run for the merged commit$`, func() { tc.Config["post_merge_mode"] = "normal" })
	ctx.Given(`^the merged commit workflow appears after the first lookup$`, func() { tc.Config["post_merge_mode"] = "delayed" })
	ctx.Given(`^the merge response has no commit$`, func() { tc.Config["post_merge_mode"] = "missing" })
	ctx.Given(`^only an older successful workflow exists$`, func() { tc.Config["post_merge_mode"] = "old" })
	ctx.When(`^CIDX waits for the post-merge workflow$`, func() error {
		mode := tc.Config["post_merge_mode"]
		p := &postMergeProvider{delayed: mode == "delayed", oldOnly: mode == "old"}
		target := &remote.MergeResult{Branch: "trunk", SHA: "merged-sha"}
		if mode == "missing" {
			target.SHA = ""
		}
		waitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		run, err := actions.WaitForPostMergeWorkflow(waitCtx, p, target, time.Millisecond)
		tc.ExitCode = 0
		tc.Output = ""
		if err != nil {
			tc.ExitCode = 1
			tc.Output = err.Error()
		} else {
			tc.Output = run.ID
		}
		if mode == "delayed" && p.lookups <= 2 {
			return fmt.Errorf("did not retry the lookup")
		}
		return nil
	})
	ctx.Then(`^it selects the CI run for the merged commit$`, func() error {
		if tc.ExitCode != 0 || tc.Output != "merged-ci" {
			return fmt.Errorf("wrong post-merge result: %s", tc.Output)
		}
		return nil
	})
	ctx.Then(`^post-merge verification fails$`, func() error {
		if tc.ExitCode == 0 {
			return fmt.Errorf("unverified merge reported success")
		}
		return nil
	})
}
