package agent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Message struct {
	Agent     string
	Session   string
	Project   string
	Timestamp string
	Text      string
	Source    Source
}

type Options struct {
	Since      *time.Time
	Paths      []string
	SourceHook SourceHook
}

type Adapter interface {
	Name() string
	VisitMessages(context.Context, Options, func(Message) error) error
}

type Source struct {
	Agent   string
	Path    string
	Session string
	Project string
	Size    int64
	ModTime int64
}

type SourceHook interface {
	BeginSource(context.Context, Source) (bool, error)
	FinishSource(context.Context, Source) error
}

func All() []Adapter {
	return []Adapter{
		Codex{},
		Claude{},
		Amp{},
		Cline{},
		Pi{},
		Zed{},
	}
}

func Names() []string {
	names := make([]string, 0, len(All())+1)
	names = append(names, "file")
	for _, adapter := range All() {
		names = append(names, adapter.Name())
	}
	slices.Sort(names)
	return names
}

func Select(names []string, includeFiles bool) ([]Adapter, error) {
	if len(names) == 0 {
		adapters := All()
		if includeFiles {
			adapters = append([]Adapter{File{}}, adapters...)
		}
		return adapters, nil
	}

	known := map[string]Adapter{}
	if includeFiles {
		known["file"] = File{}
	}
	for _, adapter := range All() {
		known[adapter.Name()] = adapter
	}

	var selected []Adapter
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		adapter, ok := known[name]
		if !ok {
			return nil, fmt.Errorf("unknown agent: %s (available: %s)", name, strings.Join(Names(), ", "))
		}
		selected = append(selected, adapter)
	}
	return selected, nil
}
