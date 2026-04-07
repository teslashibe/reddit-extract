package redditextract

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultConcurrency     = 5
	defaultBatchSize       = 1000
	defaultPromptBodyChars = 8000
	defaultPromptComments  = 50
	defaultMaxTokens       = 2048
	defaultPollInterval    = 2 * time.Second
	defaultMaxPollInterval = 30 * time.Second
	defaultMaxRetries      = 3
)

// Extractor orchestrates schema-based extraction over content records.
type Extractor struct {
	client       LLMClient
	batchClient  BatchClient
	mode         extractMode
	concurrency  int
	batchSize    int
	maxBodyChars int
	maxComments  int
	maxTokens    int
	temperature  float64
	pollInterval time.Duration

	maxPollInterval time.Duration
	maxRetries      int
	systemPrompt    string
	progress        ProgressFunc
	batchProgress   BatchProgressFunc
}

// Result contains one extraction result for one source record.
type Result[T any] struct {
	SourceID    string    `json:"source_id"`
	Source      string    `json:"source"`
	SourceURL   string    `json:"source_url,omitempty"`
	Subreddit   string    `json:"subreddit,omitempty"`
	ExtractedAt time.Time `json:"extracted_at"`

	Model string `json:"model,omitempty"`
	Usage Usage  `json:"usage,omitempty"`

	Data  T      `json:"data"`
	Error string `json:"error,omitempty"`
}

// New builds an extractor from a provider client and options.
func New(client LLMClient, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		client:          client,
		mode:            modeRealtime,
		concurrency:     defaultConcurrency,
		batchSize:       defaultBatchSize,
		maxBodyChars:    defaultPromptBodyChars,
		maxComments:     defaultPromptComments,
		maxTokens:       defaultMaxTokens,
		temperature:     0,
		pollInterval:    defaultPollInterval,
		maxPollInterval: defaultMaxPollInterval,
		maxRetries:      defaultMaxRetries,
		systemPrompt:    DefaultSystemPrompt,
	}
	if batchClient, ok := client.(BatchClient); ok {
		e.batchClient = batchClient
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run extracts typed data from records using a schema generated from type T.
func Run[T any](ctx context.Context, extractor *Extractor, records []ContentRecord) ([]Result[T], error) {
	if extractor == nil {
		return nil, fmt.Errorf("extractor is nil")
	}
	schema, err := GenerateSchema[T]()
	if err != nil {
		return nil, err
	}
	return runWithSchema[T](ctx, extractor, records, schema)
}

// RunDynamic extracts dynamic map data using a runtime schema.
func RunDynamic(ctx context.Context, extractor *Extractor, records []ContentRecord, schema DynamicSchema) ([]Result[map[string]any], error) {
	if extractor == nil {
		return nil, fmt.Errorf("extractor is nil")
	}
	if len(schema.JSONSchema) == 0 {
		return nil, fmt.Errorf("dynamic schema cannot be empty")
	}
	return runWithSchema[map[string]any](ctx, extractor, records, schema)
}

func runWithSchema[T any](ctx context.Context, extractor *Extractor, records []ContentRecord, schema DynamicSchema) ([]Result[T], error) {
	switch extractor.mode {
	case modeBatch:
		return runBatch[T](ctx, extractor, records, schema)
	default:
		return runRealtime[T](ctx, extractor, records, schema)
	}
}

func runRealtime[T any](ctx context.Context, e *Extractor, records []ContentRecord, schema DynamicSchema) ([]Result[T], error) {
	results := make([]Result[T], len(records))
	if len(records) == 0 {
		return results, nil
	}

	systemPrompt := BuildSystemPrompt(e.systemPrompt, schema)

	var completed atomic.Int64
	jobs := make(chan int)
	workers := e.concurrency
	if workers <= 0 {
		workers = 1
	}
	if workers > len(records) {
		workers = len(records)
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				record := records[idx]
				results[idx] = baseResult[T](record)

				req := CompletionRequest{
					ID:           requestID(idx, record),
					SystemPrompt: systemPrompt,
					UserPrompt:   BuildUserPrompt(record, e.maxBodyChars, e.maxComments),
					MaxTokens:    e.maxTokens,
					Temperature:  e.temperature,
				}

				resp, err := e.completeWithRetry(ctx, req)
				if err != nil {
					results[idx].Error = err.Error()
					e.reportProgress(int(completed.Add(1)), len(records))
					continue
				}

				data, err := ParseResponse[T](resp.Content)
				if err != nil {
					results[idx].Error = fmt.Sprintf("parse response: %v", err)
					results[idx].Model = resp.Model
					results[idx].Usage = resp.Usage
					e.reportProgress(int(completed.Add(1)), len(records))
					continue
				}

				results[idx].Data = data
				results[idx].Model = resp.Model
				results[idx].Usage = resp.Usage
				e.reportProgress(int(completed.Add(1)), len(records))
			}
		}()
	}

sendLoop:
	for idx := range records {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- idx:
		}
	}
	close(jobs)
	wg.Wait()

	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func runBatch[T any](ctx context.Context, e *Extractor, records []ContentRecord, schema DynamicSchema) ([]Result[T], error) {
	if e.batchClient == nil {
		return nil, fmt.Errorf("batch mode requires a BatchClient")
	}

	results := make([]Result[T], len(records))
	if len(records) == 0 {
		return results, nil
	}

	systemPrompt := BuildSystemPrompt(e.systemPrompt, schema)
	requests := make([]CompletionRequest, len(records))
	idToIndex := make(map[string]int, len(records))
	for i, record := range records {
		reqID := requestID(i, record)
		requests[i] = CompletionRequest{
			ID:           reqID,
			SystemPrompt: systemPrompt,
			UserPrompt:   BuildUserPrompt(record, e.maxBodyChars, e.maxComments),
			MaxTokens:    e.maxTokens,
			Temperature:  e.temperature,
		}
		results[i] = baseResult[T](record)
		idToIndex[reqID] = i
	}

	type batchJob struct {
		id          string
		requestIDs  []string
		requestObjs []CompletionRequest
	}
	chunks := chunkRequests(requests, e.batchSize)
	jobs := make([]batchJob, 0, len(chunks))

	var completed atomic.Int64
	for _, chunk := range chunks {
		jobID, err := e.batchClient.SubmitBatch(ctx, chunk)
		if err != nil {
			for _, req := range chunk {
				idx := idToIndex[req.ID]
				results[idx].Error = fmt.Sprintf("submit batch: %v", err)
				e.reportProgress(int(completed.Add(1)), len(records))
			}
			continue
		}

		requestIDs := make([]string, len(chunk))
		for i, req := range chunk {
			requestIDs[i] = req.ID
		}
		jobs = append(jobs, batchJob{
			id:          jobID,
			requestIDs:  requestIDs,
			requestObjs: chunk,
		})
	}

	type jobOutput struct {
		jobID      string
		requestIDs []string
		items      []BatchItemResult
		err        error
	}

	outputs := make(chan jobOutput, len(jobs))
	var wg sync.WaitGroup
	for _, job := range jobs {
		job := job
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.waitForBatch(ctx, job.id); err != nil {
				outputs <- jobOutput{
					jobID:      job.id,
					requestIDs: job.requestIDs,
					err:        err,
				}
				return
			}
			items, err := e.batchClient.GetBatchResults(ctx, job.id)
			outputs <- jobOutput{
				jobID:      job.id,
				requestIDs: job.requestIDs,
				items:      items,
				err:        err,
			}
		}()
	}
	wg.Wait()
	close(outputs)

	for out := range outputs {
		if out.err != nil {
			for _, reqID := range out.requestIDs {
				idx := idToIndex[reqID]
				if results[idx].Error == "" {
					results[idx].Error = fmt.Sprintf("batch %s: %v", out.jobID, out.err)
					e.reportProgress(int(completed.Add(1)), len(records))
				}
			}
			continue
		}

		seen := make(map[string]bool, len(out.items))
		for _, item := range out.items {
			idx, ok := idToIndex[item.RequestID]
			if !ok {
				continue
			}
			seen[item.RequestID] = true

			if item.Error != "" {
				results[idx].Error = item.Error
				e.reportProgress(int(completed.Add(1)), len(records))
				continue
			}

			data, err := ParseResponse[T](item.Response.Content)
			if err != nil {
				results[idx].Error = fmt.Sprintf("parse response: %v", err)
				results[idx].Model = item.Response.Model
				results[idx].Usage = item.Response.Usage
				e.reportProgress(int(completed.Add(1)), len(records))
				continue
			}

			results[idx].Data = data
			results[idx].Model = item.Response.Model
			results[idx].Usage = item.Response.Usage
			e.reportProgress(int(completed.Add(1)), len(records))
		}

		for _, reqID := range out.requestIDs {
			if seen[reqID] {
				continue
			}
			idx := idToIndex[reqID]
			if results[idx].Error == "" {
				results[idx].Error = "missing batch result item"
				e.reportProgress(int(completed.Add(1)), len(records))
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}

func (e *Extractor) waitForBatch(ctx context.Context, jobID string) error {
	interval := e.pollInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	maxInterval := e.maxPollInterval
	if maxInterval <= 0 {
		maxInterval = defaultMaxPollInterval
	}

	for {
		status, err := e.batchClient.PollBatch(ctx, jobID)
		if err != nil {
			return err
		}
		if e.batchProgress != nil {
			e.batchProgress(jobID, status)
		}

		switch status.State {
		case BatchCompleted:
			return nil
		case BatchFailed, BatchCanceled:
			return fmt.Errorf("job %s finished with state %s", jobID, status.State)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		interval *= 2
		if interval > maxInterval {
			interval = maxInterval
		}
	}
}

func (e *Extractor) completeWithRetry(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	var (
		resp    CompletionResponse
		lastErr error
	)

	attempts := e.maxRetries + 1
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 0; attempt < attempts; attempt++ {
		resp, lastErr = e.client.Complete(ctx, req)
		if lastErr == nil {
			return resp, nil
		}
		if attempt == attempts-1 || !isRetryable(lastErr) {
			break
		}

		backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
		select {
		case <-ctx.Done():
			return CompletionResponse{}, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return CompletionResponse{}, lastErr
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporarily unavailable")
}

func (e *Extractor) reportProgress(completed, total int) {
	if e.progress != nil {
		e.progress(completed, total)
	}
}

func requestID(index int, record ContentRecord) string {
	if strings.TrimSpace(record.ID) == "" {
		return fmt.Sprintf("record-%d", index)
	}
	return fmt.Sprintf("%s-%d", record.ID, index)
}

func chunkRequests(requests []CompletionRequest, chunkSize int) [][]CompletionRequest {
	if chunkSize <= 0 {
		chunkSize = defaultBatchSize
	}
	if len(requests) == 0 {
		return nil
	}

	chunks := make([][]CompletionRequest, 0, (len(requests)+chunkSize-1)/chunkSize)
	for i := 0; i < len(requests); i += chunkSize {
		end := i + chunkSize
		if end > len(requests) {
			end = len(requests)
		}
		chunks = append(chunks, requests[i:end])
	}
	return chunks
}

func baseResult[T any](record ContentRecord) Result[T] {
	var zero T
	return Result[T]{
		SourceID:    record.ID,
		Source:      record.Source,
		SourceURL:   record.SourceURL,
		Subreddit:   record.Subreddit,
		ExtractedAt: time.Now().UTC(),
		Data:        zero,
	}
}
