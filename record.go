package redditextract

import "time"

// SourceReddit is the source label used for records read from reddit-scraper JSONL.
const SourceReddit = "reddit"

// ContentRecord is the normalized post shape used by the extraction pipeline.
//
// It is intentionally generic so downstream consumers can map it to their own
// domain models without depending on Reddit-specific tree structures.
type ContentRecord struct {
	ID          string         `json:"id"`
	Source      string         `json:"source"`
	SourceURL   string         `json:"source_url,omitempty"`
	Subreddit   string         `json:"subreddit"`
	Author      string         `json:"author,omitempty"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	Comments    []CommentInput `json:"comments,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	PublishedAt time.Time      `json:"published_at"`
}

// CommentInput is a flattened comment shape for prompt construction.
type CommentInput struct {
	Author      string `json:"author"`
	Body        string `json:"body"`
	Score       int    `json:"score"`
	Depth       int    `json:"depth"`
	IsSubmitter bool   `json:"is_submitter"`
}
