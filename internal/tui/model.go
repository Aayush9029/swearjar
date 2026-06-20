package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Aayush9029/swearjar/internal/analytics"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

type Model struct {
	report analytics.Report
	tab    int
	cursor int
	width  int
	height int
	filter string
	input  bool
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
		if m.input {
			return m.handleFilterKey(msg), nil
		}
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "/":
			if m.tab != 0 {
				m.input = true
			}
		case "tab", "right", "l":
			m.tab = (m.tab + 1) % 4
			m.cursor, m.filter, m.input = 0, "", false
		case "shift+tab", "left", "h":
			m.tab = (m.tab + 3) % 4
			m.cursor, m.filter, m.input = 0, "", false
		case "1":
			m.tab, m.cursor, m.filter, m.input = 0, 0, "", false
		case "2":
			m.tab, m.cursor, m.filter, m.input = 1, 0, "", false
		case "3":
			m.tab, m.cursor, m.filter, m.input = 2, 0, "", false
		case "4":
			m.tab, m.cursor, m.filter, m.input = 3, 0, "", false
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.currentMax()-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) Model {
	if msg.String() == "backspace" {
		if m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.cursor = 0
		}
		return m
	}
	switch msg.Type {
	case tea.KeyEsc, tea.KeyEnter:
		m.input = false
		return m
	case tea.KeyBackspace:
		if m.filter != "" {
			_, size := utf8.DecodeLastRuneInString(m.filter)
			m.filter = m.filter[:len(m.filter)-size]
			m.cursor = 0
		}
		return m
	case tea.KeyCtrlC:
		m.input = false
		return m
	}
	if msg.Type == tea.KeyRunes {
		m.filter += msg.String()
		m.cursor = 0
	}
	return m
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n\n")
	switch m.tab {
	case 0:
		b.WriteString(m.overview())
	case 1:
		b.WriteString(m.agents())
	case 2:
		b.WriteString(m.words())
	case 3:
		b.WriteString(m.sessions())
	}
	b.WriteString("\n\n")
	b.WriteString(dim.Render("tab switch · / fuzzy filter · j/k move · 1-4 jump · q quit"))
	return b.String()
}

func (m Model) headerView() string {
	tabs := []string{"overview", "agents", "words", "sessions"}
	parts := make([]string, len(tabs))
	for i, tab := range tabs {
		if i == m.tab {
			parts[i] = selected.Render(tab)
		} else {
			parts[i] = dim.Render(tab)
		}
	}
	scope := m.report.Scope
	if scope == "" {
		scope = "all local history"
	}
	filter := ""
	if m.tab != 0 && (m.input || m.filter != "") {
		prompt := "/"
		if m.input {
			prompt = selected.Render("/")
		} else {
			prompt = dim.Render("/")
		}
		filter = dim.Render("  filter ") + prompt + m.filter
	}
	return title.Render("swearjar") + dim.Render("  "+scope+"  ") + strings.Join(parts, dim.Render(" / ")) + filter
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
		b.WriteString(header.Render("agent language"))
		b.WriteString("\n")
		for _, row := range m.report.Agents {
			b.WriteString(fmt.Sprintf("  %-10s %5d  %s\n", agentStyle(row.Agent).Render(row.Agent), row.Swears, dim.Render(fmt.Sprintf("%d messages · %.1f%%", row.Messages, row.Rate))))
		}
	}
	if len(m.report.Words) > 0 {
		b.WriteString("\n")
		b.WriteString(header.Render("top words"))
		b.WriteString("\n")
		for _, row := range m.report.Words[:min(len(m.report.Words), 8)] {
			b.WriteString(fmt.Sprintf("  %-12s %5d  %s\n", selected.Render(row.Group), row.Count, dim.Render(row.Source)))
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

func (m Model) agents() string {
	rows := m.filteredAgents()
	if len(rows) == 0 {
		return dim.Render("no agents found")
	}
	max := slices.MaxFunc(rows, func(a, b analytics.AgentRow) int {
		return int(a.Swears - b.Swears)
	}).Swears

	var b strings.Builder
	b.WriteString(header.Render("agents"))
	b.WriteString("\n")
	for i, row := range rows {
		prefix := "  "
		if i == m.cursor {
			prefix = selected.Render("› ")
		}
		b.WriteString(fmt.Sprintf("%s%-10s %5d  %-20s %s\n",
			prefix,
			agentStyle(row.Agent).Render(row.Agent),
			row.Swears,
			dim.Render(fmt.Sprintf("%d messages · %.1f%%", row.Messages, row.Rate)),
			tuiBar(row.Swears, max, 22)))
	}
	return b.String()
}

func (m Model) words() string {
	rows := m.filteredWords()
	if len(rows) == 0 {
		return dim.Render("no swears found")
	}
	max := rows[0].Count
	var b strings.Builder
	b.WriteString(header.Render("words"))
	b.WriteString("\n")
	for i, row := range rows {
		prefix := "  "
		if i == m.cursor {
			prefix = selected.Render("› ")
		}
		variants := topVariants(m.report.Variants, row.Group)
		b.WriteString(fmt.Sprintf("%s%-12s %5d  %-14s %s %s\n",
			prefix,
			selected.Render(row.Group),
			row.Count,
			dim.Render(fmt.Sprintf("%.1f%% · %s", row.Share, row.Source)),
			tuiBar(row.Count, max, 18),
			dim.Render(variants)))
	}
	return b.String()
}

func (m Model) sessions() string {
	rows := m.filteredSessions()
	if len(rows) == 0 {
		return dim.Render("no sessions found")
	}
	var b strings.Builder
	b.WriteString(header.Render("sessions"))
	b.WriteString("\n")
	limit := min(len(rows), max(8, m.height-5))
	for i, row := range rows[:limit] {
		prefix := "  "
		if i == m.cursor {
			prefix = selected.Render("› ")
		}
		session := row.Session
		if len(session) > 34 {
			session = session[:31] + "..."
		}
		b.WriteString(fmt.Sprintf("%s%-9s %-34s %5d  %s\n",
			prefix,
			agentStyle(row.Agent).Render(row.Agent),
			session,
			row.Swears,
			dim.Render(fmt.Sprintf("%d messages", row.Messages))))
	}
	return b.String()
}

func (m Model) currentMax() int {
	switch m.tab {
	case 1:
		return len(m.filteredAgents())
	case 2:
		return len(m.filteredWords())
	case 3:
		return len(m.filteredSessions())
	default:
		return 0
	}
}

func (m Model) filteredAgents() []analytics.AgentRow {
	if strings.TrimSpace(m.filter) == "" {
		return m.report.Agents
	}
	names := make([]string, len(m.report.Agents))
	for i, row := range m.report.Agents {
		names[i] = row.Agent
	}
	matches := fuzzy.Find(m.filter, names)
	out := make([]analytics.AgentRow, 0, len(matches))
	for _, match := range matches {
		out = append(out, m.report.Agents[match.Index])
	}
	return out
}

func (m Model) filteredWords() []analytics.WordRow {
	if strings.TrimSpace(m.filter) == "" {
		return m.report.Words
	}
	names := make([]string, len(m.report.Words))
	for i, row := range m.report.Words {
		names[i] = row.Group + " " + row.Source
	}
	matches := fuzzy.Find(m.filter, names)
	out := make([]analytics.WordRow, 0, len(matches))
	for _, match := range matches {
		out = append(out, m.report.Words[match.Index])
	}
	return out
}

func (m Model) filteredSessions() []analytics.SessionRow {
	if strings.TrimSpace(m.filter) == "" {
		return m.report.Sessions
	}
	names := make([]string, len(m.report.Sessions))
	for i, row := range m.report.Sessions {
		names[i] = row.Agent + " " + row.Session + " " + row.Project
	}
	matches := fuzzy.Find(m.filter, names)
	out := make([]analytics.SessionRow, 0, len(matches))
	for _, match := range matches {
		out = append(out, m.report.Sessions[match.Index])
	}
	return out
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

func topVariants(rows []analytics.VariantRow, group string) string {
	var parts []string
	for _, row := range rows {
		if row.Group != group || row.Word == group {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %d", row.Word, row.Count))
		if len(parts) == 3 {
			break
		}
	}
	return strings.Join(parts, ", ")
}
