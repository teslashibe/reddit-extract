package redditextract

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const (
	defaultReaderInitialBuffer = 1024 * 1024
	defaultReaderMaxBuffer     = 20 * 1024 * 1024
)

// Reader parses reddit-scraper JSONL files into normalized content records.
type Reader struct {
	maxCommentDepth int
	maxComments     int
	minCommentScore int
	minPostScore    int
}

// NewReader returns a reader with sensible defaults for extraction workloads.
func NewReader(opts ...ReaderOption) *Reader {
	r := &Reader{
		maxCommentDepth: DefaultMaxCommentDepth,
		maxComments:     DefaultMaxComments,
		minCommentScore: DefaultMinCommentScore,
		minPostScore:    DefaultMinPostScore,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// ReadFile reads JSONL records from disk and returns normalized content records.
func (r *Reader) ReadFile(path string) ([]ContentRecord, ReadStats, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, ReadStats{}, fmt.Errorf("open input: %w", err)
	}
	defer f.Close()
	return r.ReadLines(f)
}

// ReadLines reads JSONL records from any stream.
func (r *Reader) ReadLines(reader io.Reader) ([]ContentRecord, ReadStats, error) {
	stats := ReadStats{
		SkipReasons: make(map[string]int),
	}

	var records []ContentRecord

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, defaultReaderInitialBuffer), defaultReaderMaxBuffer)

	for scanner.Scan() {
		stats.TotalLines++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var post Post
		if err := json.Unmarshal(line, &post); err != nil {
			stats.Errors++
			continue
		}

		if reason := r.shouldSkip(post); reason != "" {
			stats.addSkip(reason)
			continue
		}

		records = append(records, r.toRecord(post))
		stats.Parsed++
	}

	if err := scanner.Err(); err != nil {
		return records, stats, fmt.Errorf("scan input: %w", err)
	}

	return records, stats, nil
}

func (r *Reader) shouldSkip(post Post) string {
	if post.Stickied {
		return "stickied"
	}
	if post.Score < r.minPostScore {
		return "low_score"
	}

	body := strings.TrimSpace(post.SelfText)
	if body == "[removed]" || body == "[deleted]" {
		return "removed"
	}

	if !post.IsSelf && body == "" {
		return "link_only"
	}

	return ""
}

func (r *Reader) toRecord(post Post) ContentRecord {
	comments := r.flattenComments(post.Comments)
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].Score > comments[j].Score
	})

	if r.maxComments >= 0 && len(comments) > r.maxComments {
		comments = comments[:r.maxComments]
	}

	sourceURL := post.URL
	if post.Permalink != "" {
		if strings.HasPrefix(post.Permalink, "http://") || strings.HasPrefix(post.Permalink, "https://") {
			sourceURL = post.Permalink
		} else {
			sourceURL = "https://www.reddit.com" + post.Permalink
		}
	}

	return ContentRecord{
		ID:        post.ID,
		Source:    SourceReddit,
		SourceURL: sourceURL,
		Subreddit: post.Subreddit,
		Author:    post.Author,
		Title:     post.Title,
		Body:      post.SelfText,
		Comments:  comments,
		Metadata: map[string]any{
			"score":        post.Score,
			"upvote_ratio": post.UpvoteRatio,
			"num_comments": post.NumComments,
			"flair":        post.LinkFlairText,
		},
		PublishedAt: post.CreatedUTC,
	}
}

func (r *Reader) flattenComments(comments []Comment) []CommentInput {
	if len(comments) == 0 {
		return nil
	}

	flat := make([]CommentInput, 0, len(comments))
	for _, comment := range comments {
		r.flattenComment(comment, &flat)
	}
	return flat
}

func (r *Reader) flattenComment(comment Comment, out *[]CommentInput) {
	if comment.Depth > r.maxCommentDepth {
		return
	}
	if comment.Score < r.minCommentScore {
		return
	}

	author := strings.TrimSpace(comment.Author)
	body := strings.TrimSpace(comment.Body)

	if author == "" || body == "" {
		return
	}
	if author == "AutoModerator" || author == "[deleted]" {
		return
	}
	if body == "[deleted]" || body == "[removed]" {
		return
	}

	*out = append(*out, CommentInput{
		Author:      author,
		Body:        body,
		Score:       comment.Score,
		Depth:       comment.Depth,
		IsSubmitter: comment.IsSubmitter,
	})

	for _, reply := range comment.Replies {
		r.flattenComment(reply, out)
	}
}
