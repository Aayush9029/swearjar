package analytics

import "time"

type Report struct {
	GeneratedAt time.Time    `json:"generatedAt"`
	Duration    string       `json:"duration"`
	Scope       string       `json:"scope"`
	Totals      Totals       `json:"totals"`
	Agents      []AgentRow   `json:"agents"`
	Words       []WordRow    `json:"words"`
	Variants    []VariantRow `json:"variants"`
	Sessions    []SessionRow `json:"sessions"`
}

type Totals struct {
	Messages int64   `json:"messages"`
	Swears   int64   `json:"swears"`
	Sessions int64   `json:"sessions"`
	Chars    int64   `json:"chars"`
	Rate     float64 `json:"rate"`
}

type AgentRow struct {
	Agent    string  `json:"agent"`
	Messages int64   `json:"messages"`
	Swears   int64   `json:"swears"`
	Sessions int64   `json:"sessions"`
	Rate     float64 `json:"rate"`
}

type WordRow struct {
	Group string  `json:"group"`
	Count int64   `json:"count"`
	Share float64 `json:"share"`
}

type VariantRow struct {
	Group string `json:"group"`
	Word  string `json:"word"`
	Count int64  `json:"count"`
}

type SessionRow struct {
	Agent    string `json:"agent"`
	Session  string `json:"session"`
	Project  string `json:"project,omitempty"`
	Messages int64  `json:"messages"`
	Swears   int64  `json:"swears"`
}

type ProgressKind string

const (
	ProgressAdapterStart ProgressKind = "adapter_start"
	ProgressMessage      ProgressKind = "message"
	ProgressAdapterDone  ProgressKind = "adapter_done"
)

type Progress struct {
	Kind          ProgressKind
	Agent         string
	AdapterIndex  int
	AdapterTotal  int
	AdaptersDone  int64
	Messages      int64
	Swears        int64
	AgentMessages int64
	AgentSwears   int64
	LastWord      string
}

type ProgressFunc func(Progress)
