package agent

import (
	"context"
	"path/filepath"
	"strings"
)

type Amp struct{}

func (Amp) Name() string { return "amp" }

func (Amp) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	root := homePath(".local", "share", "amp", "threads")
	if xdg := strings.TrimSpace(getenv("XDG_DATA_HOME")); xdg != "" {
		root = filepath.Join(xdg, "amp", "threads")
	}
	return walkFiles(root, func(path string) bool {
		return strings.HasSuffix(path, ".json")
	}, func(path string) error {
		var thread struct {
			Messages []struct {
				Role      string `json:"role"`
				Content   any    `json:"content"`
				Timestamp string `json:"timestamp"`
				CreatedAt string `json:"createdAt"`
			} `json:"messages"`
		}
		if err := readJSONFile(path, &thread); err != nil {
			return nil
		}
		session := strings.TrimSuffix(filepath.Base(path), ".json")
		for _, msg := range thread.Messages {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if msg.Role != "user" {
				continue
			}
			text := contentToString(msg.Content)
			if text == "" || skipInjected(text) {
				continue
			}
			timestamp := msg.Timestamp
			if timestamp == "" {
				timestamp = msg.CreatedAt
			}
			if !validSince(timestamp, opts.Since) {
				continue
			}
			if err := visit(Message{Agent: "amp", Session: session, Timestamp: timestamp, Text: text}); err != nil {
				return err
			}
		}
		return nil
	})
}
