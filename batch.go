package redditextract

import "context"

// BatchState describes a provider batch job lifecycle state.
type BatchState string

const (
	// BatchQueued indicates the provider accepted the batch but has not started.
	BatchQueued BatchState = "queued"
	// BatchRunning indicates the provider is actively processing the batch.
	BatchRunning BatchState = "running"
	// BatchCompleted indicates all available results can be fetched.
	BatchCompleted BatchState = "completed"
	// BatchFailed indicates the provider marked the job as failed.
	BatchFailed BatchState = "failed"
	// BatchCanceled indicates the job was canceled.
	BatchCanceled BatchState = "canceled"
)

// BatchStatus contains coarse-grained progress for one batch job.
type BatchStatus struct {
	ID        string     `json:"id"`
	State     BatchState `json:"state"`
	Total     int        `json:"total"`
	Completed int        `json:"completed"`
	Failed    int        `json:"failed"`
}

// BatchItemResult is one provider result item for one request ID.
type BatchItemResult struct {
	RequestID string             `json:"request_id"`
	Response  CompletionResponse `json:"response"`
	Error     string             `json:"error,omitempty"`
}

// BatchClient extends LLMClient with asynchronous batch operations.
type BatchClient interface {
	LLMClient
	SubmitBatch(ctx context.Context, reqs []CompletionRequest) (jobID string, err error)
	PollBatch(ctx context.Context, jobID string) (BatchStatus, error)
	GetBatchResults(ctx context.Context, jobID string) ([]BatchItemResult, error)
}
