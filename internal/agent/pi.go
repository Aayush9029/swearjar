package agent

import (
	"context"
	"path/filepath"
	"strings"
)

type Pi struct{}

func (Pi) Name() string { return "pi" }

func (Pi) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	root := homePath(".pi", "agent", "sessions")
	return walkFiles(root, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	}, func(path string) error {
		session := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		project := filepath.Base(filepath.Dir(path))
		return scanJSONLines(path, func(entry map[string]any) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if stringField(entry, "type") != "message" {
				return nil
			}
			message := asRecord(entry["message"])
			if message == nil || stringField(message, "role") != "user" {
				return nil
			}
			text := contentToString(message["content"])
			if text == "" || skipInjected(text) {
				return nil
			}
			timestamp := stringField(entry, "timestamp")
			if timestamp == "" {
				timestamp = stringField(message, "timestamp")
			}
			if !validSince(timestamp, opts.Since) {
				return nil
			}
			return visit(Message{
				Agent:     "pi",
				Session:   session,
				Project:   project,
				Timestamp: timestamp,
				Text:      text,
			})
		})
	})
}
