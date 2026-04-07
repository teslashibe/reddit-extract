package redditextract

// Default reader thresholds.
const (
	DefaultMaxCommentDepth = 3
	DefaultMaxComments     = 50
	DefaultMinCommentScore = -5
	DefaultMinPostScore    = -10
)

// ReaderOption configures reader behavior.
type ReaderOption func(*Reader)

// WithMaxCommentDepth sets the maximum comment depth to include.
func WithMaxCommentDepth(depth int) ReaderOption {
	return func(r *Reader) {
		if depth >= 0 {
			r.maxCommentDepth = depth
		}
	}
}

// WithMaxComments sets the maximum number of comments included per record.
// A value of 0 excludes all comments.
func WithMaxComments(n int) ReaderOption {
	return func(r *Reader) {
		if n >= 0 {
			r.maxComments = n
		}
	}
}

// WithMinCommentScore sets the minimum comment score to include.
func WithMinCommentScore(score int) ReaderOption {
	return func(r *Reader) {
		r.minCommentScore = score
	}
}

// WithMinPostScore sets the minimum post score before a post is skipped.
func WithMinPostScore(score int) ReaderOption {
	return func(r *Reader) {
		r.minPostScore = score
	}
}
