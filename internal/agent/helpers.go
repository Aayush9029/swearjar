package agent

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxLineSize = 32 * 1024 * 1024

func homePath(parts ...string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	all := append([]string{home}, parts...)
	return filepath.Join(all...)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func validSince(timestamp string, since *time.Time) bool {
	if since == nil || timestamp == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		t, err = time.Parse(time.RFC3339, timestamp)
	}
	if err != nil {
		return true
	}
	return !t.Before(*since)
}

func readJSONFile(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(out)
}

func scanJSONLines(path string, visit func(map[string]any) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if err := visit(entry); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func readLines(path string, visit func(string) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxLineSize)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if err := visit(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func readWhole(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func asRecord(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func stringField(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func contentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			record := asRecord(item)
			if record == nil {
				continue
			}
			if text := stringField(record, "text", "content"); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func skipInjected(text string) bool {
	text = strings.TrimSpace(text)
	for _, prefix := range []string{
		"<environment_context>",
		"<permissions instructions>",
		"<skill>",
		"<system",
		"<developer",
		"<summary>",
		"Knowledge cutoff:",
	} {
		if strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return false
}

func walkFiles(root string, want func(string) bool, visit func(string) error) error {
	if root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		if want(root) {
			return visit(root)
		}
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if want(path) {
			return visit(path)
		}
		return nil
	})
}
