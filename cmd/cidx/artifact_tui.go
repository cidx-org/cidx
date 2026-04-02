package main

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/cidx-org/cidx/internal/tui"
	"github.com/cidx-org/cidx/pkg/remote"
	"github.com/cidx-org/cidx/pkg/remote/github"
	"github.com/cidx-org/cidx/pkg/vcs"
)

// Artifact TUI styles - aliased from shared tui package
var (
	artifactTitleStyle    = tui.Title
	artifactBoxStyle      = tui.Box
	artifactSelectedStyle = tui.ListSelected
	artifactNormalStyle   = tui.Value
	artifactExpiredStyle  = tui.Dim
	artifactHeaderStyle   = tui.ListHeader
	artifactStatsStyle    = tui.Success
	artifactDeleteStyle   = tui.ErrorBold
	artifactHelpStyle     = tui.Help
)

// Artifact display item
type artifactItem struct {
	artifact remote.Artifact
	selected bool
}

// Artifact TUI model
type artifactModel struct {
	provider       *github.Client
	items          []artifactItem
	cursor         int
	loading        bool
	deleting       bool
	confirming     bool // confirmation dialog active
	err            error
	width          int
	height         int
	offset         int // scroll offset
	message        string
	messageTime    time.Time
	stats          *remote.ArtifactStats
	sortBy         string // "date", "size", "name"
	sortDesc       bool
	filterExp      bool // show only expired
	deleteProgress string // current artifact being deleted
}

// Messages
type artifactLoadedMsg struct {
	stats *remote.ArtifactStats
}

type artifactDeletedMsg struct {
	count int
	freed int64
}

type artifactErrorMsg struct {
	err error
}

type artifactDeletingMsg struct {
	name    string
	current int
	total   int
}

type clearMessageMsg struct{}

func newArtifactModel(provider *github.Client) artifactModel {
	return artifactModel{
		provider: provider,
		loading:  true,
		sortBy:   "date",
		sortDesc: true,
	}
}

func (m artifactModel) Init() tea.Cmd {
	return m.loadArtifacts()
}

func (m artifactModel) loadArtifacts() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		stats, err := m.provider.ListArtifacts(ctx)
		if err != nil {
			return artifactErrorMsg{err: err}
		}
		return artifactLoadedMsg{stats: stats}
	}
}

func (m artifactModel) deleteSelected() tea.Cmd {
	// Find first selected item to delete
	var toDelete []artifactItem
	for _, item := range m.items {
		if item.selected {
			toDelete = append(toDelete, item)
		}
	}

	if len(toDelete) == 0 {
		return func() tea.Msg {
			return artifactDeletedMsg{count: 0, freed: 0}
		}
	}

	return m.deleteNextArtifact(toDelete, 0, 0)
}

func (m artifactModel) deleteNextArtifact(items []artifactItem, index int, freedSoFar int64) tea.Cmd {
	if index >= len(items) {
		return func() tea.Msg {
			return artifactDeletedMsg{count: len(items), freed: freedSoFar}
		}
	}

	item := items[index]
	return tea.Sequence(
		func() tea.Msg {
			return artifactDeletingMsg{
				name:    item.artifact.Name,
				current: index + 1,
				total:   len(items),
			}
		},
		func() tea.Msg {
			ctx := context.Background()
			if err := m.provider.DeleteArtifact(ctx, item.artifact.ID); err != nil {
				return artifactErrorMsg{err: err}
			}
			// Continue with next item
			return artifactDeleteNextMsg{
				items:      items,
				nextIndex:  index + 1,
				freedSoFar: freedSoFar + item.artifact.SizeInBytes,
			}
		},
	)
}

type artifactDeleteNextMsg struct {
	items      []artifactItem
	nextIndex  int
	freedSoFar int64
}

func (m *artifactModel) sortItems() {
	sort.SliceStable(m.items, func(i, j int) bool {
		a, b := m.items[i].artifact, m.items[j].artifact
		var less bool
		switch m.sortBy {
		case "size":
			less = a.SizeInBytes < b.SizeInBytes
		case "name":
			less = strings.ToLower(a.Name) < strings.ToLower(b.Name)
		default: // date
			less = a.CreatedAt.Before(b.CreatedAt)
		}
		if m.sortDesc {
			return !less
		}
		return less
	})
}

func (m *artifactModel) filterItems() {
	if m.stats == nil {
		return
	}

	m.items = nil
	for _, a := range m.stats.Artifacts {
		if m.filterExp && !a.Expired {
			continue
		}
		m.items = append(m.items, artifactItem{artifact: a})
	}
	m.sortItems()
}

func (m artifactModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				maxVisible := m.maxVisibleItems()
				if m.cursor >= m.offset+maxVisible {
					m.offset = m.cursor - maxVisible + 1
				}
			}

		case "pgup":
			m.cursor -= m.maxVisibleItems()
			if m.cursor < 0 {
				m.cursor = 0
			}
			m.offset = m.cursor

		case "pgdown":
			m.cursor += m.maxVisibleItems()
			if m.cursor >= len(m.items) {
				m.cursor = len(m.items) - 1
			}
			maxVisible := m.maxVisibleItems()
			if m.cursor >= m.offset+maxVisible {
				m.offset = m.cursor - maxVisible + 1
			}

		case "home":
			m.cursor = 0
			m.offset = 0

		case "end":
			m.cursor = len(m.items) - 1
			maxVisible := m.maxVisibleItems()
			m.offset = m.cursor - maxVisible + 1
			if m.offset < 0 {
				m.offset = 0
			}

		case " ", "x":
			// Toggle selection
			if len(m.items) > 0 {
				m.items[m.cursor].selected = !m.items[m.cursor].selected
			}

		case "a":
			// Select all / deselect all
			allSelected := true
			for _, item := range m.items {
				if !item.selected {
					allSelected = false
					break
				}
			}
			for i := range m.items {
				m.items[i].selected = !allSelected
			}

		case "e":
			// Select all expired
			for i := range m.items {
				m.items[i].selected = m.items[i].artifact.Expired
			}

		case "d":
			// Show confirmation dialog for selected items (or current item if none selected)
			if !m.confirming && !m.deleting && len(m.items) > 0 {
				selected := 0
				for _, item := range m.items {
					if item.selected {
						selected++
					}
				}
				// If nothing selected, select current item
				if selected == 0 {
					m.items[m.cursor].selected = true
				}
				m.confirming = true
			}

		case "y", "Y":
			// Confirm deletion
			if m.confirming && !m.deleting {
				m.confirming = false
				m.deleting = true
				return m, m.deleteSelected()
			}

		case "n", "N":
			// Cancel deletion and deselect all
			if m.confirming {
				m.confirming = false
				for i := range m.items {
					m.items[i].selected = false
				}
			}

		case "r":
			// Refresh
			m.loading = true
			return m, m.loadArtifacts()

		case "s":
			// Cycle sort: date -> size -> name -> date
			switch m.sortBy {
			case "date":
				m.sortBy = "size"
			case "size":
				m.sortBy = "name"
			default:
				m.sortBy = "date"
			}
			m.sortItems()

		case "S":
			// Toggle sort direction
			m.sortDesc = !m.sortDesc
			m.sortItems()

		case "f":
			// Toggle expired filter
			m.filterExp = !m.filterExp
			m.filterItems()
			m.cursor = 0
			m.offset = 0
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case artifactLoadedMsg:
		m.stats = msg.stats
		m.loading = false
		m.filterItems()
		m.cursor = 0
		m.offset = 0

	case artifactDeletingMsg:
		m.deleteProgress = fmt.Sprintf("🗑️  Deleting %d/%d: %s", msg.current, msg.total, msg.name)

	case artifactDeleteNextMsg:
		return m, m.deleteNextArtifact(msg.items, msg.nextIndex, msg.freedSoFar)

	case artifactDeletedMsg:
		m.deleting = false
		m.deleteProgress = ""
		m.message = fmt.Sprintf("✓ Deleted %d artifacts, freed %s", msg.count, formatBytes(msg.freed))
		m.messageTime = time.Now()
		// Reload after delete
		m.loading = true
		return m, tea.Batch(
			m.loadArtifacts(),
			tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearMessageMsg{} }),
		)

	case artifactErrorMsg:
		m.err = msg.err
		m.loading = false
		m.deleting = false
		m.deleteProgress = ""

	case clearMessageMsg:
		if time.Since(m.messageTime) >= 3*time.Second {
			m.message = ""
		}
	}

	return m, nil
}

func (m artifactModel) maxVisibleItems() int {
	// Reserve lines for: title(1) + stats(3) + header(1) + footer(2) + help(2) + borders(4)
	return max(m.height-13, 5)
}

func (m artifactModel) View() string {
	if m.loading {
		return "\n  📦 Loading artifacts...\n"
	}

	if m.err != nil {
		return fmt.Sprintf("\n  ❌ Error: %v\n", m.err)
	}

	var sections []string

	// Title
	title := artifactTitleStyle.Render("📦 Artifact Manager")
	sections = append(sections, title)

	// Stats bar
	sections = append(sections, m.renderStats())

	// Message (if any)
	if m.message != "" {
		sections = append(sections, artifactStatsStyle.Render("  "+m.message))
	}

	// Delete progress
	if m.deleteProgress != "" {
		sections = append(sections, artifactDeleteStyle.Render("  "+m.deleteProgress))
	}

	// Confirmation dialog
	if m.confirming {
		sections = append(sections, m.renderConfirmDialog())
	}

	// Artifact list
	sections = append(sections, m.renderList())

	// Help
	sections = append(sections, m.renderHelp())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m artifactModel) renderStats() string {
	if m.stats == nil {
		return ""
	}

	// Count stats
	var totalSize, expiredSize, selectedSize int64
	var expiredCount, selectedCount int

	for _, item := range m.items {
		if item.artifact.Expired {
			expiredCount++
			expiredSize += item.artifact.SizeInBytes
		}
		if item.selected {
			selectedCount++
			selectedSize += item.artifact.SizeInBytes
		}
	}
	totalSize = m.stats.TotalSize

	// Build stats line
	statsLine := fmt.Sprintf("Total: %d (%s)", m.stats.TotalCount, formatBytes(totalSize))
	statsLine += fmt.Sprintf(" │ Expired: %d (%s)", expiredCount, formatBytes(expiredSize))

	if selectedCount > 0 {
		statsLine += artifactDeleteStyle.Render(fmt.Sprintf(" │ Selected: %d (%s)", selectedCount, formatBytes(selectedSize)))
	}

	// Filter/sort indicator
	filterSort := fmt.Sprintf("Sort: %s", m.sortBy)
	if m.sortDesc {
		filterSort += "↓"
	} else {
		filterSort += "↑"
	}
	if m.filterExp {
		filterSort += " │ Filter: expired only"
	}

	return artifactBoxStyle.Render(statsLine + "\n" + dimStyle.Render(filterSort))
}

func (m artifactModel) renderList() string {
	if len(m.items) == 0 {
		if m.filterExp {
			return artifactBoxStyle.Render("  No expired artifacts found")
		}
		return artifactBoxStyle.Render("  No artifacts found")
	}

	var content strings.Builder

	// Header
	header := fmt.Sprintf("%-3s %-40s %12s %16s %8s",
		"", "NAME", "SIZE", "CREATED", "STATUS")
	content.WriteString(artifactHeaderStyle.Render(header) + "\n")
	content.WriteString(dimStyle.Render(strings.Repeat("─", 85)) + "\n")

	// Items
	maxVisible := m.maxVisibleItems()
	start := m.offset
	end := min(start+maxVisible, len(m.items))

	for i := start; i < end; i++ {
		item := m.items[i]
		isCursor := i == m.cursor

		// Selection marker
		marker := "  "
		if item.selected {
			marker = artifactDeleteStyle.Render("✓ ")
		}

		// Status
		status := "active"
		if item.artifact.Expired {
			status = "expired"
		}

		// Name (truncate if needed)
		name := item.artifact.Name
		if len(name) > 38 {
			name = name[:36] + ".."
		}

		// Format line
		line := fmt.Sprintf("%s%-40s %12s %16s %8s",
			marker,
			name,
			formatBytes(item.artifact.SizeInBytes),
			item.artifact.CreatedAt.Format("2006-01-02 15:04"),
			status,
		)

		// Apply style
		if isCursor {
			line = artifactSelectedStyle.Render(line)
		} else if item.artifact.Expired {
			line = artifactExpiredStyle.Render(line)
		} else {
			line = artifactNormalStyle.Render(line)
		}

		content.WriteString(line + "\n")
	}

	// Scroll indicator
	if len(m.items) > maxVisible {
		scrollInfo := fmt.Sprintf("  Showing %d-%d of %d", start+1, end, len(m.items))
		content.WriteString(dimStyle.Render(scrollInfo))
	}

	return artifactBoxStyle.Render(content.String())
}

func (m artifactModel) renderConfirmDialog() string {
	// Count selected items
	var selectedCount int
	var selectedSize int64
	for _, item := range m.items {
		if item.selected {
			selectedCount++
			selectedSize += item.artifact.SizeInBytes
		}
	}

	// Build confirmation message
	confirmStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("196")).
		Padding(1, 2).
		Bold(true)

	msg := fmt.Sprintf("⚠️  Delete %d artifact(s) (%s)?\n\n", selectedCount, formatBytes(selectedSize))
	msg += artifactDeleteStyle.Render("  [y]es  ") + artifactNormalStyle.Render("  [n]o")

	return confirmStyle.Render(msg)
}

func (m artifactModel) renderHelp() string {
	var help string
	if m.confirming {
		help = "  Press [y] to confirm deletion, [n] to cancel"
	} else if m.deleting {
		help = "  🗑️  Deleting artifacts..."
	} else {
		help = "  [↑/↓] navigate  [space] select  [a]ll  [e]xpired  [d]elete  [s]ort  [f]ilter  [r]efresh  [q]uit"
	}
	return artifactHelpStyle.Render(help)
}

// formatBytes formats bytes into human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func runArtifactTUI(provider *github.Client) error {
	p := tea.NewProgram(newArtifactModel(provider), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

// getGitHubClient creates a GitHub client from the current repository
func getGitHubClient() (*github.Client, error) {
	repo, err := vcs.OpenRepository(".")
	if err != nil {
		return nil, fmt.Errorf("failed to open repository: %w", err)
	}

	provider, err := createProvider(repo)
	if err != nil {
		return nil, err
	}

	ghClient, ok := provider.(*github.Client)
	if !ok {
		return nil, fmt.Errorf("artifact management is only supported for GitHub repositories")
	}

	return ghClient, nil
}
