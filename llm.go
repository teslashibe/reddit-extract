package redditextract

import "context"

// CompletionRequest is a provider-agnostic request for one extraction call.
type CompletionRequest struct {
	// ID is an optional caller-supplied identifier used for batch correlation.
	ID string `json:"id,omitempty"`

	// SystemPrompt defines the high-level extraction behavior.
	SystemPrompt string `json:"system_prompt"`

	// UserPrompt contains the concrete post payload to extract from.
	UserPrompt string `json:"user_prompt"`

	// MaxTokens bounds the output length.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls sampling variability.
	Temperature float64 `json:"temperature,omitempty"`
}

// Usage captures provider token accounting when available.
type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// CompletionResponse is a provider-agnostic completion payload.
type CompletionResponse struct {
	// ID mirrors CompletionRequest.ID when the provider supports it.
	ID string `json:"id,omitempty"`

	// Content is the raw model text response.
	Content string `json:"content"`

	// Model is the resolved provider model name.
	Model string `json:"model,omitempty"`

	// Usage is optional token accounting.
	Usage Usage `json:"usage,omitempty"`
}

// LLMClient is the minimal provider contract required for real-time extraction.
type LLMClient interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}
