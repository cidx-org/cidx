// Package tui provides shared styles and components for CIDX terminal UIs.
package tui

import "github.com/charmbracelet/lipgloss"

// Color palette -- single source of truth for the CIDX design system.
var (
	ColorAccent  = lipgloss.Color("39")  // Blue -- titles, labels, active borders
	ColorText    = lipgloss.Color("255") // White -- primary text, values
	ColorSuccess = lipgloss.Color("42")  // Green -- passed, completed
	ColorError   = lipgloss.Color("196") // Red -- failed, errors, destructive
	ColorWarning = lipgloss.Color("214") // Orange -- warnings, pending checks
	ColorDim     = lipgloss.Color("241") // Gray -- help text, inactive, reviews pending
	ColorBorder  = lipgloss.Color("240") // Dark gray -- borders, subtle separators
	ColorPending = lipgloss.Color("33")  // Blue -- pending/in-progress
	ColorSelect  = lipgloss.Color("57")  // Purple -- selected item background
)

// Layout styles -- structural elements reused across all TUIs.
var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent).
		Padding(0, 1)

	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	ActiveBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(0, 1)

	Help = lipgloss.NewStyle().
		Foreground(ColorDim).
		Padding(1, 0, 0, 0)
)

// Text styles -- semantic text rendering.
var (
	Label = lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true)

	Value = lipgloss.NewStyle().
		Foreground(ColorText)

	Dim = lipgloss.NewStyle().
		Foreground(ColorDim)

	Success = lipgloss.NewStyle().
		Foreground(ColorSuccess)

	SuccessBold = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Bold(true)

	Error = lipgloss.NewStyle().
		Foreground(ColorError)

	ErrorBold = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	Warning = lipgloss.NewStyle().
		Foreground(ColorWarning)

	WarningBold = lipgloss.NewStyle().
			Foreground(ColorWarning).
			Bold(true)

	Pending = lipgloss.NewStyle().
		Foreground(ColorPending)

	Selected = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	Unselected = lipgloss.NewStyle().
			Foreground(ColorText)
)

// List styles -- for interactive list/selection UIs.
var (
	ListSelected = lipgloss.NewStyle().
			Background(ColorSelect).
			Foreground(ColorText).
			Bold(true)

	ListHeader = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)
)
