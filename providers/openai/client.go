package openai

import (
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
	defaultBaseURL     = "https://api.openai.com"
	defaultModel       = "gpt-4.1-mini"
	defaultHTTPTimeout = 120 * time.Second
	defaultMaxTokens   = 2048
)

// Client is an OpenAI-backed implementation of redditextract.LLMClient.
type Client struct {
	apiKey     string
	baseURL    string
	model      string
	maxTokens  int
	httpClient *http.Client
}

// Option configures a Client.
type Option func(*Client)

// New creates an OpenAI client.
func New(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:    strings.TrimSpace(apiKey),
		baseURL:   defaultBaseURL,
		model:     defaultModel,
		maxTokens: defaultMaxTokens,
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

// WithModel sets the OpenAI model.
func WithModel(model string) Option {
	return func(c *Client) {
		if strings.TrimSpace(model) != "" {
			c.model = model
		}
	}
}

// WithMaxTokens sets a default completion cap.
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

// Complete executes a chat completion request against OpenAI.
func (c *Client) Complete(ctx context.Context, req redditextract.CompletionRequest) (redditextract.CompletionResponse, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.maxTokens
	}

	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": req.SystemPrompt,
			},
			{
				"role":    "user",
				"content": req.UserPrompt,
			},
		},
		"max_tokens": maxTokens,
	}
	if req.Temperature >= 0 {
		payload["temperature"] = req.Temperature
	}

	var respBody struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := c.doJSON(ctx, http.MethodPost, "/v1/chat/completions", payload, &respBody); err != nil {
		return redditextract.CompletionResponse{}, err
	}
	if len(respBody.Choices) == 0 {
		return redditextract.CompletionResponse{}, fmt.Errorf("openai response has no choices")
	}

	return redditextract.CompletionResponse{
		ID:      req.ID,
		Content: respBody.Choices[0].Message.Content,
		Model:   respBody.Model,
		Usage: redditextract.Usage{
			InputTokens:  respBody.Usage.PromptTokens,
			OutputTokens: respBody.Usage.CompletionTokens,
		},
	}, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal openai request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create openai request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("openai request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai API error (status %d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode openai response: %w", err)
		}
	}
	return nil
}
