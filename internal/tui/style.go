package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	accent   = lipgloss.Color("205")
	yellow   = lipgloss.Color("221")
	green    = lipgloss.Color("114")
	cyan     = lipgloss.Color("81")
	muted    = lipgloss.Color("245")
	dim      = lipgloss.NewStyle().Foreground(muted)
	selected = lipgloss.NewStyle().Bold(true).Foreground(yellow)
	title    = lipgloss.NewStyle().Bold(true).Foreground(cyan)
)

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
