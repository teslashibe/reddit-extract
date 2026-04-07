package redditextract

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testExtraction struct {
	Trend string `json:"trend" desc:"Detected trend"`
	Score int    `json:"score" desc:"Trend score"`
}

type mockLLMClient struct {
	mu      sync.Mutex
	calls   map[string]int
	replies map[string]string
	errors  map[string][]error
	model   string
}

func newMockLLM() *mockLLMClient {
	return &mockLLMClient{
		calls:   make(map[string]int),
		replies: make(map[string]string),
		errors:  make(map[string][]error),
		model:   "mock-model",
	}
}

func (m *mockLLMClient) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.calls[req.ID]++
	if seq := m.errors[req.ID]; len(seq) > 0 {
		err := seq[0]
		m.errors[req.ID] = seq[1:]
		return CompletionResponse{}, err
	}

	reply := m.replies[req.ID]
	if reply == "" {
		reply = `{"trend":"default","score":1}`
	}
	return CompletionResponse{
		ID:      req.ID,
		Content: reply,
		Model:   m.model,
		Usage: Usage{
			InputTokens:  100,
			OutputTokens: 25,
		},
	}, nil
}

type mockBatchClient struct {
	*mockLLMClient

	mu         sync.Mutex
	nextJob    int
	jobs       map[string][]CompletionRequest
	pollCounts map[string]int
}

func newMockBatchClient() *mockBatchClient {
	return &mockBatchClient{
		mockLLMClient: newMockLLM(),
		jobs:          make(map[string][]CompletionRequest),
		pollCounts:    make(map[string]int),
	}
}

func (m *mockBatchClient) SubmitBatch(_ context.Context, reqs []CompletionRequest) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextJob++
	jobID := fmt.Sprintf("job-%d", m.nextJob)
	copied := make([]CompletionRequest, len(reqs))
	copy(copied, reqs)
	m.jobs[jobID] = copied
	return jobID, nil
}

func (m *mockBatchClient) PollBatch(_ context.Context, jobID string) (BatchStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	reqs := m.jobs[jobID]
	m.pollCounts[jobID]++
	state := BatchRunning
	if m.pollCounts[jobID] >= 2 {
		state = BatchCompleted
	}
	return BatchStatus{
		ID:        jobID,
		State:     state,
		Total:     len(reqs),
		Completed: len(reqs),
		Failed:    0,
	}, nil
}

func (m *mockBatchClient) GetBatchResults(_ context.Context, jobID string) ([]BatchItemResult, error) {
	m.mu.Lock()
	reqs := m.jobs[jobID]
	m.mu.Unlock()

	items := make([]BatchItemResult, 0, len(reqs))
	for _, req := range reqs {
		reply := m.replies[req.ID]
		if reply == "" {
			reply = `{"trend":"batch","score":5}`
		}
		items = append(items, BatchItemResult{
			RequestID: req.ID,
			Response: CompletionResponse{
				ID:      req.ID,
				Content: reply,
				Model:   "mock-batch",
			},
		})
	}
	return items, nil
}

func TestRunRealtimeExtractsTypedData(t *testing.T) {
	client := newMockLLM()
	records := []ContentRecord{
		{ID: "r1", Source: SourceReddit, Subreddit: "whoop", Title: "A", Body: "B", PublishedAt: time.Now().UTC()},
		{ID: "r2", Source: SourceReddit, Subreddit: "whoop", Title: "A", Body: "B", PublishedAt: time.Now().UTC()},
	}

	client.replies["r1-0"] = `{"trend":"sleep","score":8}`
	client.replies["r2-1"] = "not-json"

	var progressCalls atomic.Int64
	extractor := New(client, WithProgress(func(completed, total int) {
		if completed > total {
			t.Fatalf("completed cannot exceed total: %d > %d", completed, total)
		}
		progressCalls.Add(1)
	}))

	results, err := Run[testExtraction](context.Background(), extractor, records)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected first result error: %v", *results[0].Error)
	}
	if results[0].Data.Trend != "sleep" || results[0].Data.Score != 8 {
		t.Fatalf("first result data mismatch: %+v", results[0].Data)
	}
	if results[1].Error == nil {
		t.Fatalf("second result should contain parse error")
	}
	if got := progressCalls.Load(); got != 2 {
		t.Fatalf("progress calls = %d, want 2", got)
	}
}

func TestRunRetriesOnRateLimit(t *testing.T) {
	client := newMockLLM()
	record := ContentRecord{ID: "r1", Source: SourceReddit, Subreddit: "garmin", Title: "A", Body: "B", PublishedAt: time.Now().UTC()}

	client.errors["r1-0"] = []error{
		fmt.Errorf("429 rate limit"),
	}
	client.replies["r1-0"] = `{"trend":"retry-success","score":10}`

	extractor := New(client, WithMaxRetries(2))
	results, err := Run[testExtraction](context.Background(), extractor, []ContentRecord{record})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected error after retry: %v", *results[0].Error)
	}
	if results[0].Data.Trend != "retry-success" {
		t.Fatalf("unexpected trend: %q", results[0].Data.Trend)
	}
}

func TestRunDynamicWithRuntimeSchema(t *testing.T) {
	client := newMockLLM()
	client.replies["r1-0"] = `{"sentiment":"positive","themes":["recovery","sleep"]}`

	schema := NewDynamicSchemaBuilder("RuntimeSchema").
		AddStringField("sentiment", "Overall sentiment", true, "positive", "neutral", "negative").
		AddArrayField("themes", "Detected themes", false, map[string]any{"type": "string"}).
		Build()

	record := ContentRecord{ID: "r1", Source: SourceReddit, Subreddit: "whoop", Title: "A", Body: "B", PublishedAt: time.Now().UTC()}

	extractor := New(client)
	results, err := RunDynamic(context.Background(), extractor, []ContentRecord{record}, schema)
	if err != nil {
		t.Fatalf("RunDynamic() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Error != nil {
		t.Fatalf("unexpected result error: %v", *results[0].Error)
	}
	if got := results[0].Data["sentiment"]; got != "positive" {
		t.Fatalf("sentiment = %v, want positive", got)
	}
}

func TestRunBatchMode(t *testing.T) {
	client := newMockBatchClient()
	client.replies["r1-0"] = `{"trend":"batch-a","score":3}`
	client.replies["r2-1"] = `{"trend":"batch-b","score":4}`

	records := []ContentRecord{
		{ID: "r1", Source: SourceReddit, Subreddit: "whoop", Title: "A", Body: "B", PublishedAt: time.Now().UTC()},
		{ID: "r2", Source: SourceReddit, Subreddit: "whoop", Title: "A", Body: "B", PublishedAt: time.Now().UTC()},
	}

	var (
		stateMu sync.Mutex
		states  []BatchState
	)
	extractor := New(
		client,
		WithBatchMode(),
		WithBatchSize(1),
		WithPollInterval(10*time.Millisecond),
		WithMaxPollInterval(20*time.Millisecond),
		WithBatchProgress(func(_ string, status BatchStatus) {
			stateMu.Lock()
			defer stateMu.Unlock()
			states = append(states, status.State)
		}),
	)

	results, err := Run[testExtraction](context.Background(), extractor, records)
	if err != nil {
		t.Fatalf("Run() batch error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Error != nil || results[1].Error != nil {
		t.Fatalf("unexpected batch errors: %v | %v", results[0].Error, results[1].Error)
	}
	if results[0].Data.Trend != "batch-a" || results[1].Data.Trend != "batch-b" {
		t.Fatalf("unexpected batch data: %+v %+v", results[0].Data, results[1].Data)
	}
	if results[0].BatchJobID == "" || results[1].BatchJobID == "" {
		t.Fatalf("expected batch job IDs on results: %+v %+v", results[0], results[1])
	}

	stateMu.Lock()
	defer stateMu.Unlock()
	hasState := func(target BatchState) bool {
		for _, state := range states {
			if state == target {
				return true
			}
		}
		return false
	}
	if !hasState(BatchQueued) {
		t.Fatalf("batch progress should include queued state, states=%v", states)
	}
	if !hasState(BatchCompleted) {
		t.Fatalf("batch progress should include completed state, states=%v", states)
	}
}
