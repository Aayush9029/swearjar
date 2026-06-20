package agent

import (
	"context"
	"path/filepath"
)

type Cline struct{}

func (Cline) Name() string { return "cline" }

func (Cline) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	for _, root := range clineTaskRoots() {
		if err := walkFiles(root, func(path string) bool {
			return filepath.Base(path) == "api_conversation_history.json"
		}, func(path string) error {
			session := filepath.Base(filepath.Dir(path))
			src, err := fileSource("cline", path, session, "")
			if err != nil {
				return nil
			}
			read, err := beginSource(ctx, opts, src)
			if err != nil || !read {
				return err
			}
			var messages []struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
				TS      string `json:"ts"`
			}
			if err := readJSONFile(path, &messages); err != nil {
				return nil
			}
			for _, msg := range messages {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				if msg.Role != "user" {
					continue
				}
				text := contentToString(msg.Content)
				if text == "" || skipInjected(text) || !validSince(msg.TS, opts.Since) {
					continue
				}
				if err := visit(Message{Agent: "cline", Session: session, Timestamp: msg.TS, Text: text, Source: src}); err != nil {
					return err
				}
			}
			return finishSource(ctx, opts, src)
		}); err != nil {
			return err
		}
	}
	return nil
}

func clineTaskRoots() []string {
	roots := []string{}
	for _, base := range []string{
		homePath("Library", "Application Support", "Code", "User", "globalStorage"),
		homePath("Library", "Application Support", "Code - Insiders", "User", "globalStorage"),
		homePath("Library", "Application Support", "Cursor", "User", "globalStorage"),
	} {
		for _, ext := range []string{"saoudrizwan.claude-dev", "rooveterinaryinc.roo-cline"} {
			root := filepath.Join(base, ext, "tasks")
			if exists(root) {
				roots = append(roots, root)
			}
		}
	}
	if root := homePath(".cline", "data", "tasks"); exists(root) {
		roots = append(roots, root)
	}
	return roots
}
