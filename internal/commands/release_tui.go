package commands

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cidx-org/cidx/v2/internal/tui"
	"github.com/cidx-org/cidx/v2/pkg/actions"
	"github.com/cidx-org/cidx/v2/pkg/config"
	"github.com/cidx-org/cidx/v2/pkg/remote"
	"github.com/cidx-org/cidx/v2/pkg/vcs"
)

// Release TUI styles - aliased from shared tui package
var (
	releaseTitleStyle     = tui.Title
	releaseBoxStyle       = tui.Box
	releaseActiveBoxStyle = tui.ActiveBox
	releaseLabelStyle     = tui.Label
	releaseValueStyle     = tui.Value
	releaseHelpStyle      = tui.Help
	releaseSuccessStyle   = tui.SuccessBold
	releaseErrorStyle     = tui.ErrorBold
)

// releaseMode determines tag vs release mode
type releaseMode int

const (
	modeTag releaseMode = iota
	modeRelease
)

// focusField determines which field is focused
type focusField int

const (
	focusVersion focusField = iota
	focusMessage
	focusActions
)

// Release TUI model
type releaseModel struct {
	mode          releaseMode
	repo          *vcs.Repository
	provider      remote.Provider
	tagConfig     config.TagConfig
	releaseConfig config.ReleaseConfig
	workDir       string

	// State
	version      textinput.Model
	message      textarea.Model
	focus        focusField
	actionCursor int

	// Data
	lastTag      string
	suggestedVer string
	commits      []actions.CommitInfo

	// UI state
	loading bool
	err     error
	success string
	width   int
	height  int
}

// Actions available
var tagActions = []string{"Preview", "Create Tag", "Cancel"}
var releaseActions = []string{"Preview", "Create Release", "Cancel"}

// Messages
type releaseLoadedMsg struct {
	lastTag      string
	suggestedVer string
	message      string
	commits      []actions.CommitInfo
}

type releaseErrorMsg struct {
	err error
}

type releaseSuccessMsg struct {
	message string
}

func newReleaseModel(mode releaseMode, repo *vcs.Repository, provider remote.Provider, tagConfig config.TagConfig, releaseConfig config.ReleaseConfig) releaseModel {
	// Version input
	vi := textinput.New()
	vi.Placeholder = "0.0.0"
	vi.CharLimit = 20
	vi.Width = 20

	// Message textarea
	ta := textarea.New()
	ta.Placeholder = "Enter release message..."
	ta.CharLimit = 10000

	workDir, _ := repo.GetWorkDir()

	return releaseModel{
		mode:          mode,
		repo:          repo,
		provider:      provider,
		tagConfig:     tagConfig,
		releaseConfig: releaseConfig,
		workDir:       workDir,
		version:       vi,
		message:       ta,
		focus:         focusVersion,
		loading:       true,
	}
}

func (m releaseModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.loadData(),
	)
}

func (m releaseModel) loadData() tea.Cmd {
	return func() tea.Msg {
		// Get last tag
		lastTag := m.getLastTag()

		// Get suggested version
		var suggestedVer string
		var message string
		var commits []actions.CommitInfo

		if m.mode == modeTag {
			suggestedVer = actions.SuggestTagVersion(m.workDir)
			message = m.generateTagMessage(suggestedVer, lastTag)
		} else {
			// For release, try to get commits and generate notes
			commits = m.getCommitsSince(lastTag)
			suggestedVer = m.suggestVersionFromCommits(commits)
			message = m.generateReleaseNotes(suggestedVer, commits)
		}

		return releaseLoadedMsg{
			lastTag:      lastTag,
			suggestedVer: suggestedVer,
			message:      message,
			commits:      commits,
		}
	}
}

func (m releaseModel) getLastTag() string {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	cmd.Dir = m.workDir

	output, err := cmd.Output()
	if err != nil {
		return "(none)"
	}
	return strings.TrimSpace(string(output))
}

// suggestVersionFromCommits bumps the latest tag -- not the VERSION file --
// according to the commits in range (issue #185).
func (m releaseModel) suggestVersionFromCommits(commits []actions.CommitInfo) string {
	current := actions.ResolveVersion(m.workDir).Current()
	return actions.NextVersion(current, actions.CountCommitInfos(commits))
}

func (m releaseModel) generateTagMessage(version, lastTag string) string {
	tag := m.tagConfig.FormatTag(version)

	var sb strings.Builder
	fmt.Fprintf(&sb, "Release %s\n\n", tag)

	// Get commit summary since last tag
	if lastTag != "(none)" {
		commits := m.getCommitSummary(lastTag)
		if commits != "" {
			sb.WriteString("Changes:\n")
			sb.WriteString(commits)
		}
	}

	return sb.String()
}

func (m releaseModel) getCommitSummary(tag string) string {
	cmd := exec.Command("git", "log", tag+"..HEAD", "--oneline", "--no-merges")
	cmd.Dir = m.workDir

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) > 10 {
		lines = lines[:10]
		lines = append(lines, fmt.Sprintf("... and %d more commits", len(lines)-10))
	}

	var sb strings.Builder
	for _, line := range lines {
		if line != "" {
			fmt.Fprintf(&sb, "- %s\n", line)
		}
	}
	return sb.String()
}

func (m releaseModel) getCommitsSince(tag string) []actions.CommitInfo {
	args := []string{"log", actions.CommitLogFormat, "-20"}
	if tag != "" && tag != "(none)" {
		args = []string{"log", tag + "..HEAD", actions.CommitLogFormat}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = m.workDir

	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	return actions.ParseCommitLog(string(output))
}

func (m releaseModel) generateReleaseNotes(version string, commits []actions.CommitInfo) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Release v%s\n\n", version)

	// Group commits by type
	groups := map[string][]actions.CommitInfo{
		"feat":     {},
		"fix":      {},
		"refactor": {},
		"docs":     {},
		"chore":    {},
		"other":    {},
	}

	for _, c := range commits {
		if _, ok := groups[c.Type]; ok {
			groups[c.Type] = append(groups[c.Type], c)
		} else {
			groups["other"] = append(groups["other"], c)
		}
	}

	headers := map[string]string{
		"feat":     "## Features",
		"fix":      "## Bug Fixes",
		"refactor": "## Refactoring",
		"docs":     "## Documentation",
		"chore":    "## Maintenance",
		"other":    "## Other Changes",
	}

	order := []string{"feat", "fix", "refactor", "docs", "chore", "other"}

	for _, typ := range order {
		if len(groups[typ]) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "\n%s\n\n", headers[typ])
		for _, c := range groups[typ] {
			scope := ""
			if c.Scope != "" {
				scope = fmt.Sprintf("**%s:** ", c.Scope)
			}
			fmt.Fprintf(&sb, "- %s%s\n", scope, c.Subject)
		}
	}

	return sb.String()
}

func (m releaseModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit

		case "esc":
			if m.focus == focusMessage {
				// Exit message editing
				m.focus = focusActions
				m.message.Blur()
			} else {
				return m, tea.Quit
			}

		case "tab", "shift+tab":
			if m.focus == focusVersion {
				m.focus = focusMessage
				m.version.Blur()
				m.message.Focus()
			} else if m.focus == focusMessage && msg.String() == "shift+tab" {
				m.focus = focusVersion
				m.message.Blur()
				m.version.Focus()
			} else if m.focus == focusMessage && msg.String() == "tab" {
				m.focus = focusActions
				m.message.Blur()
			} else if m.focus == focusActions {
				if msg.String() == "shift+tab" {
					m.focus = focusMessage
					m.message.Focus()
				} else {
					m.focus = focusVersion
					m.version.Focus()
				}
			}

		case "up", "k":
			if m.focus == focusActions && m.actionCursor > 0 {
				m.actionCursor--
			}

		case "down", "j":
			if m.focus == focusActions {
				actions := tagActions
				if m.mode == modeRelease {
					actions = releaseActions
				}
				if m.actionCursor < len(actions)-1 {
					m.actionCursor++
				}
			}

		case "enter":
			switch m.focus {
			case focusActions:
				return m, m.executeAction()
			case focusVersion:
				m.focus = focusMessage
				m.version.Blur()
				m.message.Focus()
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.message.SetWidth(min(msg.Width-10, 80))
		m.message.SetHeight(min(msg.Height-15, 20))

	case releaseLoadedMsg:
		m.loading = false
		m.lastTag = msg.lastTag
		m.suggestedVer = msg.suggestedVer
		m.commits = msg.commits
		m.version.SetValue(msg.suggestedVer)
		m.message.SetValue(msg.message)
		m.version.Focus()

	case releaseErrorMsg:
		m.err = msg.err
		m.loading = false

	case releaseSuccessMsg:
		m.success = msg.message
	}

	// Update focused component
	switch m.focus {
	case focusVersion:
		var cmd tea.Cmd
		m.version, cmd = m.version.Update(msg)
		cmds = append(cmds, cmd)
	case focusMessage:
		var cmd tea.Cmd
		m.message, cmd = m.message.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m releaseModel) executeAction() tea.Cmd {
	actions := tagActions
	if m.mode == modeRelease {
		actions = releaseActions
	}

	action := actions[m.actionCursor]

	switch action {
	case "Cancel":
		return tea.Quit

	case "Preview":
		return func() tea.Msg {
			version := m.version.Value()
			message := m.message.Value()

			var preview string
			if m.mode == modeTag {
				tag := m.tagConfig.FormatTag(version)
				preview = fmt.Sprintf("Tag: %s\n\nMessage:\n%s", tag, message)
			} else {
				preview = fmt.Sprintf("Release: v%s\n\n%s", version, message)
			}

			return releaseSuccessMsg{message: "Preview:\n" + preview}
		}

	case "Create Tag":
		return m.createTag()

	case "Create Release":
		return m.createRelease()
	}

	return nil
}

func (m releaseModel) createTag() tea.Cmd {
	return func() tea.Msg {
		version := m.version.Value()
		message := m.message.Value()
		tag := m.tagConfig.FormatTag(version)

		// Create annotated tag
		cmd := exec.Command("git", "tag", "-a", tag, "-m", message)
		cmd.Dir = m.workDir

		if output, err := cmd.CombinedOutput(); err != nil {
			return releaseErrorMsg{err: fmt.Errorf("failed to create tag: %w\n%s", err, output)}
		}

		// Push tag if configured
		if m.tagConfig.AutoPush {
			cmd = exec.Command("git", "push", "origin", tag)
			cmd.Dir = m.workDir

			if output, err := cmd.CombinedOutput(); err != nil {
				return releaseErrorMsg{err: fmt.Errorf("failed to push tag: %w\n%s", err, output)}
			}
			return releaseSuccessMsg{message: fmt.Sprintf("Tag %s created and pushed!", tag)}
		}

		return releaseSuccessMsg{message: fmt.Sprintf("Tag %s created! Run 'git push origin %s' to push.", tag, tag)}
	}
}

func (m releaseModel) createRelease() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		version := m.version.Value()
		notes := m.message.Value()
		tag := fmt.Sprintf("v%s", version)

		// First, create and push tag
		cmd := exec.Command("git", "tag", "-a", tag, "-m", fmt.Sprintf("Release %s", tag))
		cmd.Dir = m.workDir

		if output, err := cmd.CombinedOutput(); err != nil {
			return releaseErrorMsg{err: fmt.Errorf("failed to create tag: %w\n%s", err, output)}
		}

		cmd = exec.Command("git", "push", "origin", tag)
		cmd.Dir = m.workDir

		if output, err := cmd.CombinedOutput(); err != nil {
			return releaseErrorMsg{err: fmt.Errorf("failed to push tag: %w\n%s", err, output)}
		}

		// Create GitHub release using gh CLI
		cmd = exec.Command("gh", "release", "create", tag, "--title", tag, "--notes", notes)
		cmd.Dir = m.workDir

		if output, err := cmd.CombinedOutput(); err != nil {
			// Try using provider API if gh CLI fails
			if m.provider != nil {
				// TODO: implement provider.CreateRelease
				return releaseErrorMsg{err: fmt.Errorf("failed to create release: %w\n%s", err, output)}
			}
			return releaseErrorMsg{err: fmt.Errorf("failed to create release: %w\n%s", err, output)}
		}

		_ = ctx // placeholder for future provider API usage

		return releaseSuccessMsg{message: fmt.Sprintf("Release %s created successfully!", tag)}
	}
}

func (m releaseModel) View() string {
	if m.loading {
		return "\n  Loading...\n"
	}

	var sections []string

	// Title
	title := "Tag"
	if m.mode == modeRelease {
		title = "Release"
	}
	sections = append(sections, releaseTitleStyle.Render(fmt.Sprintf("Create %s", title)))

	// Info bar
	info := fmt.Sprintf("Last tag: %s", m.lastTag)
	sections = append(sections, dimStyle.Render("  "+info))

	// Error/Success message
	if m.err != nil {
		sections = append(sections, releaseErrorStyle.Render(fmt.Sprintf("  Error: %v", m.err)))
	}
	if m.success != "" {
		sections = append(sections, releaseSuccessStyle.Render("  "+m.success))
	}

	// Version field
	versionBox := releaseBoxStyle
	if m.focus == focusVersion {
		versionBox = releaseActiveBoxStyle
	}
	versionContent := fmt.Sprintf("%s %s", releaseLabelStyle.Render("Version:"), m.version.View())
	sections = append(sections, versionBox.Render(versionContent))

	// Message field
	messageBox := releaseBoxStyle
	if m.focus == focusMessage {
		messageBox = releaseActiveBoxStyle
	}
	messageLabel := "Message:"
	if m.mode == modeRelease {
		messageLabel = "Release Notes:"
	}
	messageContent := fmt.Sprintf("%s\n%s", releaseLabelStyle.Render(messageLabel), m.message.View())
	sections = append(sections, messageBox.Render(messageContent))

	// Actions
	actionsBox := releaseBoxStyle
	if m.focus == focusActions {
		actionsBox = releaseActiveBoxStyle
	}

	actions := tagActions
	if m.mode == modeRelease {
		actions = releaseActions
	}

	var actionLines []string
	for i, action := range actions {
		marker := "  "
		if i == m.actionCursor && m.focus == focusActions {
			marker = "> "
			action = releaseValueStyle.Render(action)
		}
		actionLines = append(actionLines, marker+action)
	}
	actionsContent := releaseLabelStyle.Render("Actions:") + "\n" + strings.Join(actionLines, "\n")
	sections = append(sections, actionsBox.Render(actionsContent))

	// Help
	help := "  [Tab] next field  [Shift+Tab] prev field  [Enter] select  [Esc] back/quit"
	sections = append(sections, releaseHelpStyle.Render(help))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// runReleaseTUI starts the release/tag TUI
func runReleaseTUI(mode releaseMode, repo *vcs.Repository, provider remote.Provider, tagConfig config.TagConfig, releaseConfig config.ReleaseConfig) error {
	p := tea.NewProgram(
		newReleaseModel(mode, repo, provider, tagConfig, releaseConfig),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
