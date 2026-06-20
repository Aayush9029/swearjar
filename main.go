package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/Aayush9029/swearjar/internal/agent"
	"github.com/Aayush9029/swearjar/internal/analytics"
	"github.com/Aayush9029/swearjar/internal/render"
	"github.com/Aayush9029/swearjar/internal/tui"
	"github.com/Aayush9029/swearjar/internal/ui"
)

var version = "dev"

const day = 24 * time.Hour

type options struct {
	agents     []string
	paths      []string
	since      *time.Time
	scope      string
	jsonOutput bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		ui.Fatalf("%s", err)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		if ui.IsTTY() {
			return cmdUI(ctx, nil)
		}
		return cmdScan(ctx, nil)
	}

	switch args[0] {
	case "scan":
		return cmdScan(ctx, args[1:])
	case "ui", "tui":
		return cmdUI(ctx, args[1:])
	case "--version", "-v", "version":
		fmt.Printf("swearjar %s\n", version)
		return nil
	case "--help", "-h", "help":
		showHelp()
		return nil
	default:
		return cmdScan(ctx, args)
	}
}

func cmdScan(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	report, err := buildReport(ctx, opts)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	render.Report(os.Stdout, report, ui.IsTTY())
	return nil
}

func cmdUI(ctx context.Context, args []string) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	adapters, err := agent.Select(opts.agents, len(opts.paths) > 0)
	if err != nil {
		return err
	}
	if !ui.IsTTY() {
		report, err := scanAdapters(ctx, adapters, opts)
		if err != nil {
			return err
		}
		render.Report(os.Stdout, report, false)
		return nil
	}
	return tui.RunScan(ctx, adapters, agent.Options{
		Since: opts.since,
		Paths: opts.paths,
	}, opts.scope)
}

func buildReport(ctx context.Context, opts options) (analytics.Report, error) {
	adapters, err := agent.Select(opts.agents, len(opts.paths) > 0)
	if err != nil {
		return analytics.Report{}, err
	}
	return scanAdapters(ctx, adapters, opts)
}

func scanAdapters(ctx context.Context, adapters []agent.Adapter, opts options) (analytics.Report, error) {
	return analytics.Scan(ctx, adapters, agent.Options{
		Since: opts.since,
		Paths: opts.paths,
	}, opts.scope)
}

func parseOptions(args []string) (options, error) {
	opts := options{scope: "all local history"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--agent", "-a":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--agent requires a value")
			}
			opts.agents = append(opts.agents, splitList(args[i])...)
		case "--path", "-p":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--path requires a value")
			}
			opts.paths = append(opts.paths, args[i])
		case "--since", "-s":
			i++
			if i >= len(args) {
				return opts, fmt.Errorf("--since requires a value")
			}
			since, err := parseTime(args[i])
			if err != nil {
				return opts, err
			}
			opts.since = &since
			opts.scope = "since " + since.Format("2006-01-02")
		case "--day", "--days":
			days := 1
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				n, err := strconv.Atoi(args[i])
				if err != nil || n < 1 {
					return opts, fmt.Errorf("invalid days: %s", args[i])
				}
				days = n
			}
			since := time.Now().Add(-time.Duration(days) * day)
			opts.since = &since
			if days == 1 {
				opts.scope = "last 1 day"
			} else {
				opts.scope = fmt.Sprintf("last %d days", days)
			}
		case "--week":
			since := time.Now().Add(-7 * day)
			opts.since = &since
			opts.scope = "last 7 days"
		case "--month":
			since := time.Now().Add(-30 * day)
			opts.since = &since
			opts.scope = "last 30 days"
		case "--json", "-j":
			opts.jsonOutput = true
		case "--help", "-h":
			showHelp()
			os.Exit(0)
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, fmt.Errorf("unknown option: %s", arg)
			}
			opts.paths = append(opts.paths, arg)
		}
	}
	if len(opts.paths) > 0 && len(opts.agents) == 0 {
		opts.agents = []string{"file"}
		opts.scope = "files"
	}
	return opts, nil
}

func splitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseTime(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		t, err := time.Parse(layout, value)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date: %s", value)
}

func showHelp() {
	fmt.Println(`swearjar - count how much you swear at coding agents

USAGE
  swearjar                         open the interactive TUI
  swearjar scan                    print a compact report
  swearjar scan --agent codex      scan one agent
  swearjar scan --week             scan the last 7 days
  swearjar scan ./chat.jsonl       scan files or folders

FLAGS
  --agent, -a <name>               codex, claude, amp, cline, pi, zed, file
  --path, -p <path>                file or folder to scan
  --since, -s <date>               ISO date or YYYY-MM-DD
  --day, --days [n]                last n days (default 1)
  --week                           last 7 days
  --month                          last 30 days
  --json, -j                       print JSON
  --version, -v                    print version`)
}
