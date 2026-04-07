package redditextract

import "time"

type extractMode string

const (
	modeRealtime extractMode = "realtime"
	modeBatch    extractMode = "batch"
)

// ProgressFunc receives extraction progress updates.
type ProgressFunc func(completed, total int)

// BatchProgressFunc receives provider batch status updates.
type BatchProgressFunc func(jobID string, status BatchStatus)

// ExtractorOption configures extraction behavior.
type ExtractorOption func(*Extractor)

// WithBatchMode enables asynchronous batch extraction.
func WithBatchMode() ExtractorOption {
	return func(e *Extractor) { e.mode = modeBatch }
}

// WithRealTimeMode enables synchronous real-time extraction.
func WithRealTimeMode() ExtractorOption {
	return func(e *Extractor) { e.mode = modeRealtime }
}

// WithConcurrency sets worker concurrency for real-time mode.
func WithConcurrency(n int) ExtractorOption {
	return func(e *Extractor) {
		if n > 0 {
			e.concurrency = n
		}
	}
}

// WithBatchSize sets request count per submitted provider batch.
func WithBatchSize(n int) ExtractorOption {
	return func(e *Extractor) {
		if n > 0 {
			e.batchSize = n
		}
	}
}

// WithSystemPrompt sets a custom system prompt.
func WithSystemPrompt(prompt string) ExtractorOption {
	return func(e *Extractor) {
		e.systemPrompt = prompt
	}
}

// WithProgress sets a per-item progress callback.
func WithProgress(fn ProgressFunc) ExtractorOption {
	return func(e *Extractor) {
		e.progress = fn
	}
}

// WithBatchProgress sets a per-batch status callback.
func WithBatchProgress(fn BatchProgressFunc) ExtractorOption {
	return func(e *Extractor) {
		e.batchProgress = fn
	}
}

// WithPromptBodyLimit sets max body characters included in prompts.
func WithPromptBodyLimit(maxChars int) ExtractorOption {
	return func(e *Extractor) {
		if maxChars > 0 {
			e.maxBodyChars = maxChars
		}
	}
}

// WithPromptCommentLimit sets max comments included in prompts.
// A value of 0 excludes all comments from generated prompts.
func WithPromptCommentLimit(maxComments int) ExtractorOption {
	return func(e *Extractor) {
		if maxComments >= 0 {
			e.maxComments = maxComments
		}
	}
}

// WithMaxTokens sets request max tokens.
func WithMaxTokens(maxTokens int) ExtractorOption {
	return func(e *Extractor) {
		if maxTokens > 0 {
			e.maxTokens = maxTokens
		}
	}
}

// WithTemperature sets request temperature.
func WithTemperature(temperature float64) ExtractorOption {
	return func(e *Extractor) {
		if temperature >= 0 {
			e.temperature = temperature
		}
	}
}

// WithPollInterval sets initial poll interval for batch mode.
func WithPollInterval(interval time.Duration) ExtractorOption {
	return func(e *Extractor) {
		if interval > 0 {
			e.pollInterval = interval
		}
	}
}

// WithMaxPollInterval sets max backoff poll interval for batch mode.
func WithMaxPollInterval(interval time.Duration) ExtractorOption {
	return func(e *Extractor) {
		if interval > 0 {
			e.maxPollInterval = interval
		}
	}
}

// WithMaxRetries sets max retries for transient API failures.
func WithMaxRetries(retries int) ExtractorOption {
	return func(e *Extractor) {
		if retries >= 0 {
			e.maxRetries = retries
		}
	}
}
