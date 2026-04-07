package redditextract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteJSONLAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "results.jsonl")

	results := []Result[map[string]any]{
		{
			SourceID:    "p1",
			Source:      SourceReddit,
			Subreddit:   "whoop",
			ExtractedAt: time.Now().UTC(),
			Data:        map[string]any{"trend": "sleep"},
		},
		{
			SourceID:    "p2",
			Source:      SourceReddit,
			Subreddit:   "garmin",
			ExtractedAt: time.Now().UTC(),
			Data:        map[string]any{"trend": "recovery"},
		},
	}

	if err := WriteJSONL(path, results); err != nil {
		t.Fatalf("WriteJSONL() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count = %d, want 2", len(lines))
	}
}

func TestWriteJSONLAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "append.jsonl")

	first := []Result[map[string]any]{
		{
			SourceID:    "p1",
			Source:      SourceReddit,
			ExtractedAt: time.Now().UTC(),
			Data:        map[string]any{"x": 1},
		},
	}
	second := []Result[map[string]any]{
		{
			SourceID:    "p2",
			Source:      SourceReddit,
			ExtractedAt: time.Now().UTC(),
			Data:        map[string]any{"x": 2},
		},
	}

	if err := WriteJSONL(path, first); err != nil {
		t.Fatalf("initial WriteJSONL() error = %v", err)
	}
	if err := WriteJSONL(path, second, WithAppendMode()); err != nil {
		t.Fatalf("append WriteJSONL() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("line count after append = %d, want 2", len(lines))
	}
}
