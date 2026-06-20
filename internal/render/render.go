package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/Aayush9029/swearjar/internal/analytics"
	"github.com/Aayush9029/swearjar/internal/ui"
)

func Report(w io.Writer, report analytics.Report, color bool) {
	c := palette(color)
	fmt.Fprintf(w, "  %s%sswearjar%s %s%s%s\n\n", c.bold, c.red, c.reset, c.dim, report.Scope, c.reset)

	if report.Totals.Messages == 0 {
		fmt.Fprintf(w, "  %sno local messages found%s\n\n", c.dim, c.reset)
		return
	}

	fmt.Fprintf(w, "  %s%s%d%s swears  %s%d user messages · %.1f%% · %s%s\n",
		c.bold, c.yellow, report.Totals.Swears, c.reset,
		c.dim, report.Totals.Messages, report.Totals.Rate, report.Duration, c.reset)

	if len(report.Agents) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %sagents%s %s(share of swears)%s\n", c.bold, c.reset, c.dim, c.reset)
		for _, row := range report.Agents {
			share := percent(row.Swears, report.Totals.Swears)
			fmt.Fprintf(w, "    %s%-10s%s %s%5d%s  %s%5.1f%% swears · %5d msgs · %5.1f%% rate%s  %s\n",
				agentColor(row.Agent, c), row.Agent, c.reset,
				c.bold, row.Swears, c.reset,
				c.dim, share, row.Messages, row.Rate, c.reset,
				barPercent(share, 22, color))
		}
	}

	if len(report.Words) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %stop words%s %s(share of swears)%s\n", c.bold, c.reset, c.dim, c.reset)
		for _, row := range report.Words {
			fmt.Fprintf(w, "    %s%-12s%s %s%5d%s  %s%5.1f%%%s  %s\n",
				c.yellow, row.Group, c.reset,
				c.bold, row.Count, c.reset,
				c.dim, row.Share, c.reset,
				barPercent(row.Share, 22, color))
		}
	} else {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "  %sthe jar is empty. not a single swear found.%s\n", c.green, c.reset)
	}
	fmt.Fprintln(w)
}

type colors struct {
	reset  string
	bold   string
	dim    string
	red    string
	green  string
	yellow string
	cyan   string
	blue   string
}

func palette(enabled bool) colors {
	if !enabled {
		return colors{}
	}
	return colors{
		reset:  ui.Reset,
		bold:   ui.Bold,
		dim:    ui.Dim,
		red:    ui.Red,
		green:  ui.Green,
		yellow: ui.Yellow,
		cyan:   ui.Cyan,
		blue:   ui.Blue,
	}
}

func agentColor(agent string, c colors) string {
	switch agent {
	case "codex":
		return c.green
	case "claude":
		return c.cyan
	case "amp":
		return c.blue
	case "file":
		return c.yellow
	default:
		return c.red
	}
}

func bar(value, max int64, width int, color bool) string {
	if width <= 0 {
		return ""
	}
	n := 0
	if max > 0 {
		n = int(float64(value) / float64(max) * float64(width))
	}
	if n == 0 && value > 0 {
		n = 1
	}
	if n > width {
		n = width
	}
	fill := strings.Repeat("█", n)
	empty := strings.Repeat("░", width-n)
	if !color {
		return fill + empty
	}
	return ui.Cyan + fill + ui.Dim + empty + ui.Reset
}

func barPercent(percent float64, width int, color bool) string {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	n := int(percent / 100 * float64(width))
	if percent > 0 && n == 0 {
		n = 1
	}
	fill := strings.Repeat("█", n)
	empty := strings.Repeat("░", width-n)
	if !color {
		return fill + empty
	}
	return ui.Cyan + fill + ui.Dim + empty + ui.Reset
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

func percent(num, denom int64) float64 {
	if denom <= 0 {
		return 0
	}
	return float64(num) / float64(denom) * 100
}
