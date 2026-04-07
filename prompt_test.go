package redditextract

import (
	"strings"
	"testing"
	"time"
)

func TestBuildSystemPromptIncludesSchema(t *testing.T) {
	schema := NewDynamicSchemaBuilder("TrendSchema").
		AddStringField("trend", "Detected trend", true).
		Build()

	prompt := BuildSystemPrompt("custom-base", schema)
	if !strings.Contains(prompt, "custom-base") {
		t.Fatalf("system prompt missing base prompt")
	}
	if !strings.Contains(prompt, `"trend"`) {
		t.Fatalf("system prompt missing schema json")
	}
}

func TestBuildUserPromptTruncatesBodyAndLimitsComments(t *testing.T) {
	record := ContentRecord{
		ID:          "abc123",
		Source:      SourceReddit,
		SourceURL:   "https://reddit.com/r/test/comments/abc123/example/",
		Subreddit:   "test",
		Title:       "Example title",
		Body:        strings.Repeat("x", 100),
		PublishedAt: time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		Comments: []CommentInput{
			{Author: "a", Body: "one", Score: 10, Depth: 0},
			{Author: "b", Body: "two", Score: 9, Depth: 1, IsSubmitter: true},
			{Author: "c", Body: "three", Score: 8, Depth: 2},
		},
	}

	prompt := BuildUserPrompt(record, 20, 2)
	if !strings.Contains(prompt, "[...truncated]") {
		t.Fatalf("expected body truncation marker")
	}
	if !strings.Contains(prompt, "[...1 more comments omitted]") {
		t.Fatalf("expected omitted comments marker")
	}
	if strings.Contains(prompt, "three") {
		t.Fatalf("third comment should be omitted")
	}
}

func TestBuildUserPromptAllowsZeroCommentLimit(t *testing.T) {
	record := ContentRecord{
		ID:          "abc123",
		Source:      SourceReddit,
		Subreddit:   "test",
		Title:       "Example title",
		Body:        "Body",
		PublishedAt: time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		Comments: []CommentInput{
			{Author: "a", Body: "one", Score: 10, Depth: 0},
			{Author: "b", Body: "two", Score: 9, Depth: 0},
		},
	}

	prompt := BuildUserPrompt(record, 8000, 0)
	if strings.Contains(prompt, "one") || strings.Contains(prompt, "two") {
		t.Fatalf("comment bodies should be omitted when maxComments=0")
	}
	if !strings.Contains(prompt, "[...2 more comments omitted]") {
		t.Fatalf("expected omitted comments marker for zero limit")
	}
}
