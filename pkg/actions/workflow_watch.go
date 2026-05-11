package actions

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/pkg/remote"
	log "github.com/sirupsen/logrus"
)

// WorkflowWatchAction watches a single workflow run on a branch (or by run ID).
// It complements `cidx pr watch` by supporting non-PR branches such as direct
// pushes to main (issue #125).
type WorkflowWatchAction struct {
	provider remote.Provider
	branch   string // resolved branch name; empty if runID is set
	runID    string // explicit run ID; empty if branch lookup is used
	quiet    bool
}

// NewWorkflowWatch creates a new workflow watch action.
//
// Exactly one of branch or runID should be set. When branch is set, the action
// resolves the most recent workflow run on that branch. When runID is set, it
// watches the specified run directly.
func NewWorkflowWatch(provider remote.Provider, branch, runID string, quiet bool) *WorkflowWatchAction {
	return &WorkflowWatchAction{
		provider: provider,
		branch:   branch,
		runID:    runID,
		quiet:    quiet,
	}
}

// Execute resolves the run to watch, then streams updates until completion.
func (a *WorkflowWatchAction) Execute(ctx context.Context) error {
	workflow, err := a.resolveWorkflow(ctx)
	if err != nil {
		return err
	}

	if !a.quiet {
		log.Infof("👀 Watching workflow run %s...", workflow.ID)
		log.Infof("🔗 %s", workflow.URL)
	} else {
		fmt.Printf("Watching workflow run %s\n", workflow.ID)
	}

	// Already completed? Print final status and return.
	if workflow.Status == "completed" {
		return reportCompletion(workflow)
	}

	updates, err := a.provider.WatchWorkflow(ctx, workflow.ID)
	if err != nil {
		return fmt.Errorf("failed to watch workflow: %w", err)
	}

	for update := range updates {
		if update.Error != nil {
			return update.Error
		}

		if !a.quiet {
			DisplayWorkflowStatus(update.Workflow)
		}

		if update.Workflow.Status == "completed" {
			if !a.quiet {
				fmt.Println()
			}
			return reportCompletion(update.Workflow)
		}
	}

	return nil
}

// resolveWorkflow picks the run to watch based on branch or runID.
func (a *WorkflowWatchAction) resolveWorkflow(ctx context.Context) (*remote.Workflow, error) {
	if a.runID != "" {
		workflow, err := a.provider.GetWorkflowRun(ctx, a.runID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch workflow run %s: %w", a.runID, err)
		}
		return workflow, nil
	}

	if a.branch == "" {
		return nil, fmt.Errorf("either a branch or a run ID is required")
	}

	workflow, err := a.provider.GetLatestRunForBranch(ctx, a.branch)
	if err != nil {
		return nil, fmt.Errorf("no workflow run found for branch %q (push a commit or check the branch name): %w", a.branch, err)
	}
	return workflow, nil
}

// reportCompletion prints the final outcome and returns an error on failure.
func reportCompletion(w *remote.Workflow) error {
	switch w.Conclusion {
	case "success", "skipped", "neutral":
		log.Info("🎉 Workflow completed successfully!")
		return nil
	case "":
		// Status is "completed" but no conclusion -- shouldn't happen normally.
		log.Warnf("Workflow completed with empty conclusion (status=%s)", w.Status)
		return nil
	default:
		log.Errorf("❌ Workflow failed: %s", w.Conclusion)
		return fmt.Errorf("workflow failed with conclusion: %s", w.Conclusion)
	}
}
