// Package ai is a minimal OpenAI chat-completions client over net/http.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Message is one chat message. Content is either a string (plain text) or a
// []ContentPart (multimodal, e.g. text + image for vision requests).
type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

// ContentPart is one part of a multimodal message content array.
type ContentPart struct {
	Type     string    `json:"type"` // "text" or "image_url"
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL carries a data: or https: URL of an image.
type ImageURL struct {
	URL string `json:"url"`
}

// ResponseFormat requests structured output.
type ResponseFormat struct {
	Type       string         `json:"type"`
	JSONSchema *JSONSchemaDef `json:"json_schema,omitempty"`
}

// JSONSchemaDef is the json_schema response_format payload.
type JSONSchemaDef struct {
	Name   string         `json:"name"`
	Strict bool           `json:"strict"`
	Schema map[string]any `json:"schema"`
}

// ChatRequest is the chat-completions request body.
type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// Client talks to the OpenAI-compatible chat-completions endpoint.
type Client struct {
	BaseURL string // e.g. https://api.openai.com/v1
	APIKey  string
	Model   string
	HTTP    *http.Client
	log     *slog.Logger
}

// NewClient creates a client for the given model (default gpt-4o-mini).
func NewClient(baseURL, apiKey, model string) *Client {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{},
		log:     slog.Default(),
	}
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Chat sends a chat-completions request and returns the first choice's
// content. It retries up to 3 times on 429/5xx with exponential backoff and
// jitter; each attempt has a 30s timeout. Token usage is logged.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	const attempts = 3
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		content, retryable, err := c.chatOnce(ctx, body)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if !retryable || attempt == attempts {
			return "", err
		}
		backoff := time.Duration(1<<uint(attempt-1)) * time.Second
		jitter := time.Duration(rand.Int63n(int64(500 * time.Millisecond)))
		c.log.Warn("openai retryable error, backing off",
			"attempt", attempt, "backoff", backoff+jitter, "err", err)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff + jitter):
		}
	}
	return "", lastErr
}

func (c *Client) chatOnce(ctx context.Context, body []byte) (content string, retryable bool, err error) {
	attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(attemptCtx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return "", true, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", true, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", true, fmt.Errorf("openai: status %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("openai: status %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var cr chatResponse
	if err := json.Unmarshal(respBody, &cr); err != nil {
		return "", false, fmt.Errorf("decode response: %w", err)
	}
	if cr.Error != nil {
		return "", false, fmt.Errorf("openai error: %s (%s)", cr.Error.Message, cr.Error.Type)
	}
	if len(cr.Choices) == 0 {
		return "", false, errors.New("openai: no choices in response")
	}

	c.log.Info("openai usage", "model", req2model(body),
		"prompt_tokens", cr.Usage.PromptTokens,
		"completion_tokens", cr.Usage.CompletionTokens,
		"total_tokens", cr.Usage.TotalTokens)

	content, ok := cr.Choices[0].Message.Content.(string)
	if !ok {
		return "", false, errors.New("openai: non-string message content")
	}
	return content, false, nil
}

func req2model(body []byte) string {
	var r struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Model
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
