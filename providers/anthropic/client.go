package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	redditextract "github.com/teslashibe/reddit-extract"
)

const (
	defaultBaseURL        = "https://api.anthropic.com"
	defaultModel          = "claude-3-5-haiku-latest"
	defaultHTTPTimeout    = 120 * time.Second
	defaultAnthropicToken = 2048
	apiVersion            = "2023-06-01"
	betaHeader            = "message-batches-2024-09-24"
)

// Client is an Anthropic-backed implementation of redditextract LLM interfaces.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// New creates a Client configured for Anthropic Messages and Batch APIs.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    strings.TrimSpace(apiKey),
		baseURL:   defaultBaseURL,
		model:     defaultModel,
		maxTokens: defaultAnthropicToken,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithBaseURL overrides the API base URL.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if strings.TrimSpace(baseURL) != "" {
			c.baseURL = strings.TrimRight(baseURL, "/")
		}
	}
}

// WithModel sets the Anthropic model name.
func WithModel(model string) Option {
	return func(c *Client) {
		if strings.TrimSpace(model) != "" {
			c.model = model
		}
	}
}

// WithMaxTokens sets the default max_tokens value.
func WithMaxTokens(maxTokens int) Option {
	return func(c *Client) {
		if maxTokens > 0 {
			c.maxTokens = maxTokens
		}
	}
}

// WithHTTPClient injects a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(c *Client) {
		if httpClient != nil {
			c.httpClient = httpClient
		}
	}
}

// Complete executes a real-time Anthropic extraction request.
func (c *Client) Complete(ctx context.Context, req redditextract.CompletionRequest) (redditextract.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}

	payload := map[string]any{
		"model":       c.model,
		"max_tokens":  maxTokens,
		"temperature": req.Temperature,
		"system":      req.SystemPrompt,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": req.UserPrompt,
			},
		},
	}

	var respBody struct {
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := c.doJSON(ctx, http.MethodPost, "/v1/messages", payload, &respBody); err != nil {
		return redditextract.CompletionResponse{}, err
	}

	var text strings.Builder
	for _, block := range respBody.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	return redditextract.CompletionResponse{
		ID:      req.ID,
		Content: text.String(),
		Model:   respBody.Model,
		Usage: redditextract.Usage{
			InputTokens:  respBody.Usage.InputTokens,
			OutputTokens: respBody.Usage.OutputTokens,
		},
	}, nil
}

// SubmitBatch submits an asynchronous Anthropic message batch.
func (c *Client) SubmitBatch(ctx context.Context, reqs []redditextract.CompletionRequest) (string, error) {
	items := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		maxTokens := req.MaxTokens
		if maxTokens <= 0 {
			maxTokens = c.maxTokens
		}
		items = append(items, map[string]any{
			"custom_id": req.ID,
			"params": map[string]any{
				"model":       c.model,
				"max_tokens":  maxTokens,
				"temperature": req.Temperature,
				"system":      req.SystemPrompt,
				"messages": []map[string]string{
					{
						"role":    "user",
						"content": req.UserPrompt,
					},
				},
			},
		})
	}

	var respBody struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/messages/batches", map[string]any{"requests": items}, &respBody); err != nil {
		return "", err
	}
	if strings.TrimSpace(respBody.ID) == "" {
		return "", fmt.Errorf("anthropic batch response missing id")
	}
	return respBody.ID, nil
}

// PollBatch fetches coarse batch status.
func (c *Client) PollBatch(ctx context.Context, jobID string) (redditextract.BatchStatus, error) {
	var respBody struct {
		ID               string `json:"id"`
		ProcessingStatus string `json:"processing_status"`
		RequestCounts    struct {
			Processing int `json:"processing"`
			Succeeded  int `json:"succeeded"`
			Errored    int `json:"errored"`
			Canceled   int `json:"canceled"`
			Expired    int `json:"expired"`
		} `json:"request_counts"`
	}

	if err := c.doJSON(ctx, http.MethodGet, "/v1/messages/batches/"+jobID, nil, &respBody); err != nil {
		return redditextract.BatchStatus{}, err
	}

	state := redditextract.BatchRunning
	switch respBody.ProcessingStatus {
	case "ended":
		state = redditextract.BatchCompleted
	case "canceled", "expired":
		state = redditextract.BatchCanceled
	case "failed":
		state = redditextract.BatchFailed
	}

	completed := respBody.RequestCounts.Succeeded + respBody.RequestCounts.Errored + respBody.RequestCounts.Canceled + respBody.RequestCounts.Expired
	total := completed + respBody.RequestCounts.Processing
	failed := respBody.RequestCounts.Errored + respBody.RequestCounts.Canceled + respBody.RequestCounts.Expired

	return redditextract.BatchStatus{
		ID:        respBody.ID,
		State:     state,
		Total:     total,
		Completed: completed,
		Failed:    failed,
	}, nil
}

// GetBatchResults fetches and parses NDJSON batch result rows.
func (c *Client) GetBatchResults(ctx context.Context, jobID string) ([]redditextract.BatchItemResult, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/v1/messages/batches/"+jobID+"/results", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic batch results request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.readHTTPError(resp)
	}

	results := make([]redditextract.BatchItemResult, 0, 256)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 20*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var row struct {
			CustomID string `json:"custom_id"`
			Result   struct {
				Type    string `json:"type"`
				Message *struct {
					Model   string `json:"model"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
					Usage struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				} `json:"message"`
				Error *struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			} `json:"result"`
		}

		if err := json.Unmarshal(line, &row); err != nil {
			continue
		}

		item := redditextract.BatchItemResult{
			RequestID: row.CustomID,
		}

		if row.Result.Type != "succeeded" || row.Result.Message == nil {
			if row.Result.Error != nil {
				item.Error = fmt.Sprintf("%s: %s", row.Result.Error.Type, row.Result.Error.Message)
			} else {
				item.Error = fmt.Sprintf("batch result type=%s", row.Result.Type)
			}
			results = append(results, item)
			continue
		}

		var text strings.Builder
		for _, block := range row.Result.Message.Content {
			if block.Type == "text" {
				text.WriteString(block.Text)
			}
		}
		item.Response = redditextract.CompletionResponse{
			ID:      row.CustomID,
			Content: text.String(),
			Model:   row.Result.Message.Model,
			Usage: redditextract.Usage{
				InputTokens:  row.Result.Message.Usage.InputTokens,
				OutputTokens: row.Result.Message.Usage.OutputTokens,
			},
		}
		results = append(results, item)
	}
	if err := scanner.Err(); err != nil {
		return results, fmt.Errorf("scan batch results: %w", err)
	}
	return results, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("anthropic request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.readHTTPError(resp)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode anthropic response: %w", err)
		}
	}
	return nil
}

func (c *Client) newRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal anthropic request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create anthropic request: %w", err)
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("anthropic-beta", betaHeader)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (c *Client) readHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(data))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, message)
}
