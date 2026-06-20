package agent

import (
	"context"
	"path/filepath"
	"strings"
)

type Claude struct{}

func (Claude) Name() string { return "claude" }

func (Claude) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	root := homePath(".claude", "projects")
	return walkFiles(root, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	}, func(path string) error {
		session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		project := filepath.Base(filepath.Dir(path))
		if project == "subagents" {
			project = filepath.Base(filepath.Dir(filepath.Dir(path)))
		}
		return scanJSONLines(path, func(entry map[string]any) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			var content any
			switch {
			case stringField(entry, "type") == "user" || stringField(entry, "type") == "human":
				if message := asRecord(entry["message"]); message != nil {
					content = message["content"]
				}
			case stringField(entry, "role") == "user":
				content = entry["content"]
			default:
				return nil
			}

			text := contentToString(content)
			if text == "" || skipInjected(text) {
				return nil
			}
			timestamp := stringField(entry, "timestamp", "createdAt")
			if !validSince(timestamp, opts.Since) {
				return nil
			}
			return visit(Message{
				Agent:     "claude",
				Session:   session,
				Project:   project,
				Timestamp: timestamp,
				Text:      text,
			})
		})
	})
}
