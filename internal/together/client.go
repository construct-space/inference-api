package together

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to Together AI's OpenAI-compatible chat completions endpoint.
// Reusable for any OpenAI-compatible backend (Fireworks, DeepInfra, vLLM,
// self-hosted) by swapping BaseURL + APIKey.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// normalizeBaseURL accepts either `https://api.together.xyz`,
// `https://api.together.xyz/`, or `https://api.together.xyz/v1` and
// rewrites them into a canonical no-trailing-slash form that ends in
// `/v1` (the OpenAI-compat root). Without this a `TOGETHER_BASE_URL`
// like `https://api.together.xyz/` concatenated with `/chat/completions`
// produced a double-slash URL that Together returned 404 for.
func normalizeBaseURL(raw string) string {
	s := strings.TrimRight(strings.TrimSpace(raw), "/")
	if s == "" {
		return s
	}
	if !strings.HasSuffix(s, "/v1") {
		s += "/v1"
	}
	return s
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: normalizeBaseURL(baseURL),
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout: 0, // streaming responses can run long
		},
	}
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature *float64  `json:"temperature,omitempty"`
	TopP        *float64  `json:"top_p,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
	Tools       []Tool    `json:"tools,omitempty"`
	ToolChoice  any       `json:"tool_choice,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type AssistantMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int              `json:"index"`
		Message      AssistantMessage `json:"message"`
		FinishReason string           `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if c.APIKey == "" {
		return nil, errors.New("together: missing API key (set TOGETHER_API_KEY)")
	}
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("together: %d %s: %s", resp.StatusCode, resp.Status, string(errBody))
	}
	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Stream sends a streaming chat completion request and returns the response
// body so the caller can proxy the SSE bytes straight through to its own
// client. Caller MUST close the returned ReadCloser.
func (c *Client) Stream(ctx context.Context, req *ChatRequest) (io.ReadCloser, error) {
	if c.APIKey == "" {
		return nil, errors.New("together: missing API key (set TOGETHER_API_KEY)")
	}
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Accept", "text/event-stream")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("together: %d %s: %s", resp.StatusCode, resp.Status, string(errBody))
	}
	return resp.Body, nil
}

// PingTimeout is the wall-clock cap for a non-streaming completion. Streaming
// requests bypass the http.Client timeout entirely (it's 0) so they can run
// for the full generation length.
const PingTimeout = 120 * time.Second
