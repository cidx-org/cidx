package actions

import (
	"context"
	"fmt"

	"github.com/cidx-org/cidx/v2/pkg/remote"
	log "github.com/sirupsen/logrus"
)

// WorkflowListAction lists workflow runs.
//
// It lists a named workflow's runs, and -- with no name -- the runs of every
// workflow on one branch. That second view is what you want when a check has
// just failed and you do not yet know which workflow owns it: the failing check
// names a job, and having to name the workflow file before being allowed to look
// is what sent this question to `gh` (issue #342).
//
// The runs come from the provider rather than from `gh api`, so the listing
// speaks the same client, the same repository resolution and the same identifier
// as the `watch` and `rerun` commands beside it, and works on GitLab.
type WorkflowListAction struct {
	provider remote.Provider
	workflow string // workflow file or name; empty means every workflow
	branch   string // branch to filter on; empty means every branch
	limit    int
	verbose  bool
}

// NewWorkflowList creates a new workflow list action. Exactly one of workflow
// and branch is expected to be set; setting both narrows to that workflow's runs
// on that branch, which is a legitimate, if narrower, question.
func NewWorkflowList(provider remote.Provider, workflow, branch string, limit int, verbose bool) *WorkflowListAction {
	return &WorkflowListAction{
		provider: provider,
		workflow: workflow,
		branch:   branch,
		limit:    limit,
		verbose:  verbose,
	}
}

// Execute lists the runs and displays them.
func (a *WorkflowListAction) Execute(ctx context.Context) error {
	runs, err := a.provider.ListRuns(ctx, a.workflow, a.branch, a.limit)
	if err != nil {
		return fmt.Errorf("failed to get workflow runs: %w", err)
	}

	if len(runs) == 0 {
		log.Infof("No runs found for %s", a.scope())
		if a.workflow != "" {
			log.Info("")
			log.Infof("Note: check that .github/workflows/%s exists and has been triggered.", remote.NormalizeWorkflowFile(a.workflow))
		}
		return nil
	}

	if a.limit > 0 && len(runs) > a.limit {
		runs = runs[:a.limit]
	}

	if a.verbose {
		a.displayVerbose(runs)
	} else {
		a.displaySimple(runs)
	}

	return nil
}

// scope names what was listed, so an empty result says which question it
// answered rather than leaving the reader to guess which filter applied.
func (a *WorkflowListAction) scope() string {
	switch {
	case a.workflow != "" && a.branch != "":
		return fmt.Sprintf("workflow '%s' on branch '%s'", a.workflow, a.branch)
	case a.workflow != "":
		return fmt.Sprintf("workflow '%s'", a.workflow)
	case a.branch != "":
		return fmt.Sprintf("branch '%s'", a.branch)
	default:
		return "this repository"
	}
}

// displaySimple shows runs in a simple list format.
//
// Both numbers are printed, labelled. `#640` is what GitHub shows in its UI, and
// it was for a long time the only thing listed here — but the API addresses a run
// by its ID, so `cidx repo workflow watch 640` sent the run number where an ID
// was expected and got a flat 404. The commands of one namespace have to speak
// the same identifier, and the workaround (polling this list instead of
// watching) defeats the point of having a watch at all (issue #291).
//
// The workflow name comes with them in the branch view: naming which workflow a
// run belongs to is the whole reason that view exists (issue #342).
func (a *WorkflowListAction) displaySimple(runs []remote.Workflow) {
	log.Infof("🔄 Runs for %s (%d):", a.scope(), len(runs))
	log.Info("")

	for _, run := range runs {
		fmt.Printf("  %s #%-4d  %s  %s  id %s", formatRunStatus(run.Conclusion, run.Status),
			run.Number, run.CreatedAt.Format("2006-01-02 15:04"), shortSHA(run.HeadSHA), run.ID)
		if a.workflow == "" && run.Name != "" {
			fmt.Printf("  %s", run.Name)
		}
		fmt.Println()
	}

	printRunHints()
}

// displayVerbose shows runs with additional information
func (a *WorkflowListAction) displayVerbose(runs []remote.Workflow) {
	log.Infof("🔄 Runs for %s (%d):", a.scope(), len(runs))
	log.Info("")

	fmt.Printf("  %-8s %-6s %-12s %-16s %-10s %-12s %-16s %s\n", "STATUS", "RUN", "ID", "DATE", "COMMIT", "BRANCH", "WORKFLOW", "TITLE")
	fmt.Printf("  %-8s %-6s %-12s %-16s %-10s %-12s %-16s %s\n", "------", "---", "--", "----", "------", "------", "--------", "-----")

	for _, run := range runs {
		fmt.Printf("  %-8s #%-5d %-12s %-16s %-10s %-12s %-16s %s\n",
			formatRunStatus(run.Conclusion, run.Status),
			run.Number,
			run.ID,
			run.CreatedAt.Format("2006-01-02 15:04"),
			shortSHA(run.HeadSHA),
			clip(run.Branch, 12),
			clip(run.Name, 16),
			clip(run.Title, 35),
		)
	}

	printRunHints()
}

// printRunHints names the identifier the neighbouring commands take, and takes
// it from the column right next to it. The hint used to send the reader to
// `gh run view <run-number>` — the wrong tool for a repository that dogfoods its
// own, and the wrong number for the command it named (issue #291).
func printRunHints() {
	fmt.Println()
	fmt.Println("  Follow one with:   cidx repo workflow watch <id>")
	fmt.Println("  Restart one with:  cidx repo workflow rerun --failed <id>")
	fmt.Println("  Fetch its output:  cidx repo artifact download --run <id>")
}

// shortSHA abbreviates a commit to the seven characters git prints.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// clip keeps a column from overflowing. The run ID is deliberately never clipped
// — an eleven-digit identifier cut short is a number the API does not know, the
// failure #291 is about, restored by formatting.
func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// formatRunStatus returns a formatted status string with emoji
func formatRunStatus(conclusion, status string) string {
	if status == "in_progress" || status == "queued" {
		return "🔄 run"
	}

	switch conclusion {
	case "success":
		return "✅ ok"
	case "failure":
		return "❌ fail"
	case "cancelled":
		return "⏹️  stop"
	case "skipped":
		return "⏭️  skip"
	default:
		return "❓ " + conclusion
	}
}
