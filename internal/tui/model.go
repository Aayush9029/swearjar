package tui

import (
	"fmt"
	"strings"

	"github.com/Aayush9029/swearjar/internal/analytics"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	report analytics.Report
	width  int
	height int
}

var (
	accent   = lipgloss.Color("205")
	yellow   = lipgloss.Color("221")
	green    = lipgloss.Color("114")
	cyan     = lipgloss.Color("81")
	muted    = lipgloss.Color("245")
	header   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dim      = lipgloss.NewStyle().Foreground(muted)
	selected = lipgloss.NewStyle().Bold(true).Foreground(yellow)
	title    = lipgloss.NewStyle().Bold(true).Foreground(cyan)
)

func Run(report analytics.Report) error {
	_, err := tea.NewProgram(Model{report: report}, tea.WithAltScreen()).Run()
	return err
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n\n")
	b.WriteString(m.overview())
	b.WriteString("\n\n")
	b.WriteString(dim.Render("q quit"))
	return b.String()
}

func (m Model) headerView() string {
	scope := m.report.Scope
	if scope == "" {
		scope = "all local history"
	}
	return title.Render("swearjar") + dim.Render("  "+scope)
}

func (m Model) overview() string {
	var b strings.Builder
	b.WriteString(header.Render("total"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  %s\n",
		selected.Render(fmt.Sprintf("%d swears", m.report.Totals.Swears)),
		dim.Render(fmt.Sprintf("%d messages · %.1f%% · %s", m.report.Totals.Messages, m.report.Totals.Rate, m.report.Duration))))

	if len(m.report.Agents) > 0 {
		b.WriteString("\n")
		b.WriteString(header.Render("agents"))
		b.WriteString("\n")
		maxSwears := maxAgentSwears(m.report.Agents)
		for _, row := range m.report.Agents[:min(len(m.report.Agents), 6)] {
			b.WriteString(fmt.Sprintf("  %-10s %5d  %-22s %s\n",
				agentStyle(row.Agent).Render(row.Agent),
				row.Swears,
				dim.Render(fmt.Sprintf("%d messages · %.1f%%", row.Messages, row.Rate)),
				tuiBar(row.Swears, maxSwears, 18)))
		}
	}

	if len(m.report.Words) > 0 {
		b.WriteString("\n")
		b.WriteString(header.Render("top words"))
		b.WriteString("\n")
		maxCount := m.report.Words[0].Count
		for _, row := range m.report.Words[:min(len(m.report.Words), 10)] {
			b.WriteString(fmt.Sprintf("  %-12s %5d  %-8s %s\n",
				selected.Render(row.Group),
				row.Count,
				dim.Render(fmt.Sprintf("%.1f%%", row.Share)),
				tuiBar(row.Count, maxCount, 18)))
		}
	} else if m.report.Totals.Messages > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render("  the jar is empty. not a single swear found."))
	}

	if m.report.Totals.Messages == 0 {
		b.WriteString(dim.Render("  no local messages found"))
	}
	return b.String()
}

func agentStyle(agent string) lipgloss.Style {
	switch agent {
	case "codex":
		return lipgloss.NewStyle().Foreground(green)
	case "claude":
		return lipgloss.NewStyle().Foreground(cyan)
	case "amp":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	case "file":
		return lipgloss.NewStyle().Foreground(yellow)
	default:
		return lipgloss.NewStyle().Foreground(accent)
	}
}

func tuiBar(value, max int64, width int) string {
	if max <= 0 {
		return dim.Render(strings.Repeat("░", width))
	}
	n := int(float64(value) / float64(max) * float64(width))
	if value > 0 && n == 0 {
		n = 1
	}
	if n > width {
		n = width
	}
	return lipgloss.NewStyle().Foreground(cyan).Render(strings.Repeat("█", n)) +
		dim.Render(strings.Repeat("░", width-n))
}

func maxAgentSwears(rows []analytics.AgentRow) int64 {
	var max int64
	for _, row := range rows {
		if row.Swears > max {
			max = row.Swears
		}
	}
	return max
}
