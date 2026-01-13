package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicBaseURL      = "https://api.anthropic.com/v1"
	anthropicChatEndpoint = "/messages"
	anthropicAPIVersion   = "2023-06-01"
)

// Anthropic implements the Provider interface for Anthropic's API
type Anthropic struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewAnthropic creates a new Anthropic provider
func NewAnthropic(apiKey string) *Anthropic {
	return &Anthropic{
		apiKey:  apiKey,
		baseURL: anthropicBaseURL,
		client:  &http.Client{},
	}
}

// NewAnthropicWithClient creates a new Anthropic provider with a custom HTTP client
func NewAnthropicWithClient(apiKey string, client *http.Client) *Anthropic {
	return &Anthropic{
		apiKey:  apiKey,
		baseURL: anthropicBaseURL,
		client:  client,
	}
}

// Name returns the provider identifier
func (a *Anthropic) Name() string {
	return "anthropic"
}

// SupportsStreaming returns true as Anthropic supports streaming
func (a *Anthropic) SupportsStreaming() bool {
	return true
}

// anthropicRequest is the request format for Anthropic's messages API
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicResponse is the response format from Anthropic's messages API
type anthropicResponse struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Role         string `json:"role"`
	Model        string `json:"model"`
	Content      []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *anthropicError `json:"error,omitempty"`
}

type anthropicError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// anthropicStreamEvent represents a streaming event from Anthropic
type anthropicStreamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index,omitempty"`
	ContentBlock *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content_block,omitempty"`
	Delta *struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
	} `json:"delta,omitempty"`
	Message *anthropicResponse `json:"message,omitempty"`
}

// Models returns available Anthropic models
func (a *Anthropic) Models(ctx context.Context) ([]string, error) {
	// Return static list as Anthropic doesn't have a models endpoint
	models := make([]string, 0, len(DefaultModels["anthropic"]))
	for _, m := range DefaultModels["anthropic"] {
		models = append(models, m.ID)
	}
	return models, nil
}

// Send sends a chat request and returns the complete response
func (a *Anthropic) Send(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if a.apiKey == "" {
		return ChatResponse{}, ErrNoAPIKey
	}

	// Extract system message and convert to Anthropic format
	var systemPrompt string
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		// Anthropic only accepts "user" and "assistant" roles
		role := string(m.Role)
		if role == "tool" {
			role = "user" // Map tool responses to user
		}
		messages = append(messages, anthropicMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	// Ensure max_tokens is set (required by Anthropic)
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	anthropicReq := anthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+anthropicChatEndpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return ChatResponse{}, ErrContextCanceled
		}
		return ChatResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return ChatResponse{}, ErrRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error anthropicError `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return ChatResponse{}, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return ChatResponse{}, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(anthropicResp.Content) == 0 {
		return ChatResponse{}, ErrInvalidResponse
	}

	// Extract text content
	var content strings.Builder
	for _, c := range anthropicResp.Content {
		if c.Type == "text" {
			content.WriteString(c.Text)
		}
	}

	return ChatResponse{
		Content:      content.String(),
		Model:        anthropicResp.Model,
		FinishReason: anthropicResp.StopReason,
		Usage: Usage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
	}, nil
}

// Stream sends a chat request and streams the response
func (a *Anthropic) Stream(ctx context.Context, req ChatRequest, onDelta func(delta string)) error {
	if a.apiKey == "" {
		return ErrNoAPIKey
	}

	// Extract system message and convert to Anthropic format
	var systemPrompt string
	messages := make([]anthropicMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == RoleSystem {
			systemPrompt = m.Content
			continue
		}
		role := string(m.Role)
		if role == "tool" {
			role = "user"
		}
		messages = append(messages, anthropicMessage{
			Role:    role,
			Content: m.Content,
		})
	}

	// Ensure max_tokens is set
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	anthropicReq := anthropicRequest{
		Model:       req.Model,
		Messages:    messages,
		System:      systemPrompt,
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+anthropicChatEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return ErrContextCanceled
		}
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		var errResp struct {
			Error anthropicError `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ErrContextCanceled
		default:
		}

		line := scanner.Text()

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse event type
		if strings.HasPrefix(line, "event: ") {
			eventType := strings.TrimPrefix(line, "event: ")
			if eventType == "message_stop" {
				break
			}
			continue
		}

		// Parse data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var event anthropicStreamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// Handle content_block_delta events
		if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
			onDelta(event.Delta.Text)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// SetAPIKey updates the API key
func (a *Anthropic) SetAPIKey(key string) {
	a.apiKey = key
}
