package agent

import (
	"context"
	"path/filepath"
	"strings"
)

type Zed struct{}

func (Zed) Name() string { return "zed" }

func (Zed) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	root := homePath("Library", "Application Support", "Zed", "conversations")
	return walkFiles(root, func(path string) bool {
		return strings.HasSuffix(path, ".json")
	}, func(path string) error {
		session := strings.TrimSuffix(filepath.Base(path), ".json")
		src, err := fileSource("zed", path, session, "")
		if err != nil {
			return nil
		}
		read, err := beginSource(ctx, opts, src)
		if err != nil || !read {
			return err
		}
		var conversation struct {
			Messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"messages"`
		}
		if err := readJSONFile(path, &conversation); err != nil {
			return nil
		}
		for _, msg := range conversation.Messages {
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
			if err := visit(Message{Agent: "zed", Session: session, Text: text, Source: src}); err != nil {
				return err
			}
		}
		return finishSource(ctx, opts, src)
	})
}
