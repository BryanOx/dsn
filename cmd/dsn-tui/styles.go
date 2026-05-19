package main

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	primary   = lipgloss.Color("#7C3AED") // purple
	secondary = lipgloss.Color("#10B981") // green
	warning   = lipgloss.Color("#F59E0B") // amber
	errorCol  = lipgloss.Color("#EF4444") // red
	muted     = lipgloss.Color("#6B7280") // gray
	highlight = lipgloss.Color("#3B82F6") // blue

	// App styles
	appStyle = lipgloss.NewStyle().
			Padding(1, 2)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primary).
			Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primary).
			MarginBottom(1)

	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(muted)

	tabActiveStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(primary).
			Bold(true)

	labelStyle = lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF"))

	errorStyle = lipgloss.NewStyle().
			Foreground(errorCol).
			Bold(true)

	infoStyle = lipgloss.NewStyle().
			Foreground(highlight)

	helpStyle = lipgloss.NewStyle().
			Foreground(muted).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(secondary)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2)

	successStyle = lipgloss.NewStyle().
			Foreground(secondary).
			Bold(true)
)

const (
	tabDashboard = " Dashboard "
	tabBalance   = " Balance "
	tabSend      = " Send "
	tabPending   = " Pending "
	tabWallet    = " Wallet "
)

var allTabs = []string{tabDashboard, tabBalance, tabSend, tabPending, tabWallet}

func renderTabs(active int) string {
	var rendered []string
	for i, t := range allTabs {
		if i == active {
			rendered = append(rendered, tabActiveStyle.Render(t))
		} else {
			rendered = append(rendered, tabStyle.Render(t))
		}
	}
	line := ""
	for _, r := range rendered {
		line += r
	}
	return line
}
