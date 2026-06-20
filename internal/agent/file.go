package agent

import (
	"context"
	"path/filepath"
	"strings"
)

type File struct{}

func (File) Name() string { return "file" }

func (File) VisitMessages(ctx context.Context, opts Options, visit func(Message) error) error {
	for _, root := range opts.Paths {
		if err := walkFiles(root, supportedInputFile, func(path string) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			ext := strings.ToLower(filepath.Ext(path))
			session := filepath.Base(path)
			src, err := fileSource("file", path, session, "")
			if err != nil {
				return nil
			}
			read, err := beginSource(ctx, opts, src)
			if err != nil || !read {
				return err
			}
			if ext == ".jsonl" {
				if err := scanJSONLines(path, func(entry map[string]any) error {
					text := fileJSONText(entry)
					if text == "" || skipInjected(text) {
						return nil
					}
					return visit(Message{
						Agent:     "file",
						Session:   session,
						Timestamp: stringField(entry, "timestamp", "createdAt"),
						Text:      text,
						Source:    src,
					})
				}); err != nil {
					return err
				}
				return finishSource(ctx, opts, src)
			}
			if ext == ".txt" || ext == ".md" {
				text, err := readWhole(path)
				if err != nil || strings.TrimSpace(text) == "" {
					return nil
				}
				if err := visit(Message{Agent: "file", Session: session, Text: text, Source: src}); err != nil {
					return err
				}
				return finishSource(ctx, opts, src)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func supportedInputFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".txt", ".md", ".jsonl":
		return true
	default:
		return false
	}
}

func fileJSONText(entry map[string]any) string {
	if text := stringField(entry, "text", "prompt", "message"); text != "" {
		return text
	}
	if message := asRecord(entry["message"]); message != nil {
		if role := stringField(message, "role"); role != "" && role != "user" {
			return ""
		}
		return contentToString(message["content"])
	}
	if role := stringField(entry, "role"); role != "" && role != "user" {
		return ""
	}
	return contentToString(entry["content"])
}
