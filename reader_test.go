package redditextract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestReaderReadLinesFiltersAndParses(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)

	valid := Post{
		ID:          "p1",
		Subreddit:   "whoop",
		Title:       "Recovery trend",
		Author:      "alice",
		SelfText:    "I noticed better recovery.",
		Score:       42,
		UpvoteRatio: 0.96,
		NumComments: 6,
		CreatedUTC:  now,
		Permalink:   "/r/whoop/comments/p1/recovery_trend/",
		IsSelf:      true,
		Comments: []Comment{
			{
				ID:     "c1",
				Author: "bob",
				Body:   "Same result here.",
				Score:  10,
				Depth:  0,
				Replies: []Comment{
					{
						ID:          "c2",
						Author:      "charlie",
						Body:        "How long did it take?",
						Score:       1,
						Depth:       1,
						IsSubmitter: false,
					},
				},
			},
			{
				ID:     "c3",
				Author: "AutoModerator",
				Body:   "Read the rules",
				Score:  100,
				Depth:  0,
			},
			{
				ID:     "c4",
				Author: "dana",
				Body:   "downvoted",
				Score:  -50,
				Depth:  0,
			},
			{
				ID:     "c5",
				Author: "erin",
				Body:   "[removed]",
				Score:  10,
				Depth:  0,
			},
			{
				ID:     "c6",
				Author: "frank",
				Body:   "too deep",
				Score:  99,
				Depth:  4,
			},
		},
	}

	stickied := Post{ID: "p2", Title: "mod post", Stickied: true, IsSelf: true}
	lowScore := Post{ID: "p3", Title: "downvoted", Score: -11, IsSelf: true}
	removed := Post{ID: "p4", Title: "removed", SelfText: "[removed]", IsSelf: true}
	linkOnly := Post{ID: "p5", Title: "link", IsSelf: false, SelfText: ""}

	var input strings.Builder
	mustWriteJSONLine(t, &input, valid)
	mustWriteJSONLine(t, &input, stickied)
	mustWriteJSONLine(t, &input, lowScore)
	mustWriteJSONLine(t, &input, removed)
	mustWriteJSONLine(t, &input, linkOnly)
	input.WriteString("{this is invalid json}\n")

	reader := NewReader()
	records, stats, err := reader.ReadLines(strings.NewReader(input.String()))
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}

	if stats.TotalLines != 6 {
		t.Fatalf("TotalLines = %d, want 6", stats.TotalLines)
	}
	if stats.Parsed != 1 {
		t.Fatalf("Parsed = %d, want 1", stats.Parsed)
	}
	if stats.Skipped != 4 {
		t.Fatalf("Skipped = %d, want 4", stats.Skipped)
	}
	if stats.Errors != 1 {
		t.Fatalf("Errors = %d, want 1", stats.Errors)
	}

	if got := stats.SkipReasons["stickied"]; got != 1 {
		t.Fatalf("skip reason stickied = %d, want 1", got)
	}
	if got := stats.SkipReasons["low_score"]; got != 1 {
		t.Fatalf("skip reason low_score = %d, want 1", got)
	}
	if got := stats.SkipReasons["removed"]; got != 1 {
		t.Fatalf("skip reason removed = %d, want 1", got)
	}
	if got := stats.SkipReasons["link_only"]; got != 1 {
		t.Fatalf("skip reason link_only = %d, want 1", got)
	}

	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	record := records[0]

	if record.Source != SourceReddit {
		t.Fatalf("record.Source = %q, want %q", record.Source, SourceReddit)
	}
	if record.SourceURL != "https://www.reddit.com/r/whoop/comments/p1/recovery_trend/" {
		t.Fatalf("record.SourceURL = %q", record.SourceURL)
	}
	if record.Subreddit != "whoop" {
		t.Fatalf("record.Subreddit = %q, want whoop", record.Subreddit)
	}
	if len(record.Comments) != 2 {
		t.Fatalf("len(record.Comments) = %d, want 2", len(record.Comments))
	}
	if record.Comments[0].Author != "bob" || record.Comments[0].Score != 10 {
		t.Fatalf("first comment mismatch: %+v", record.Comments[0])
	}
	if record.Comments[1].Author != "charlie" || record.Comments[1].Score != 1 {
		t.Fatalf("second comment mismatch: %+v", record.Comments[1])
	}
}

func TestReaderOptionsAreApplied(t *testing.T) {
	now := time.Date(2026, 4, 6, 12, 0, 0, 0, time.UTC)
	post := Post{
		ID:         "p1",
		Subreddit:  "garmin",
		Title:      "Deep thread",
		SelfText:   "Body",
		Score:      -50,
		CreatedUTC: now,
		IsSelf:     true,
		Comments: []Comment{
			{ID: "c1", Author: "a", Body: "score2", Score: 2, Depth: 0},
			{ID: "c2", Author: "b", Body: "score1", Score: 1, Depth: 0},
			{ID: "c3", Author: "c", Body: "score-1", Score: -1, Depth: 0},
			{
				ID:     "c4",
				Author: "d",
				Body:   "deep",
				Score:  10,
				Depth:  0,
				Replies: []Comment{
					{ID: "c5", Author: "e", Body: "too deep", Score: 100, Depth: 2},
				},
			},
		},
	}

	var input strings.Builder
	mustWriteJSONLine(t, &input, post)

	reader := NewReader(
		WithMinPostScore(-100),
		WithMinCommentScore(0),
		WithMaxCommentDepth(1),
		WithMaxComments(1),
	)

	records, stats, err := reader.ReadLines(strings.NewReader(input.String()))
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}
	if stats.Parsed != 1 {
		t.Fatalf("Parsed = %d, want 1", stats.Parsed)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if len(records[0].Comments) != 1 {
		t.Fatalf("len(records[0].Comments) = %d, want 1", len(records[0].Comments))
	}
	if got := records[0].Comments[0].Body; got != "deep" {
		t.Fatalf("top comment after sorting/cap = %q, want deep", got)
	}
}

func TestReaderHandlesLargeLines(t *testing.T) {
	largeBody := strings.Repeat("x", 2*1024*1024) // 2MB
	post := Post{
		ID:         "large-1",
		Subreddit:  "whoop",
		Title:      "large line",
		SelfText:   largeBody,
		Score:      5,
		IsSelf:     true,
		CreatedUTC: time.Now().UTC(),
	}

	var input bytes.Buffer
	mustWriteJSONLineBuffer(t, &input, post)

	reader := NewReader()
	records, stats, err := reader.ReadLines(&input)
	if err != nil {
		t.Fatalf("ReadLines() error = %v", err)
	}
	if stats.Parsed != 1 || len(records) != 1 {
		t.Fatalf("parsed records mismatch: parsed=%d len=%d", stats.Parsed, len(records))
	}
	if records[0].Body != largeBody {
		t.Fatal("large body did not round-trip")
	}
}

func mustWriteJSONLine(t *testing.T, b *strings.Builder, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	b.Write(data)
	b.WriteString("\n")
}

func mustWriteJSONLineBuffer(t *testing.T, b *bytes.Buffer, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	b.Write(data)
	b.WriteString("\n")
}
