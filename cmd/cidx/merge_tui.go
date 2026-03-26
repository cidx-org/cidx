package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cidx-org/cidx/internal/tui"
	"github.com/cidx-org/cidx/pkg/config"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/remote/github"
)

// Merge TUI styles - aliased from shared tui package
var (
	mergeTitleStyle          = tui.Title
	mergeBoxStyle            = tui.Box
	mergeActiveBoxStyle      = tui.ActiveBox
	mergeLabelStyle          = tui.Label
	mergeDimStyle            = tui.Dim
	mergeSuccessStyle        = tui.SuccessBold
	mergeErrorStyle          = tui.ErrorBold
	mergeWarningStyle        = tui.WarningBold
	mergeSelectedStyle       = tui.Selected
	mergeUnselectedStyle     = tui.Unselected
	mergeReviewApprovedStyle = tui.Success
	mergeReviewChangesStyle  = tui.Warning
	mergeReviewPendingStyle  = tui.Dim
	mergeHelpStyle           = tui.Help
	mergeCheckSuccessStyle   = tui.Success
	mergeCheckFailureStyle   = tui.Error
	mergeCheckPendingStyle   = tui.Warning
)

// mergeFocus determines which section is focused
type mergeFocus int

const (
	focusMergeMethod mergeFocus = iota
	focusMergeMessage
	focusMergeActions
)

// Merge methods
var mergeMethods = []string{"squash", "merge", "rebase"}

// Merge actions
var mergeActions = []string{"Merge", "Cancel"}

// mergeModel is the TUI model for PR merge
type mergeModel struct {
	provider  *github.Client
	prNumber  int
	prDetails *remote.PullRequestDetails
	checks    *remote.PRChecks
	prConfig  config.PRConfig

	// Editable fields
	mergeMethod int // index into mergeMethods
	commitTitle textarea.Model
	commitBody  textarea.Model
	editingBody bool // whether we're editing body vs title

	// UI state
	focus            mergeFocus
	actionCursor     int
	loading          bool
	loadingMsg       string
	spinner          spinner.Model
	err              error
	success          string
	width            int
	height           int
	showConfirmation     bool   // Show merge confirmation dialog
	showQuitConfirmation bool   // Show quit confirmation after merge complete
	postMergeStatus      string // Status message for post-merge operations

	// Watch mode state
	lastKnownSHA  string // Track SHA to detect new commits
	autoRefresh   bool   // Whether auto-refresh is enabled
	refreshing    bool   // Currently refreshing
	showErrorLogs bool   // Whether to show expanded error logs

	// Post-merge pipeline monitoring
	merged            bool               // PR has been merged
	mainPipelineCheck *remote.PRChecks   // Pipeline status on main after merge
	pipelineComplete  bool               // All pipelines finished
}

// Messages
type prDetailsLoadedMsg struct {
	details *remote.PullRequestDetails
	checks  *remote.PRChecks
}

type prMergeErrorMsg struct {
	err error
}

type prMergeSuccessMsg struct {
	message string
}

type checksRefreshMsg struct {
	checks  *remote.PRChecks
	details *remote.PullRequestDetails // Optional: nil if only refreshing checks
}

type mergeTickMsg struct{}

type postMergeMsg struct {
	status string
	err    error
}

type mainPipelineMsg struct {
	checks *remote.PRChecks
	err    error
}

// newMergeModel creates a new merge TUI model
func newMergeModel(provider *github.Client, prNumber int, prConfig config.PRConfig) mergeModel {
	// Create textarea for commit title
	titleInput := textarea.New()
	titleInput.Placeholder = "Commit title..."
	titleInput.SetHeight(1)
	titleInput.SetWidth(70)
	titleInput.CharLimit = 200
	titleInput.ShowLineNumbers = false

	// Create textarea for commit body
	bodyInput := textarea.New()
	bodyInput.Placeholder = "Commit body (optional)..."
	bodyInput.SetHeight(5)
	bodyInput.SetWidth(70)
	bodyInput.ShowLineNumbers = false

	// Create spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))

	// Determine default merge method from config
	defaultMethod := 0 // squash
	switch prConfig.GetDefaultMergeMethod() {
	case "merge":
		defaultMethod = 1
	case "rebase":
		defaultMethod = 2
	}

	return mergeModel{
		provider:     provider,
		prNumber:     prNumber,
		prConfig:     prConfig,
		mergeMethod:  defaultMethod,
		commitTitle:  titleInput,
		commitBody:   bodyInput,
		focus:        focusMergeMethod,
		actionCursor: 0,
		loading:      true,
		loadingMsg:   "Loading PR details...",
		spinner:      s,
		autoRefresh:  prConfig.AutoRefreshInterval > 0,
	}
}

// mergeTickCmd returns a command that ticks at the configured interval for merge TUI
func mergeTickCmd(intervalSeconds int) tea.Cmd {
	if intervalSeconds <= 0 {
		intervalSeconds = 5
	}
	return tea.Tick(time.Duration(intervalSeconds)*time.Second, func(t time.Time) tea.Msg {
		return mergeTickMsg{}
	})
}

func (m mergeModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.spinner.Tick,
		m.loadPRDetails(),
	}
	if m.autoRefresh {
		cmds = append(cmds, mergeTickCmd(m.prConfig.AutoRefreshInterval))
	}
	return tea.Batch(cmds...)
}

func (m mergeModel) loadPRDetails() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get PR details
		details, err := m.provider.GetPullRequestDetails(ctx, m.prNumber)
		if err != nil {
			return prMergeErrorMsg{err: err}
		}

		// Get checks status
		checks, err := m.provider.GetPullRequestChecks(ctx, m.prNumber)
		if err != nil {
			// Non-fatal, just won't show checks
			checks = nil
		}

		return prDetailsLoadedMsg{details: details, checks: checks}
	}
}

// refreshAll reloads both PR details and checks (for when SHA changes)
func (m mergeModel) refreshAll() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get PR details
		details, err := m.provider.GetPullRequestDetails(ctx, m.prNumber)
		if err != nil {
			return checksRefreshMsg{checks: nil, details: nil}
		}

		// Get checks status
		checks, _ := m.provider.GetPullRequestChecks(ctx, m.prNumber)

		return checksRefreshMsg{checks: checks, details: details}
	}
}

func (m mergeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle quit
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		// Handle refresh even during loading (but not during initial load)
		if msg.String() == "r" && m.prDetails != nil && !m.refreshing {
			m.refreshing = true
			return m, m.refreshAll()
		}

		// Toggle error logs with 'e'
		if msg.String() == "e" && m.checks != nil {
			m.showErrorLogs = !m.showErrorLogs
			return m, nil
		}

		// Toggle auto-refresh with 'a'
		if msg.String() == "a" {
			m.autoRefresh = !m.autoRefresh
			if m.autoRefresh {
				cmds = append(cmds, mergeTickCmd(m.prConfig.AutoRefreshInterval))
			}
			return m, tea.Batch(cmds...)
		}

		// Don't handle other keys while loading
		if m.loading {
			return m, nil
		}

		// Handle error state
		if m.err != nil {
			if msg.String() == "enter" || msg.String() == "esc" {
				return m, tea.Quit
			}
			return m, nil
		}

		// Handle post-merge state
		if m.merged {
			switch msg.String() {
			case "q", "Q":
				return m, tea.Quit
			case "enter":
				if m.showQuitConfirmation {
					// Stay - dismiss the quit confirmation
					m.showQuitConfirmation = false
					return m, nil
				}
				if m.pipelineComplete && !m.prConfig.ConfirmQuitAfterMerge {
					return m, tea.Quit
				}
			case "esc":
				if m.showQuitConfirmation {
					m.showQuitConfirmation = false
					return m, nil
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "tab", "shift+tab":
			// Cycle through focus areas
			if msg.String() == "tab" {
				m.focus = (m.focus + 1) % 3
			} else {
				m.focus = (m.focus + 2) % 3
			}
			// Update textarea focus
			m.commitTitle.Blur()
			m.commitBody.Blur()
			if m.focus == focusMergeMessage {
				if m.editingBody {
					m.commitBody.Focus()
				} else {
					m.commitTitle.Focus()
				}
			}

		case "up", "k":
			switch m.focus {
			case focusMergeMethod:
				if m.mergeMethod > 0 {
					m.mergeMethod--
					m.updateCommitMessage()
				}
			case focusMergeMessage:
				if m.editingBody {
					m.editingBody = false
					m.commitBody.Blur()
					m.commitTitle.Focus()
				}
			case focusMergeActions:
				if m.actionCursor > 0 {
					m.actionCursor--
				}
			}

		case "down", "j":
			switch m.focus {
			case focusMergeMethod:
				if m.mergeMethod < len(mergeMethods)-1 {
					m.mergeMethod++
					m.updateCommitMessage()
				}
			case focusMergeMessage:
				if !m.editingBody {
					m.editingBody = true
					m.commitTitle.Blur()
					m.commitBody.Focus()
				}
			case focusMergeActions:
				if m.actionCursor < len(mergeActions)-1 {
					m.actionCursor++
				}
			}

		case "enter":
			// If showing confirmation dialog, handle y/n
			if m.showConfirmation {
				// Enter in confirmation = confirm merge
				m.showConfirmation = false
				m.loading = true
				m.loadingMsg = "Merging pull request..."
				return m, m.doMerge()
			}

			switch m.focus {
			case focusMergeActions:
				action := mergeActions[m.actionCursor]
				if action == "Merge" && m.prConfig.ConfirmMerge {
					// Show confirmation dialog
					m.showConfirmation = true
					return m, nil
				}
				return m, m.executeAction()
			case focusMergeMethod:
				// Confirm method selection, move to message
				m.focus = focusMergeMessage
				m.commitTitle.Focus()
			}

		case "y", "Y":
			// Confirm merge when in confirmation dialog
			if m.showConfirmation {
				m.showConfirmation = false
				m.loading = true
				m.loadingMsg = "Merging pull request..."
				return m, m.doMerge()
			}

		case "n", "N", "esc":
			// Cancel confirmation dialog
			if m.showConfirmation {
				m.showConfirmation = false
				return m, nil
			}
		}

	case mergeTickMsg:
		// Post-merge pipeline monitoring
		if m.merged && m.prConfig.WatchPipelineAfterMerge && !m.pipelineComplete && !m.refreshing {
			m.refreshing = true
			cmds = append(cmds, m.fetchMainPipeline())
			cmds = append(cmds, mergeTickCmd(m.prConfig.AutoRefreshInterval))
			return m, tea.Batch(cmds...)
		}
		// Pre-merge auto-refresh ticker
		if m.autoRefresh && m.prDetails != nil && !m.refreshing && m.success == "" && m.err == nil && !m.showConfirmation && !m.merged {
			m.refreshing = true
			cmds = append(cmds, m.refreshAll())
		}
		// Continue ticking if auto-refresh is enabled and not merged
		if m.autoRefresh && m.success == "" && m.err == nil && !m.merged {
			cmds = append(cmds, mergeTickCmd(m.prConfig.AutoRefreshInterval))
		}
		return m, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.commitTitle.SetWidth(min(msg.Width-20, 70))
		m.commitBody.SetWidth(min(msg.Width-20, 70))
		m.commitBody.SetHeight(min(msg.Height/4, 8))

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case prDetailsLoadedMsg:
		m.loading = false
		m.prDetails = msg.details
		m.checks = msg.checks
		m.updateCommitMessage()

	case checksRefreshMsg:
		m.refreshing = false
		if msg.checks != nil {
			// Check if SHA changed (new commit pushed)
			if m.lastKnownSHA != "" && m.checks != nil && msg.checks.HeadSHA != m.lastKnownSHA {
				// New commit detected - clear old status
				m.lastKnownSHA = msg.checks.HeadSHA
			}
			m.checks = msg.checks
			if m.checks.HeadSHA != "" {
				m.lastKnownSHA = m.checks.HeadSHA
			}
		}
		// Update PR details if provided (full refresh)
		if msg.details != nil {
			m.prDetails = msg.details
			m.updateCommitMessage()
		}

	case prMergeErrorMsg:
		m.loading = false
		m.err = msg.err

	case prMergeSuccessMsg:
		m.loading = false
		m.merged = true
		m.success = msg.message
		// Run post-merge actions if configured
		if m.prConfig.CheckoutAfterMerge || m.prConfig.DeleteBranchAfterMerge {
			m.loading = true
			m.loadingMsg = "Running post-merge actions..."
			return m, m.runPostMergeActions()
		}
		// If no post-merge actions, start watching pipeline directly
		if m.prConfig.WatchPipelineAfterMerge {
			// Clear success to return to main screen in "merged" mode
			m.success = ""
			return m, tea.Batch(m.fetchMainPipeline(), mergeTickCmd(m.prConfig.AutoRefreshInterval))
		}
		// No watching, show quit confirmation or exit
		if m.prConfig.ConfirmQuitAfterMerge {
			m.pipelineComplete = true
			m.showQuitConfirmation = true
		}

	case postMergeMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		if msg.status != "" {
			m.postMergeStatus = msg.status
		}
		// After post-merge actions, start watching pipeline if configured
		if m.prConfig.WatchPipelineAfterMerge {
			// Clear success to return to main screen in "merged" mode
			m.success = ""
			return m, tea.Batch(m.fetchMainPipeline(), mergeTickCmd(m.prConfig.AutoRefreshInterval))
		}
		// No watching, show quit confirmation or exit
		if m.prConfig.ConfirmQuitAfterMerge {
			m.pipelineComplete = true
			m.showQuitConfirmation = true
		}

	case mainPipelineMsg:
		m.loading = false
		m.refreshing = false
		if msg.err != nil {
			// Pipeline fetch error - don't fail, just show status
			m.postMergeStatus += fmt.Sprintf("\n⚠ Could not fetch pipeline status: %v", msg.err)
			m.pipelineComplete = true
			if m.prConfig.ConfirmQuitAfterMerge {
				m.showQuitConfirmation = true
			}
			return m, nil
		}
		m.mainPipelineCheck = msg.checks
		// Check if all checks are complete
		if msg.checks != nil && msg.checks.Pending == 0 && msg.checks.Queued == 0 && msg.checks.InProgress == 0 {
			m.pipelineComplete = true
			if m.prConfig.ConfirmQuitAfterMerge {
				m.showQuitConfirmation = true
			}
		}
	}

	// Update focused textarea
	switch m.focus {
	case focusMergeMessage:
		var cmd tea.Cmd
		if m.editingBody {
			m.commitBody, cmd = m.commitBody.Update(msg)
		} else {
			m.commitTitle, cmd = m.commitTitle.Update(msg)
		}
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *mergeModel) updateCommitMessage() {
	if m.prDetails == nil {
		return
	}

	method := mergeMethods[m.mergeMethod]
	switch method {
	case "squash":
		// For squash: PR title as commit title, PR body + commits as body
		m.commitTitle.SetValue(fmt.Sprintf("%s (#%d)", m.prDetails.Title, m.prDetails.Number))

		var body strings.Builder
		if m.prDetails.Body != "" {
			body.WriteString(m.prDetails.Body)
			body.WriteString("\n\n")
		}
		// Add commit list
		if len(m.prDetails.Commits) > 0 {
			body.WriteString("Commits:\n")
			for _, c := range m.prDetails.Commits {
				body.WriteString(fmt.Sprintf("* %s %s\n", c.SHA, c.Message))
			}
		}
		m.commitBody.SetValue(body.String())

	case "merge":
		// For merge: standard merge commit message
		m.commitTitle.SetValue(fmt.Sprintf("Merge pull request #%d from %s", m.prDetails.Number, m.prDetails.HeadBranch))
		m.commitBody.SetValue(m.prDetails.Title)

	case "rebase":
		// For rebase: no custom message needed
		m.commitTitle.SetValue("(Rebase uses original commit messages)")
		m.commitBody.SetValue("")
	}
}

func (m mergeModel) executeAction() tea.Cmd {
	action := mergeActions[m.actionCursor]

	switch action {
	case "Cancel":
		return tea.Quit

	case "Merge":
		// If confirmation is required and not shown yet, show it
		if m.prConfig.ConfirmMerge && !m.showConfirmation {
			return nil // Will be handled by setting showConfirmation = true
		}
		return m.doMerge()
	}

	return nil
}

func (m mergeModel) doMerge() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		method := mergeMethods[m.mergeMethod]

		// Perform the merge
		err := m.provider.MergePullRequest(ctx, m.prNumber, method)
		if err != nil {
			return prMergeErrorMsg{err: err}
		}

		return prMergeSuccessMsg{
			message: fmt.Sprintf("Successfully merged PR #%d using %s method", m.prNumber, method),
		}
	}
}

// runPostMergeActions executes post-merge operations based on config
func (m mergeModel) runPostMergeActions() tea.Cmd {
	return func() tea.Msg {
		var statusMessages []string

		// Delete branch after merge (if configured and not protected)
		if m.prConfig.DeleteBranchAfterMerge && m.prDetails != nil {
			headBranch := m.prDetails.HeadBranch

			// Delete remote branch using git
			cmd := exec.Command("git", "push", "origin", "--delete", headBranch)
			if err := cmd.Run(); err != nil {
				statusMessages = append(statusMessages, fmt.Sprintf("⚠ Failed to delete remote branch: %v", err))
			} else {
				statusMessages = append(statusMessages, fmt.Sprintf("✓ Deleted remote branch: %s", headBranch))
			}

			// Delete local branch
			cmd = exec.Command("git", "branch", "-D", headBranch)
			if err := cmd.Run(); err != nil {
				// Not critical, branch might not exist locally
				statusMessages = append(statusMessages, fmt.Sprintf("⚠ Local branch not deleted: %s", headBranch))
			} else {
				statusMessages = append(statusMessages, fmt.Sprintf("✓ Deleted local branch: %s", headBranch))
			}
		}

		// Checkout main branch (if configured)
		if m.prConfig.CheckoutAfterMerge {
			mainBranch := "main" // Could be configurable from release config
			cmd := exec.Command("git", "checkout", mainBranch)
			if err := cmd.Run(); err != nil {
				return postMergeMsg{
					status: strings.Join(statusMessages, "\n"),
					err:    fmt.Errorf("failed to checkout %s: %w", mainBranch, err),
				}
			}
			statusMessages = append(statusMessages, fmt.Sprintf("✓ Switched to branch: %s", mainBranch))

			// Sync with remote (if configured)
			if m.prConfig.SyncAfterMerge {
				cmd = exec.Command("git", "pull", "--ff-only")
				if err := cmd.Run(); err != nil {
					statusMessages = append(statusMessages, fmt.Sprintf("⚠ Failed to sync: %v", err))
				} else {
					statusMessages = append(statusMessages, "✓ Synced with remote")
				}
			}
		}

		return postMergeMsg{
			status: strings.Join(statusMessages, "\n"),
			err:    nil,
		}
	}
}

// fetchMainPipeline fetches the latest workflow/pipeline status for the main branch
func (m mergeModel) fetchMainPipeline() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get the latest workflow on main branch
		workflow, err := m.provider.GetLatestWorkflow(ctx, "main")
		if err != nil {
			return mainPipelineMsg{err: err}
		}

		if workflow == nil {
			// No workflow running, consider it complete
			return mainPipelineMsg{
				checks: &remote.PRChecks{
					Status:     "success",
					TotalCount: 0,
				},
			}
		}

		// Convert workflow to PRChecks format for consistent display
		checks := &remote.PRChecks{
			TotalCount: len(workflow.Jobs),
			HeadSHA:    workflow.ID,
		}

		for _, job := range workflow.Jobs {
			switch job.Status {
			case "queued":
				checks.Queued++
			case "in_progress":
				checks.InProgress++
			case "completed":
				if job.Conclusion == "success" {
					checks.Success++
				} else {
					checks.Failure++
				}
			}

			checks.Checks = append(checks.Checks, remote.CheckRun{
				Name:       job.Name,
				Status:     job.Status,
				Conclusion: job.Conclusion,
			})
		}

		// Determine overall status
		if checks.Failure > 0 {
			checks.Status = "failure"
		} else if checks.Queued > 0 || checks.InProgress > 0 {
			checks.Status = "pending"
		} else {
			checks.Status = "success"
		}

		return mainPipelineMsg{checks: checks}
	}
}

func (m mergeModel) View() string {
	if m.loading {
		return fmt.Sprintf("\n  %s %s\n", m.spinner.View(), m.loadingMsg)
	}

	if m.err != nil {
		return fmt.Sprintf("\n  %s %v\n\n  Press Enter to exit.\n",
			mergeErrorStyle.Render("Error:"), m.err)
	}

	// Show quit confirmation dialog
	if m.showQuitConfirmation {
		return m.renderQuitConfirmation()
	}

	// Show confirmation dialog
	if m.showConfirmation {
		return m.renderConfirmation()
	}

	if m.prDetails == nil {
		return "\n  No PR details available.\n"
	}

	// Calculate box width (terminal width minus margins, capped)
	boxWidth := m.width - 4
	if boxWidth < 60 {
		boxWidth = 60
	}
	if boxWidth > 100 {
		boxWidth = 100
	}

	var b strings.Builder

	// Title - different when merged
	var title string
	if m.merged {
		title = fmt.Sprintf("✓ PR #%d MERGED - Watching main pipeline", m.prNumber)
		b.WriteString(mergeSuccessStyle.Render(title))
	} else {
		title = fmt.Sprintf("🔀 Merge PR #%d", m.prNumber)
		b.WriteString(mergeTitleStyle.Render(title))
	}
	b.WriteString("\n\n")

	// Tree view: Issue → PR relationship
	b.WriteString(m.renderTreeView(boxWidth))
	b.WriteString("\n")

	// CI Status section
	b.WriteString(m.renderChecks(boxWidth))
	b.WriteString("\n")

	// In merged mode, show post-merge status instead of merge controls
	if m.merged {
		// Show post-merge action status
		if m.postMergeStatus != "" {
			var statusBox strings.Builder
			statusBox.WriteString(mergeLabelStyle.Render("📋 Post-merge actions"))
			statusBox.WriteString("\n")
			for _, line := range strings.Split(m.postMergeStatus, "\n") {
				if line != "" {
					statusBox.WriteString(fmt.Sprintf("  %s\n", line))
				}
			}
			b.WriteString(mergeBoxStyle.Width(boxWidth).Render(statusBox.String()))
			b.WriteString("\n")
		}

		// Help for merged mode
		var help string
		if m.pipelineComplete {
			help = "q: quit • r: refresh"
		} else {
			help = "Watching pipeline... • q: quit • r: refresh"
		}
		b.WriteString(mergeHelpStyle.Render(help))
	} else {
		// Normal mode: show merge controls

		// Merge Method selection
		b.WriteString(m.renderMergeMethod(boxWidth))
		b.WriteString("\n")

		// Commit Message editing
		if mergeMethods[m.mergeMethod] != "rebase" {
			b.WriteString(m.renderCommitMessage(boxWidth))
			b.WriteString("\n")
		}

		// Actions
		b.WriteString(m.renderActions(boxWidth))
		b.WriteString("\n")

		// Help
		help := "Tab: switch section • ↑↓: navigate • Enter: confirm • r: refresh • a: auto-refresh • e: errors • q: quit"
		b.WriteString(mergeHelpStyle.Render(help))
	}

	return b.String()
}

// renderTreeView creates a tree representation showing Issue → PR relationship
func (m mergeModel) renderTreeView(width int) string {
	var b strings.Builder

	// Tree characters
	const (
		treeRoot   = "📦"
		treeBranch = "├──"
		treeLast   = "└──"
		treeVert   = "│  "
		treeSpace  = "   "
	)

	// If we have linked issues, show Issue → PR tree
	if len(m.prDetails.LinkedIssues) > 0 {
		// For each linked issue, create a tree
		for i, issue := range m.prDetails.LinkedIssues {
			// Issue root
			issueIcon := "🟢"
			if issue.State == "closed" {
				issueIcon = "🟣"
			}
			b.WriteString(fmt.Sprintf("%s Issue #%d\n", issueIcon, issue.Number))

			// Issue details
			b.WriteString(fmt.Sprintf("%s %s\n", treeVert, mergeLabelStyle.Render(issue.Title)))

			// Issue body (truncated)
			if issue.Body != "" {
				bodyPreview := truncateStr(strings.ReplaceAll(issue.Body, "\n", " "), 60)
				b.WriteString(fmt.Sprintf("%s %s\n", treeVert, mergeDimStyle.Render(bodyPreview)))
			}

			// Issue metadata
			if len(issue.Labels) > 0 {
				labelsStr := strings.Join(issue.Labels, ", ")
				b.WriteString(fmt.Sprintf("%s 🏷 %s\n", treeVert, mergeDimStyle.Render(labelsStr)))
			}
			if issue.Author != "" {
				b.WriteString(fmt.Sprintf("%s 👤 @%s\n", treeVert, mergeDimStyle.Render(issue.Author)))
			}

			// PR as child of issue
			b.WriteString(fmt.Sprintf("%s\n", treeVert))
			b.WriteString(fmt.Sprintf("%s 🔀 PR #%d\n", treeLast, m.prDetails.Number))

			// PR details (indented under the issue)
			prIndent := treeSpace
			b.WriteString(fmt.Sprintf("%s%s %s\n", prIndent, treeBranch, mergeLabelStyle.Render(m.prDetails.Title)))

			// Branch info
			branchLine := fmt.Sprintf("%s → %s", m.prDetails.HeadBranch, m.prDetails.BaseBranch)
			b.WriteString(fmt.Sprintf("%s%s 🌿 %s\n", prIndent, treeVert, mergeDimStyle.Render(branchLine)))

			// Stats
			statsLine := fmt.Sprintf("+%d -%d • %d files", m.prDetails.Additions, m.prDetails.Deletions, m.prDetails.ChangedFiles)
			b.WriteString(fmt.Sprintf("%s%s 📊 %s\n", prIndent, treeVert, mergeDimStyle.Render(statsLine)))

			// Author
			b.WriteString(fmt.Sprintf("%s%s 👤 @%s\n", prIndent, treeVert, mergeDimStyle.Render(m.prDetails.Author)))

			// Reviews
			if len(m.prDetails.Reviewers) > 0 {
				b.WriteString(fmt.Sprintf("%s%s 👥 Reviews:\n", prIndent, treeVert))
				for j, reviewer := range m.prDetails.Reviewers {
					prefix := treeVert + treeVert
					if j == len(m.prDetails.Reviewers)-1 {
						prefix = treeVert + treeSpace
					}
					reviewIcon := "○"
					var reviewStyle lipgloss.Style
					switch reviewer.State {
					case "APPROVED":
						reviewIcon = "✓"
						reviewStyle = mergeReviewApprovedStyle
					case "CHANGES_REQUESTED":
						reviewIcon = "✗"
						reviewStyle = mergeReviewChangesStyle
					default:
						reviewStyle = mergeReviewPendingStyle
					}
					reviewLine := fmt.Sprintf("%s @%s (%s)", reviewIcon, reviewer.Login, reviewer.State)
					b.WriteString(fmt.Sprintf("%s%s    %s\n", prIndent, prefix, reviewStyle.Render(reviewLine)))
				}
			}

			// Commits
			if len(m.prDetails.Commits) > 0 {
				b.WriteString(fmt.Sprintf("%s%s 📝 Commits (%d):\n", prIndent, treeVert, len(m.prDetails.Commits)))
				// Show last 3 commits
				start := 0
				if len(m.prDetails.Commits) > 3 {
					start = len(m.prDetails.Commits) - 3
					b.WriteString(fmt.Sprintf("%s%s    %s\n", prIndent, treeVert, mergeDimStyle.Render(fmt.Sprintf("... %d earlier commits", start))))
				}
				for j := start; j < len(m.prDetails.Commits); j++ {
					c := m.prDetails.Commits[j]
					isLast := j == len(m.prDetails.Commits)-1
					prefix := treeVert
					if isLast {
						prefix = treeSpace
					}
					commitLine := fmt.Sprintf("%s %s", c.SHA[:7], truncateStr(c.Message, 40))
					b.WriteString(fmt.Sprintf("%s%s    %s\n", prIndent, prefix, mergeDimStyle.Render(commitLine)))
				}
			}

			// Mergeable status
			if m.prDetails.Mergeable {
				b.WriteString(fmt.Sprintf("%s%s %s\n", prIndent, treeLast, mergeSuccessStyle.Render("✓ Ready to merge")))
			} else {
				b.WriteString(fmt.Sprintf("%s%s %s\n", prIndent, treeLast, mergeWarningStyle.Render("⚠ Not mergeable")))
			}

			// Add separator between multiple issues
			if i < len(m.prDetails.LinkedIssues)-1 {
				b.WriteString("\n")
			}
		}
	} else {
		// No linked issues - show PR only
		b.WriteString(fmt.Sprintf("🔀 PR #%d\n", m.prDetails.Number))
		b.WriteString(fmt.Sprintf("%s %s\n", treeBranch, mergeLabelStyle.Render(m.prDetails.Title)))

		// Branch info
		branchLine := fmt.Sprintf("%s → %s", m.prDetails.HeadBranch, m.prDetails.BaseBranch)
		b.WriteString(fmt.Sprintf("%s 🌿 %s\n", treeVert, mergeDimStyle.Render(branchLine)))

		// Stats
		statsLine := fmt.Sprintf("+%d -%d • %d files", m.prDetails.Additions, m.prDetails.Deletions, m.prDetails.ChangedFiles)
		b.WriteString(fmt.Sprintf("%s 📊 %s\n", treeVert, mergeDimStyle.Render(statsLine)))

		// Author
		b.WriteString(fmt.Sprintf("%s 👤 @%s\n", treeVert, mergeDimStyle.Render(m.prDetails.Author)))

		// Labels
		if len(m.prDetails.Labels) > 0 {
			labelsLine := strings.Join(m.prDetails.Labels, ", ")
			b.WriteString(fmt.Sprintf("%s 🏷 %s\n", treeVert, mergeDimStyle.Render(labelsLine)))
		}

		// Reviews
		if len(m.prDetails.Reviewers) > 0 {
			b.WriteString(fmt.Sprintf("%s 👥 Reviews:\n", treeVert))
			for i, reviewer := range m.prDetails.Reviewers {
				prefix := treeVert
				if i == len(m.prDetails.Reviewers)-1 {
					prefix = treeSpace
				}
				reviewIcon := "○"
				var reviewStyle lipgloss.Style
				switch reviewer.State {
				case "APPROVED":
					reviewIcon = "✓"
					reviewStyle = mergeReviewApprovedStyle
				case "CHANGES_REQUESTED":
					reviewIcon = "✗"
					reviewStyle = mergeReviewChangesStyle
				default:
					reviewStyle = mergeReviewPendingStyle
				}
				reviewLine := fmt.Sprintf("%s @%s (%s)", reviewIcon, reviewer.Login, reviewer.State)
				b.WriteString(fmt.Sprintf("%s    %s\n", prefix, reviewStyle.Render(reviewLine)))
			}
		}

		// Commits
		if len(m.prDetails.Commits) > 0 {
			b.WriteString(fmt.Sprintf("%s 📝 Commits (%d):\n", treeVert, len(m.prDetails.Commits)))
			start := 0
			if len(m.prDetails.Commits) > 3 {
				start = len(m.prDetails.Commits) - 3
				b.WriteString(fmt.Sprintf("%s    %s\n", treeVert, mergeDimStyle.Render(fmt.Sprintf("... %d earlier commits", start))))
			}
			for j := start; j < len(m.prDetails.Commits); j++ {
				c := m.prDetails.Commits[j]
				commitLine := fmt.Sprintf("%s %s", c.SHA[:7], truncateStr(c.Message, 40))
				b.WriteString(fmt.Sprintf("%s    %s\n", treeVert, mergeDimStyle.Render(commitLine)))
			}
		}

		// Mergeable status
		if m.prDetails.Mergeable {
			b.WriteString(fmt.Sprintf("%s %s\n", treeLast, mergeSuccessStyle.Render("✓ Ready to merge")))
		} else {
			b.WriteString(fmt.Sprintf("%s %s\n", treeLast, mergeWarningStyle.Render("⚠ Not mergeable")))
		}
	}

	return mergeBoxStyle.Width(width).Render(b.String())
}

func (m mergeModel) renderChecks(width int) string {
	var b strings.Builder

	// Use mainPipelineCheck when in merged mode, otherwise use checks
	checks := m.checks
	if m.merged && m.mainPipelineCheck != nil {
		checks = m.mainPipelineCheck
	}

	// Header with auto-refresh status and merged mode indicator
	headerLabel := "🔍 CI Status"
	if m.merged {
		headerLabel = "🔍 Main Pipeline"
	}
	autoRefreshStatus := ""
	if m.merged && !m.pipelineComplete {
		autoRefreshStatus = " [watching]"
	} else if m.autoRefresh {
		autoRefreshStatus = " [auto]"
	}
	if m.refreshing {
		autoRefreshStatus = " [refreshing...]"
	}
	b.WriteString(mergeLabelStyle.Render(fmt.Sprintf("%s%s", headerLabel, autoRefreshStatus)))
	b.WriteString("\n")

	if checks == nil || checks.TotalCount == 0 {
		if m.merged {
			b.WriteString(mergeDimStyle.Render("  ⏳ Waiting for main pipeline to start..."))
		} else {
			b.WriteString(mergeDimStyle.Render("  ⏳ Waiting for CI to start..."))
		}
		b.WriteString("\n")
		if !m.merged {
			b.WriteString(mergeDimStyle.Render("  Press 'r' to refresh • 'a' to toggle auto-refresh"))
		}
		return mergeBoxStyle.Width(width).Render(b.String())
	}

	// Commit SHA (verify this is the right CI run)
	shortSHA := checks.HeadSHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	if m.merged {
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  📌 Branch: main (workflow %s)", shortSHA)))
	} else {
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  📌 Commit: %s", shortSHA)))
	}
	b.WriteString("\n")

	// Last refresh time
	if !checks.UpdatedAt.IsZero() {
		refreshTime := checks.UpdatedAt.Format("15:04:05")
		b.WriteString(mergeDimStyle.Render(fmt.Sprintf("  🔄 Last refresh: %s", refreshTime)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Progress bar
	b.WriteString(m.renderProgressBarWithChecks(checks, width-6))
	b.WriteString("\n\n")

	// Summary with detailed breakdown
	var statusIcon string
	var statusStyle lipgloss.Style
	switch checks.Status {
	case "success":
		statusIcon = "✓"
		statusStyle = mergeCheckSuccessStyle
	case "failure":
		statusIcon = "✗"
		statusStyle = mergeCheckFailureStyle
	default:
		statusIcon = "○"
		statusStyle = mergeCheckPendingStyle
	}

	// Status summary line
	summary := fmt.Sprintf("  %s %d/%d checks passed", statusIcon, checks.Success, checks.TotalCount)
	b.WriteString(statusStyle.Render(summary))
	b.WriteString("\n")

	// Detailed breakdown if there are pending checks
	if checks.Pending > 0 || checks.Queued > 0 || checks.InProgress > 0 {
		breakdown := fmt.Sprintf("     (%d queued, %d running, %d completed)",
			checks.Queued, checks.InProgress, checks.Success+checks.Failure)
		b.WriteString(mergeDimStyle.Render(breakdown))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// All checks (show all, not limited)
	hasFailures := false
	for _, check := range checks.Checks {
		var icon string
		var style lipgloss.Style
		var statusText string

		switch check.Status {
		case "completed":
			switch check.Conclusion {
			case "success":
				icon = "✓"
				style = mergeCheckSuccessStyle
				if !check.CompletedAt.IsZero() {
					duration := check.CompletedAt.Sub(check.StartedAt).Round(time.Second)
					statusText = fmt.Sprintf(" (%s)", duration)
				}
			case "failure":
				icon = "✗"
				style = mergeCheckFailureStyle
				statusText = " (failed)"
				hasFailures = true
			case "cancelled":
				icon = "⊘"
				style = mergeCheckPendingStyle
				statusText = " (cancelled)"
			case "skipped":
				icon = "⊘"
				style = mergeDimStyle
				statusText = " (skipped)"
			default:
				icon = "?"
				style = mergeCheckPendingStyle
				statusText = fmt.Sprintf(" (%s)", check.Conclusion)
			}
		case "in_progress":
			icon = "●"
			style = mergeCheckPendingStyle
			if !check.StartedAt.IsZero() {
				elapsed := time.Since(check.StartedAt).Round(time.Second)
				statusText = fmt.Sprintf(" (running %s)", elapsed)
			} else {
				statusText = " (running)"
			}
		case "queued":
			icon = "○"
			style = mergeDimStyle
			statusText = " (queued)"
		default:
			icon = "○"
			style = mergeDimStyle
			statusText = fmt.Sprintf(" (%s)", check.Status)
		}

		checkLine := fmt.Sprintf("    %s %s%s", icon, check.Name, statusText)
		b.WriteString(style.Render(checkLine))
		b.WriteString("\n")

		// Show error log if enabled and check failed
		if m.showErrorLogs && check.Conclusion == "failure" && check.ErrorLog != "" {
			b.WriteString(mergeErrorStyle.Render("      └─ Error: "))
			// Show error log, indented
			errorLines := strings.Split(check.ErrorLog, "\n")
			for i, line := range errorLines {
				if i > 0 {
					b.WriteString("         ")
				}
				b.WriteString(mergeDimStyle.Render(line))
				if i < len(errorLines)-1 {
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		}

		// Show failed step if available
		if check.Conclusion == "failure" && check.FailedStep != "" {
			b.WriteString(mergeErrorStyle.Render(fmt.Sprintf("      └─ Failed step: %s", check.FailedStep)))
			b.WriteString("\n")
		}
	}

	// Error logs toggle hint
	if hasFailures {
		if m.showErrorLogs {
			b.WriteString(mergeDimStyle.Render("\n  Press 'e' to hide error details"))
		} else {
			b.WriteString(mergeDimStyle.Render("\n  Press 'e' to show error details"))
		}
	}

	return mergeBoxStyle.Width(width).Render(b.String())
}

// renderProgressBarWithChecks creates a visual progress bar for given checks
func (m mergeModel) renderProgressBarWithChecks(checks *remote.PRChecks, width int) string {
	if checks == nil || checks.TotalCount == 0 {
		return ""
	}

	// Calculate progress
	completed := checks.Success + checks.Failure
	total := checks.TotalCount
	if total == 0 {
		total = 1 // Avoid division by zero
	}

	// Bar dimensions
	barWidth := width - 20 // Leave space for label and percentage
	if barWidth < 20 {
		barWidth = 20
	}

	successWidth := (checks.Success * barWidth) / total
	failureWidth := (checks.Failure * barWidth) / total
	runningWidth := (checks.InProgress * barWidth) / total
	queuedWidth := barWidth - successWidth - failureWidth - runningWidth

	// Ensure at least 1 char for running jobs if any
	if checks.InProgress > 0 && runningWidth == 0 {
		runningWidth = 1
		if queuedWidth > 0 {
			queuedWidth--
		}
	}

	// Build progress bar
	var bar strings.Builder
	bar.WriteString("  [")

	// Success (green)
	if successWidth > 0 {
		bar.WriteString(mergeCheckSuccessStyle.Render(strings.Repeat("█", successWidth)))
	}

	// Failure (red)
	if failureWidth > 0 {
		bar.WriteString(mergeCheckFailureStyle.Render(strings.Repeat("█", failureWidth)))
	}

	// Running (yellow/orange)
	if runningWidth > 0 {
		bar.WriteString(mergeCheckPendingStyle.Render(strings.Repeat("▓", runningWidth)))
	}

	// Queued (dim)
	if queuedWidth > 0 {
		bar.WriteString(mergeDimStyle.Render(strings.Repeat("░", queuedWidth)))
	}

	bar.WriteString("]")

	// Percentage
	percentage := (completed * 100) / total
	bar.WriteString(fmt.Sprintf(" %d%%", percentage))

	return bar.String()
}

func (m mergeModel) renderConfirmation() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(mergeWarningStyle.Render("  ⚠ Confirm Merge"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  You are about to merge PR #%d into %s\n",
		m.prNumber, m.prDetails.BaseBranch))
	b.WriteString(fmt.Sprintf("  Method: %s\n", mergeMethods[m.mergeMethod]))
	b.WriteString("\n")

	// Show what will happen after merge
	var postActions []string
	if m.prConfig.DeleteBranchAfterMerge {
		postActions = append(postActions, fmt.Sprintf("Delete branch '%s'", m.prDetails.HeadBranch))
	}
	if m.prConfig.CheckoutAfterMerge {
		postActions = append(postActions, "Checkout to main")
	}
	if m.prConfig.SyncAfterMerge {
		postActions = append(postActions, "Sync with remote")
	}

	if len(postActions) > 0 {
		b.WriteString("  After merge:\n")
		for _, action := range postActions {
			b.WriteString(fmt.Sprintf("    • %s\n", action))
		}
		b.WriteString("\n")
	}

	b.WriteString(mergeSelectedStyle.Render("  Press [Y] to confirm, [N] or [Esc] to cancel"))
	b.WriteString("\n")

	return b.String()
}

func (m mergeModel) renderQuitConfirmation() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(mergeSuccessStyle.Render("  ✓ Merge Complete"))
	b.WriteString("\n\n")

	if m.mainPipelineCheck != nil {
		switch m.mainPipelineCheck.Status {
		case "success":
			b.WriteString("  All CI pipelines passed!\n")
		case "failure":
			b.WriteString(mergeWarningStyle.Render("  ⚠ Some CI pipelines failed\n"))
		}
	}

	b.WriteString("\n")
	b.WriteString(mergeSelectedStyle.Render("  Press [Q] to quit, [Enter] to stay"))
	b.WriteString("\n")

	return b.String()
}

func (m mergeModel) renderMergeMethod(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeMethod {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("⚙ Merge Method"))
	b.WriteString("\n")

	for i, method := range mergeMethods {
		var prefix string
		var style lipgloss.Style
		if i == m.mergeMethod {
			prefix = "● "
			style = mergeSelectedStyle
		} else {
			prefix = "○ "
			style = mergeUnselectedStyle
		}

		var description string
		switch method {
		case "squash":
			description = "Squash and merge (combine all commits into one)"
		case "merge":
			description = "Create a merge commit"
		case "rebase":
			description = "Rebase and merge (linear history)"
		}

		line := fmt.Sprintf("  %s%s - %s", prefix, method, description)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return boxStyle.Render(b.String())
}

func (m mergeModel) renderCommitMessage(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeMessage {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("✏ Commit Message"))
	b.WriteString("\n")

	// Title
	titleLabel := "Title:"
	if m.focus == focusMergeMessage && !m.editingBody {
		titleLabel = "Title: (editing)"
	}
	b.WriteString(mergeDimStyle.Render("  " + titleLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.commitTitle.View())
	b.WriteString("\n\n")

	// Body
	bodyLabel := "Body:"
	if m.focus == focusMergeMessage && m.editingBody {
		bodyLabel = "Body: (editing)"
	}
	b.WriteString(mergeDimStyle.Render("  " + bodyLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.commitBody.View())

	return boxStyle.Render(b.String())
}

func (m mergeModel) renderActions(width int) string {
	var b strings.Builder

	boxStyle := mergeBoxStyle.Width(width)
	if m.focus == focusMergeActions {
		boxStyle = mergeActiveBoxStyle.Width(width)
	}

	b.WriteString(mergeLabelStyle.Render("🚀 Actions"))
	b.WriteString("\n")

	for i, action := range mergeActions {
		var style lipgloss.Style
		var prefix string
		if i == m.actionCursor && m.focus == focusMergeActions {
			prefix = "▶ "
			style = mergeSelectedStyle
		} else {
			prefix = "  "
			style = mergeUnselectedStyle
		}

		b.WriteString(style.Render(prefix + action))
		b.WriteString("\n")
	}

	return boxStyle.Render(b.String())
}

// truncateStr truncates a string to maxLen with ellipsis
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// runMergeTUI runs the merge TUI
func runMergeTUI(provider *github.Client, prNumber int, prConfig config.PRConfig) error {
	model := newMergeModel(provider, prNumber, prConfig)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
