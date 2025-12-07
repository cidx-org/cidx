package actions

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// WorkflowListAction lists workflow runs from GitHub Actions
type WorkflowListAction struct {
	workflow string
	limit    int
	verbose  bool
}

// WorkflowRun represents a GitHub Actions workflow run
type WorkflowRun struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	HeadBranch   string    `json:"head_branch"`
	HeadSha      string    `json:"head_sha"`
	Status       string    `json:"status"`
	Conclusion   string    `json:"conclusion"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	HTMLURL      string    `json:"html_url"`
	RunNumber    int       `json:"run_number"`
	DisplayTitle string    `json:"display_title"`
}

// WorkflowRunsResponse represents the GitHub API response for workflow runs
type WorkflowRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

// NewWorkflowList creates a new workflow list action
func NewWorkflowList(workflow string, limit int, verbose bool) *WorkflowListAction {
	return &WorkflowListAction{
		workflow: workflow,
		limit:    limit,
		verbose:  verbose,
	}
}

// Execute lists workflow runs from GitHub Actions
func (a *WorkflowListAction) Execute(ctx context.Context) error {
	// Check if gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found. Install from https://cli.github.com")
	}

	// Get workflow runs
	runs, err := a.getWorkflowRuns()
	if err != nil {
		return fmt.Errorf("failed to get workflow runs: %w", err)
	}

	if len(runs) == 0 {
		log.Infof("No runs found for workflow '%s'", a.workflow)
		log.Info("")
		log.Infof("Note: Check that .github/workflows/%s.yml exists and has been triggered.", a.workflow)
		return nil
	}

	// Apply limit
	if a.limit > 0 && len(runs) > a.limit {
		runs = runs[:a.limit]
	}

	// Display runs
	if a.verbose {
		a.displayVerbose(runs)
	} else {
		a.displaySimple(runs)
	}

	return nil
}

// getWorkflowRuns fetches workflow runs from GitHub
func (a *WorkflowListAction) getWorkflowRuns() ([]WorkflowRun, error) {
	// Use gh CLI to get workflow runs
	workflowFile := a.workflow
	if !strings.HasSuffix(workflowFile, ".yml") && !strings.HasSuffix(workflowFile, ".yaml") {
		workflowFile = a.workflow + ".yml"
	}

	args := []string{
		"api",
		fmt.Sprintf("repos/{owner}/{repo}/actions/workflows/%s/runs", workflowFile),
		"--jq", ".",
	}

	cmd := exec.Command("gh", args...)
	output, err := cmd.Output()
	if err != nil {
		// Check if workflow doesn't exist yet
		if strings.Contains(string(output), "Not Found") {
			return nil, nil
		}
		return nil, fmt.Errorf("gh api failed: %w", err)
	}

	var response WorkflowRunsResponse
	if err := json.Unmarshal(output, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.WorkflowRuns, nil
}

// displaySimple shows runs in a simple list format
func (a *WorkflowListAction) displaySimple(runs []WorkflowRun) {
	log.Infof("🔄 Workflow '%s' runs (%d):", a.workflow, len(runs))
	log.Info("")

	for _, run := range runs {
		status := a.formatStatus(run.Conclusion, run.Status)
		date := run.CreatedAt.Format("2006-01-02 15:04")
		shortSha := run.HeadSha
		if len(shortSha) > 7 {
			shortSha = shortSha[:7]
		}

		fmt.Printf("  %s #%-4d  %s  %s\n", status, run.RunNumber, date, shortSha)
	}
}

// displayVerbose shows runs with additional information
func (a *WorkflowListAction) displayVerbose(runs []WorkflowRun) {
	log.Infof("🔄 Workflow '%s' runs (%d):", a.workflow, len(runs))
	log.Info("")

	// Header
	fmt.Printf("  %-8s %-6s %-16s %-10s %-12s %s\n", "STATUS", "RUN", "DATE", "COMMIT", "BRANCH", "TITLE")
	fmt.Printf("  %-8s %-6s %-16s %-10s %-12s %s\n", "------", "---", "----", "------", "------", "-----")

	for _, run := range runs {
		status := a.formatStatus(run.Conclusion, run.Status)
		date := run.CreatedAt.Format("2006-01-02 15:04")
		shortSha := run.HeadSha
		if len(shortSha) > 7 {
			shortSha = shortSha[:7]
		}

		branch := run.HeadBranch
		if len(branch) > 12 {
			branch = branch[:9] + "..."
		}

		title := run.DisplayTitle
		if len(title) > 35 {
			title = title[:32] + "..."
		}

		fmt.Printf("  %-8s #%-5d %-16s %-10s %-12s %s\n", status, run.RunNumber, date, shortSha, branch, title)
	}

	log.Info("")
	log.Info("  Use 'gh run view <run-number>' for details")
}

// formatStatus returns a formatted status string with emoji
func (a *WorkflowListAction) formatStatus(conclusion, status string) string {
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
