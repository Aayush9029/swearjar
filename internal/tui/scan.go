package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Aayush9029/swearjar/internal/agent"
	"github.com/Aayush9029/swearjar/internal/analytics"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type scanModel struct {
	events       <-chan scanEvent
	cancel       context.CancelFunc
	scope        string
	startedAt    time.Time
	spinner      spinner.Model
	progress     progress.Model
	width        int
	height       int
	agentOrder   []string
	agents       map[string]scanAgent
	adapterTotal int
	adaptersDone int64
	messages     int64
	swears       int64
	lastAgent    string
	lastWord     string
	report       Model
	done         bool
	err          error
}

type scanAgent struct {
	Messages int64
	Swears   int64
	Active   bool
	Done     bool
}

type scanEvent struct {
	Progress    analytics.Progress
	HasProgress bool
	Report      analytics.Report
	Err         error
	Done        bool
}

type scanEventMsg scanEvent

func RunScan(ctx context.Context, adapters []agent.Adapter, opts agent.Options, scope string) error {
	ctx, cancel := context.WithCancel(ctx)
	events := make(chan scanEvent, 256)
	finished := make(chan struct{})

	go func() {
		defer close(finished)
		report, err := analytics.ScanWithProgress(ctx, adapters, opts, scope, func(p analytics.Progress) {
			select {
			case <-ctx.Done():
			case events <- scanEvent{Progress: p, HasProgress: true}:
			}
		})
		select {
		case <-ctx.Done():
		case events <- scanEvent{Report: report, Err: err, Done: true}:
		}
		close(events)
	}()

	s := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(selected))
	bar := progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage())
	model := scanModel{
		events:       events,
		cancel:       cancel,
		scope:        scope,
		startedAt:    time.Now(),
		spinner:      s,
		progress:     bar,
		agentOrder:   adapterNames(adapters),
		agents:       scanAgents(adapters),
		adapterTotal: len(adapters),
	}
	_, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	cancel()
	<-finished
	return err
}

func (m scanModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.waitEvent())
}

func (m scanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done && m.err == nil {
		updated, cmd := m.report.Update(msg)
		if next, ok := updated.(Model); ok {
			m.report = next
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = max(18, min(msg.Width-6, 64))
		m.report.width = msg.Width
		m.report.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	case scanEventMsg:
		if msg.HasProgress {
			m.applyProgress(msg.Progress)
			return m, m.waitEvent()
		}
		if msg.Done {
			if msg.Err != nil {
				m.done = true
				m.err = msg.Err
				return m, nil
			}
			m.done = true
			m.report = Model{report: msg.Report, width: m.width, height: m.height}
			return m, nil
		}
		return m, m.waitEvent()
	}
	return m, nil
}

func (m scanModel) View() string {
	if m.done && m.err == nil {
		return m.report.View()
	}
	if m.err != nil {
		return "\n  " + title.Render("swearjar") + "\n\n  " + lipgloss.NewStyle().Foreground(accent).Render(m.err.Error()) + "\n\n  " + dim.Render("q quit")
	}

	scope := m.scope
	if scope == "" {
		scope = "all local history"
	}
	elapsed := time.Since(m.startedAt).Round(time.Second)
	rate := 0.0
	if m.messages > 0 {
		rate = float64(m.swears) / float64(m.messages) * 100
	}
	adapterRatio := 0.0
	if m.adapterTotal > 0 {
		adapterRatio = float64(m.adaptersDone) / float64(m.adapterTotal)
	}

	var b strings.Builder
	b.WriteString("\n  ")
	b.WriteString(title.Render("swearjar"))
	b.WriteString(dim.Render("  " + scope))
	b.WriteString("\n\n  ")
	b.WriteString(m.spinner.View())
	b.WriteString(" ")
	b.WriteString(selected.Render(m.statusText()))
	b.WriteString("\n\n  ")
	b.WriteString(m.progress.ViewAs(adapterRatio))
	b.WriteString(dim.Render(fmt.Sprintf("  %d/%d agents", m.adaptersDone, m.adapterTotal)))
	b.WriteString("\n\n  ")
	b.WriteString(fmt.Sprintf("%s  %s  %s  %s",
		selected.Render(fmt.Sprintf("%d swears", m.swears)),
		dim.Render(fmt.Sprintf("%d messages", m.messages)),
		dim.Render(fmt.Sprintf("%.1f%%", rate)),
		dim.Render(elapsed.String())))
	if m.lastWord != "" {
		b.WriteString("\n  ")
		b.WriteString(dim.Render("last drop "))
		b.WriteString(selected.Render(m.lastWord))
	}
	b.WriteString("\n\n")
	b.WriteString(m.agentProgressView())
	b.WriteString("\n\n  ")
	b.WriteString(dim.Render("q quit"))
	return strings.TrimRight(b.String(), "\n")
}

func (m scanModel) waitEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.events
		if !ok {
			return scanEventMsg{Done: true}
		}
		return scanEventMsg(event)
	}
}

func (m *scanModel) applyProgress(p analytics.Progress) {
	if p.AdapterTotal > 0 {
		m.adapterTotal = p.AdapterTotal
	}
	if p.AdaptersDone > m.adaptersDone {
		m.adaptersDone = p.AdaptersDone
	}
	if p.Messages > 0 || p.Kind == analytics.ProgressMessage {
		m.messages = p.Messages
		m.swears = p.Swears
	}
	if p.LastWord != "" {
		m.lastWord = p.LastWord
	}
	if p.Agent != "" {
		m.lastAgent = p.Agent
		agentProgress := m.ensureAgent(p.Agent)
		switch p.Kind {
		case analytics.ProgressAdapterStart:
			agentProgress.Active = true
			agentProgress.Done = false
		case analytics.ProgressAdapterDone:
			agentProgress.Active = false
			agentProgress.Done = true
		case analytics.ProgressMessage:
			agentProgress.Active = true
			agentProgress.Messages = p.AgentMessages
			agentProgress.Swears = p.AgentSwears
		}
		m.agents[p.Agent] = agentProgress
	}
}

func (m *scanModel) ensureAgent(name string) scanAgent {
	if m.agents == nil {
		m.agents = map[string]scanAgent{}
	}
	if _, ok := m.agents[name]; !ok {
		m.agentOrder = append(m.agentOrder, name)
	}
	return m.agents[name]
}

func (m scanModel) statusText() string {
	if m.lastAgent != "" {
		return "rattling " + m.lastAgent
	}
	return "rattling the swear jar"
}

func (m scanModel) agentProgressView() string {
	visible := m.visibleAgents()
	if len(visible) == 0 {
		return dim.Render("  waiting for adapters")
	}
	maxSwears := int64(1)
	for _, name := range visible {
		row := m.agents[name]
		if row.Swears > maxSwears {
			maxSwears = row.Swears
		}
	}
	limit := min(len(visible), max(4, m.height-12))
	var b strings.Builder
	for _, name := range visible[:limit] {
		row := m.agents[name]
		state := dim.Render("queued")
		if row.Active {
			state = selected.Render("scanning")
		}
		if row.Done {
			state = lipgloss.NewStyle().Foreground(green).Render("done")
		}
		b.WriteString(fmt.Sprintf("  %-10s %5d  %-10s %s\n",
			agentStyle(name).Render(name),
			row.Swears,
			state,
			tuiBar(row.Swears, maxSwears, 18)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m scanModel) visibleAgents() []string {
	out := make([]string, 0, len(m.agentOrder))
	for _, name := range m.agentOrder {
		row := m.agents[name]
		if row.Messages == 0 && row.Swears == 0 && row.Done {
			continue
		}
		out = append(out, name)
	}
	return out
}

func adapterNames(adapters []agent.Adapter) []string {
	names := make([]string, 0, len(adapters))
	for _, adapter := range adapters {
		names = append(names, adapter.Name())
	}
	return names
}

func scanAgents(adapters []agent.Adapter) map[string]scanAgent {
	out := make(map[string]scanAgent, len(adapters))
	for _, adapter := range adapters {
		out[adapter.Name()] = scanAgent{}
	}
	return out
}
