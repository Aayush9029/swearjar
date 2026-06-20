package analytics

import (
	"context"
	"testing"

	"github.com/Aayush9029/swearjar/internal/agent"
	"github.com/Aayush9029/swearjar/internal/detector"
)

func TestStoreReportAggregatesWithDuckDB(t *testing.T) {
	ctx := context.Background()
	store, err := New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	d := detector.New()
	input := []agent.Message{
		{Agent: "codex", Session: "a", Text: "fuck this shit"},
		{Agent: "codex", Session: "a", Text: "clean prompt"},
		{Agent: "claude", Session: "b", Text: "what the fuck"},
	}
	for _, msg := range input {
		if err := store.Insert(ctx, msg, d.Detect(msg.Text)); err != nil {
			t.Fatal(err)
		}
	}

	report, err := store.Report(ctx, "test")
	if err != nil {
		t.Fatal(err)
	}
	if report.Totals.Messages != 3 || report.Totals.Swears != 3 {
		t.Fatalf("totals=%+v", report.Totals)
	}
	if len(report.Agents) != 2 || report.Agents[0].Agent != "codex" || report.Agents[0].Swears != 2 {
		t.Fatalf("agents=%+v", report.Agents)
	}
}
