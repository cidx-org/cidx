package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
)

// Environment types
type Environment string

const (
	EnvLocal        Environment = "local"
	EnvGitHubActions Environment = "github-actions"
	EnvGitLabCI     Environment = "gitlab-ci"
	EnvJenkins      Environment = "jenkins"
	EnvCircleCI     Environment = "circleci"
	EnvTravisCI     Environment = "travis-ci"
	EnvAzurePipelines Environment = "azure-pipelines"
	EnvGenericCI    Environment = "ci"
)

// DetectEnvironment detects the current execution environment
func DetectEnvironment() Environment {
	// GitHub Actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		return EnvGitHubActions
	}

	// GitLab CI
	if os.Getenv("GITLAB_CI") == "true" {
		return EnvGitLabCI
	}

	// Jenkins
	if os.Getenv("JENKINS_URL") != "" || os.Getenv("BUILD_ID") != "" && os.Getenv("JOB_NAME") != "" {
		return EnvJenkins
	}

	// CircleCI
	if os.Getenv("CIRCLECI") == "true" {
		return EnvCircleCI
	}

	// Travis CI
	if os.Getenv("TRAVIS") == "true" {
		return EnvTravisCI
	}

	// Azure Pipelines
	if os.Getenv("TF_BUILD") == "true" {
		return EnvAzurePipelines
	}

	// Generic CI detection (many CI systems set CI=true)
	if os.Getenv("CI") == "true" {
		return EnvGenericCI
	}

	return EnvLocal
}

// IsCI returns true if running in a CI environment
func IsCI() bool {
	return DetectEnvironment() != EnvLocal
}

// EnvironmentName returns a human-readable name for the environment
func (e Environment) String() string {
	switch e {
	case EnvGitHubActions:
		return "GitHub Actions"
	case EnvGitLabCI:
		return "GitLab CI"
	case EnvJenkins:
		return "Jenkins"
	case EnvCircleCI:
		return "CircleCI"
	case EnvTravisCI:
		return "Travis CI"
	case EnvAzurePipelines:
		return "Azure Pipelines"
	case EnvGenericCI:
		return "CI"
	default:
		return "Local"
	}
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("39")).
			Padding(0, 1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	pendingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("33"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(1, 0, 0, 0)
)

// StatusInfo holds all gathered information
type StatusInfo struct {
	// Environment
	Environment Environment

	// Git info
	GitConfigured bool
	Branch        string
	CommitsAhead  int
	CommitsBehind int
	Staged        int
	Modified      int
	Untracked     int
	HasChanges    bool

	// GitHub info
	GitHubUser     string
	GitHubLoggedIn bool

	// PR info
	PRNumber int
	PRTitle  string
	PRState  string
	PRURL    string

	// CI info
	CIStatus  string
	CIChecks  []CICheck
	CIRunning bool

	// Project info
	ProjectName   string
	ProjectPath   string
	ConfigExists  bool
	PresetsLoaded int
}

// CICheck represents a CI check status
type CICheck struct {
	Name     string
	Status   string // success, failure, pending, running
	Duration string // e.g., "12s", "2m 15s"
}

// Watch mode states
type watchMode string

const (
	watchOff    watchMode = "off"
	watchSensor watchMode = "sensor" // Polling every 15s, waiting for CI to start
	watchActive watchMode = "active" // Polling every 3s, CI is running
)

// Model for bubbletea
type statusModel struct {
	info       StatusInfo
	loading    bool
	err        error
	width      int
	height     int
	watching   watchMode
	watchStart time.Time
}

// Messages
type statusLoadedMsg struct {
	info StatusInfo
}

type errMsg struct {
	err error
}

type tickMsg time.Time

func (m statusModel) Init() tea.Cmd {
	return loadStatus
}

// tickCmd returns a command that sends a tick after the interval
func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m statusModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.loading = true
			return m, loadStatus
		case "w":
			// Toggle watch mode (only if PR exists)
			if m.info.PRNumber > 0 {
				if m.watching == watchOff {
					m.watching = watchSensor
					m.watchStart = time.Now()
					// Start in sensor mode, will switch to active if CI is running
					return m, tea.Batch(loadStatus, tickCmd(15*time.Second))
				} else {
					m.watching = watchOff
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tickMsg:
		if m.watching != watchOff {
			// Check if any CI checks are still running
			hasRunning := false
			for _, check := range m.info.CIChecks {
				status := check.Status
				// Consider check as "in progress" if status is empty or explicitly running
				if status == "" || status == "pending" || status == "queued" || status == "in_progress" || status == "expected" || status == "waiting" {
					hasRunning = true
					break
				}
			}

			// Determine next interval based on CI state
			var nextInterval time.Duration
			if hasRunning {
				// CI is running - switch to active mode (3s)
				m.watching = watchActive
				nextInterval = 3 * time.Second
			} else {
				// CI idle or finished - switch to sensor mode (15s)
				m.watching = watchSensor
				nextInterval = 15 * time.Second
			}

			// Continue watching (never stops automatically in sensor mode)
			return m, tea.Batch(loadStatus, tickCmd(nextInterval))
		}

	case statusLoadedMsg:
		m.info = msg.info
		m.loading = false

	case errMsg:
		m.err = msg.err
		m.loading = false
	}

	return m, nil
}

func (m statusModel) View() string {
	if m.loading {
		return "\n  Loading...\n"
	}

	if m.err != nil {
		return fmt.Sprintf("\n  Error: %v\n", m.err)
	}

	var sections []string

	// Title
	title := titleStyle.Render("CIDX Dashboard")
	sections = append(sections, title)

	// GitHub section
	sections = append(sections, m.renderGitHubSection())

	// Git section
	sections = append(sections, m.renderGitSection())

	// PR section (if exists)
	if m.info.PRNumber > 0 {
		sections = append(sections, m.renderPRSection())
	}

	// Project section
	sections = append(sections, m.renderProjectSection())

	// Help - show watch option only when PR exists
	helpText := "  [r]efresh  [q]uit"
	if m.info.PRNumber > 0 {
		switch m.watching {
		case watchSensor:
			helpText = "  [w]atch:SENSOR  [r]efresh  [q]uit"
		case watchActive:
			helpText = "  [w]atch:ACTIVE  [r]efresh  [q]uit"
		default:
			helpText = "  [w]atch  [r]efresh  [q]uit"
		}
	}
	help := helpStyle.Render(helpText)
	sections = append(sections, help)

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m statusModel) renderGitHubSection() string {
	var content strings.Builder

	icon := "🔐"
	if m.info.GitHubLoggedIn {
		content.WriteString(fmt.Sprintf("%s GitHub: %s  %s\n",
			icon,
			valueStyle.Render(m.info.GitHubUser),
			successStyle.Render("✓ authenticated")))
	} else {
		content.WriteString(fmt.Sprintf("%s GitHub: %s\n",
			icon,
			warningStyle.Render("not logged in (run: gh auth login)")))
	}

	return boxStyle.Render(content.String())
}

func (m statusModel) renderGitSection() string {
	var content strings.Builder

	// Branch
	content.WriteString(fmt.Sprintf("🌿 Branch: %s\n", valueStyle.Render(m.info.Branch)))

	// Commits ahead/behind
	aheadBehind := ""
	if m.info.CommitsAhead > 0 {
		aheadBehind += successStyle.Render(fmt.Sprintf("↑%d ahead", m.info.CommitsAhead))
	}
	if m.info.CommitsBehind > 0 {
		if aheadBehind != "" {
			aheadBehind += ", "
		}
		aheadBehind += warningStyle.Render(fmt.Sprintf("↓%d behind", m.info.CommitsBehind))
	}
	if aheadBehind == "" {
		aheadBehind = dimStyle.Render("up to date")
	}
	content.WriteString(fmt.Sprintf("📊 Commits: %s\n", aheadBehind))

	// Changes
	if m.info.HasChanges {
		changes := []string{}
		if m.info.Staged > 0 {
			changes = append(changes, successStyle.Render(fmt.Sprintf("%d staged", m.info.Staged)))
		}
		if m.info.Modified > 0 {
			changes = append(changes, warningStyle.Render(fmt.Sprintf("%d modified", m.info.Modified)))
		}
		if m.info.Untracked > 0 {
			changes = append(changes, dimStyle.Render(fmt.Sprintf("%d untracked", m.info.Untracked)))
		}
		content.WriteString(fmt.Sprintf("📝 Changes: %s", strings.Join(changes, ", ")))
	} else {
		content.WriteString(fmt.Sprintf("📝 Changes: %s", dimStyle.Render("clean")))
	}

	return boxStyle.Render(content.String())
}

func (m statusModel) renderPRSection() string {
	var content strings.Builder

	// PR info
	prStatus := ""
	switch m.info.PRState {
	case "open":
		prStatus = pendingStyle.Render("open")
	case "merged":
		prStatus = successStyle.Render("merged")
	case "closed":
		prStatus = dimStyle.Render("closed")
	}

	// Watch indicator
	watchIndicator := ""
	if m.watching != watchOff {
		elapsed := time.Since(m.watchStart).Round(time.Second)
		modeLabel := "sensor"
		if m.watching == watchActive {
			modeLabel = "active"
		}
		watchIndicator = warningStyle.Render(fmt.Sprintf(" 👀 %s (%s)", modeLabel, formatDuration(elapsed)))
	}

	content.WriteString(fmt.Sprintf("🔀 PR #%d: %s  [%s]%s\n",
		m.info.PRNumber,
		valueStyle.Render(m.info.PRTitle),
		prStatus,
		watchIndicator))

	// CI checks with duration
	if len(m.info.CIChecks) > 0 {
		for i, check := range m.info.CIChecks {
			prefix := "   ├─ "
			if i == len(m.info.CIChecks)-1 {
				prefix = "   └─ "
			}

			icon := ""
			switch check.Status {
			case "success":
				icon = successStyle.Render("✓")
			case "failure":
				icon = errorStyle.Render("✗")
			case "pending", "queued":
				icon = pendingStyle.Render("○")
			case "in_progress":
				icon = warningStyle.Render("◐")
			default:
				icon = dimStyle.Render("?")
			}

			duration := ""
			if check.Duration != "" {
				duration = dimStyle.Render(fmt.Sprintf(" (%s)", check.Duration))
			}

			content.WriteString(fmt.Sprintf("%s%s %s%s\n", prefix, icon, check.Name, duration))
		}
	}

	return boxStyle.Render(content.String())
}

func (m statusModel) renderProjectSection() string {
	var content strings.Builder

	// Environment
	envIcon := "🖥️"
	envStyle := successStyle
	if m.info.Environment != EnvLocal {
		envIcon = "⚙️"
		envStyle = pendingStyle
	}
	content.WriteString(fmt.Sprintf("%s  Environment: %s\n", envIcon, envStyle.Render(m.info.Environment.String())))

	// Project name and path
	content.WriteString(fmt.Sprintf("📁 Project: %s\n", valueStyle.Render(m.info.ProjectName)))

	// Config status
	if m.info.ConfigExists {
		content.WriteString(fmt.Sprintf("📋 Config: %s  %s",
			valueStyle.Render("cidx.toml"),
			successStyle.Render("✓")))
	} else {
		content.WriteString(fmt.Sprintf("📋 Config: %s",
			dimStyle.Render("not found (run: cidx init)")))
	}

	return boxStyle.Render(content.String())
}

func loadStatus() tea.Msg {
	info := StatusInfo{}

	// Detect environment
	info.Environment = DetectEnvironment()

	// Get project info
	info.ProjectPath = runCmd("pwd")
	parts := strings.Split(info.ProjectPath, "/")
	if len(parts) > 0 {
		info.ProjectName = parts[len(parts)-1]
	}

	// Check config
	info.ConfigExists = fileExists("cidx.toml")

	// Get GitHub user
	ghUser := runCmd("gh", "api", "user", "--jq", ".login")
	if ghUser != "" {
		info.GitHubLoggedIn = true
		info.GitHubUser = ghUser
	}

	// Get git branch
	info.Branch = runCmd("git", "rev-parse", "--abbrev-ref", "HEAD")

	// Get commits ahead/behind
	upstream := runCmd("git", "rev-parse", "--abbrev-ref", "@{upstream}")
	if upstream != "" {
		aheadBehind := runCmd("git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
		parts := strings.Fields(aheadBehind)
		if len(parts) == 2 {
			info.CommitsAhead, _ = strconv.Atoi(parts[0])
			info.CommitsBehind, _ = strconv.Atoi(parts[1])
		}
	}

	// Get changes
	status := runCmd("git", "status", "--porcelain")
	if status != "" {
		info.HasChanges = true
		lines := strings.Split(status, "\n")
		for _, line := range lines {
			if len(line) < 2 {
				continue
			}
			// First char is staged status, second is working tree status
			staged := line[0]
			working := line[1]

			if staged != ' ' && staged != '?' {
				info.Staged++
			}
			if working == 'M' || working == 'D' {
				info.Modified++
			}
			if staged == '?' {
				info.Untracked++
			}
		}
	}

	// Get PR for current branch
	prJSON := runCmd("gh", "pr", "view", "--json", "number,title,state,url,statusCheckRollup")
	if prJSON != "" {
		// Parse PR info
		info.PRNumber = extractJSONInt(prJSON, "number")
		info.PRTitle = extractJSONString(prJSON, "title")
		info.PRState = strings.ToLower(extractJSONString(prJSON, "state"))
		info.PRURL = extractJSONString(prJSON, "url")

		// Parse CI checks
		info.CIChecks = extractCIChecks(prJSON)
	}

	return statusLoadedMsg{info: info}
}

func runCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fileExists(path string) bool {
	cmd := exec.Command("test", "-f", path)
	return cmd.Run() == nil
}

func extractJSONString(json, key string) string {
	// Simple JSON extraction (avoid adding json dependency for now)
	search := fmt.Sprintf(`"%s":"`, key)
	idx := strings.Index(json, search)
	if idx == -1 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(json[start:], `"`)
	if end == -1 {
		return ""
	}
	return json[start : start+end]
}

func extractJSONInt(json, key string) int {
	search := fmt.Sprintf(`"%s":`, key)
	idx := strings.Index(json, search)
	if idx == -1 {
		return 0
	}
	start := idx + len(search)
	end := start
	for end < len(json) && (json[end] >= '0' && json[end] <= '9') {
		end++
	}
	val, _ := strconv.Atoi(json[start:end])
	return val
}

func extractCIChecks(json string) []CICheck {
	var checks []CICheck

	// Look for statusCheckRollup array
	search := `"statusCheckRollup":[`
	idx := strings.Index(json, search)
	if idx == -1 {
		return checks
	}

	// Find the array end
	start := idx + len(search)
	depth := 1
	end := start
	for end < len(json) && depth > 0 {
		switch json[end] {
		case '[':
			depth++
		case ']':
			depth--
		}
		end++
	}

	checkArray := json[start : end-1]

	// Parse individual checks
	for _, part := range strings.Split(checkArray, "},{") {
		name := ""
		status := ""

		// Extract name (could be "name" or "context")
		if n := extractJSONString(part, "name"); n != "" {
			name = n
		} else if n := extractJSONString(part, "context"); n != "" {
			name = n
		}

		// Extract status (could be "conclusion" or "state")
		if s := extractJSONString(part, "conclusion"); s != "" {
			status = strings.ToLower(s)
		} else if s := extractJSONString(part, "state"); s != "" {
			status = strings.ToLower(s)
		}

		if name != "" {
			// Shorten common names
			name = shortenCheckName(name)
			checks = append(checks, CICheck{Name: name, Status: status})
		}
	}

	return checks
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if seconds == 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dm%ds", minutes, seconds)
}

func shortenCheckName(name string) string {
	// Map common CI job names to shorter versions
	replacements := map[string]string{
		"Security":     "Sec",
		"Code Quality": "Lint",
		"Build":        "Build",
		"Test":         "Test",
		"Setup":        "Setup",
	}

	for long, short := range replacements {
		if strings.Contains(name, long) {
			return short
		}
	}

	// Truncate if too long
	if len(name) > 10 {
		return name[:10]
	}
	return name
}

func statusCommand() *cli.Command {
	return &cli.Command{
		Name:  "status",
		Usage: "Show interactive dashboard with project status",
		Description: `Display project status in an interactive TUI dashboard.

In CI environments (GitHub Actions, GitLab CI, etc.), automatically
uses simple text output instead of TUI to avoid blocking the pipeline.

Use --tui to force TUI mode, or --no-tui to force simple output.`,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "no-tui",
				Aliases: []string{"n"},
				Usage:   "Force simple text output (no TUI)",
			},
			&cli.BoolFlag{
				Name:  "tui",
				Usage: "Force TUI mode even in CI",
			},
		},
		Action: func(c *cli.Context) error {
			// Determine if we should use TUI
			// Auto-detect: TUI in local, simple text in CI
			useTUI := !IsCI()

			// Explicit flags override auto-detection
			if c.Bool("no-tui") {
				useTUI = false
			}
			if c.Bool("tui") {
				useTUI = true
			}

			if !useTUI {
				return printSimpleStatus()
			}

			p := tea.NewProgram(statusModel{loading: true})
			_, err := p.Run()
			return err
		},
	}
}

func printSimpleStatus() error {
	// Simple non-interactive output
	msg := loadStatus().(statusLoadedMsg)
	info := msg.info

	fmt.Println("CIDX Status")
	fmt.Println("===========")
	fmt.Println()

	// Environment
	fmt.Printf("Environment: %s\n", info.Environment.String())

	// GitHub
	if info.GitHubLoggedIn {
		fmt.Printf("GitHub: %s (authenticated)\n", info.GitHubUser)
	} else {
		fmt.Println("GitHub: not logged in")
	}

	// Git
	fmt.Printf("Branch: %s\n", info.Branch)
	fmt.Printf("Commits: %d ahead, %d behind\n", info.CommitsAhead, info.CommitsBehind)
	fmt.Printf("Changes: %d staged, %d modified, %d untracked\n",
		info.Staged, info.Modified, info.Untracked)

	// PR
	if info.PRNumber > 0 {
		fmt.Printf("PR #%d: %s [%s]\n", info.PRNumber, info.PRTitle, info.PRState)
		// Show CI checks in simple format
		if len(info.CIChecks) > 0 {
			fmt.Print("CI: ")
			for i, check := range info.CIChecks {
				if i > 0 {
					fmt.Print(" | ")
				}
				status := "?"
				switch check.Status {
				case "success":
					status = "✓"
				case "failure":
					status = "✗"
				case "pending", "queued":
					status = "○"
				case "in_progress":
					status = "◐"
				}
				fmt.Printf("%s %s", status, check.Name)
			}
			fmt.Println()
		}
	}

	// Project
	fmt.Printf("Project: %s\n", info.ProjectName)
	if info.ConfigExists {
		fmt.Println("Config: cidx.toml found")
	} else {
		fmt.Println("Config: not found")
	}

	return nil
}
