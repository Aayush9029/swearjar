package agent

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

type Codex struct{}

func (Codex) Name() string { return "codex" }

func (Codex) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	root := homePath(".codex", "sessions")
	return walkFiles(root, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	}, func(path string) error {
		session := codexSessionFromPath(path)
		return scanJSONLines(path, func(entry map[string]any) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if stringField(entry, "type") != "response_item" {
				return nil
			}
			payload := asRecord(entry["payload"])
			if payload == nil || stringField(payload, "role") != "user" {
				return nil
			}
			text := contentToString(payload["content"])
			if text == "" || skipInjected(text) {
				return nil
			}
			timestamp := stringField(entry, "timestamp")
			if !validSince(timestamp, opts.Since) {
				return nil
			}
			return visit(Message{
				Agent:     "codex",
				Session:   session,
				Timestamp: timestamp,
				Text:      text,
			})
		})
	})
}

var codexSessionRe = regexp.MustCompile(`([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.jsonl$`)

func codexSessionFromPath(path string) string {
	name := filepath.Base(path)
	if m := codexSessionRe.FindStringSubmatch(name); len(m) == 2 {
		return m[1]
	}
	return strings.TrimSuffix(name, ".jsonl")
}
