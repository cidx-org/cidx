package actions

import (
	"context"
	"fmt"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/remote"
)

// WaitForPostMergeWorkflow waits for a CI run of the commit returned by the
// merge API. An old run of a preferred workflow must not hide a matching run
// of another candidate. Missing runs are retried; API failures are not success.
func WaitForPostMergeWorkflow(ctx context.Context, provider remote.Provider, merged *remote.MergeResult, interval time.Duration) (*remote.Workflow, error) {
	if merged == nil || merged.SHA == "" || merged.Branch == "" {
		return nil, fmt.Errorf("merge response did not identify the target branch and commit")
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for CI on %s at %s: %w", merged.Branch, merged.SHA, err)
		}
		for _, file := range remote.CandidateWorkflowFiles {
			runs, err := provider.ListRuns(ctx, file, merged.Branch, 100)
			if err != nil {
				return nil, fmt.Errorf("list post-merge runs: %w", err)
			}
			for _, run := range runs {
				if run.HeadSHA == merged.SHA && run.Branch == merged.Branch {
					return &run, nil
				}
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("waiting for CI on %s at %s: %w", merged.Branch, merged.SHA, ctx.Err())
		case <-timer.C:
		}
	}
}
