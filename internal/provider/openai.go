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
	openAIBaseURL       = "https://api.openai.com/v1"
	openAIChatEndpoint  = "/chat/completions"
	openAIModelsEndpoint = "/models"
)

// OpenAI implements the Provider interface for OpenAI's API
type OpenAI struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// NewOpenAI creates a new OpenAI provider
func NewOpenAI(apiKey string) *OpenAI {
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: openAIBaseURL,
		client:  &http.Client{},
	}
}

// NewOpenAIWithClient creates a new OpenAI provider with a custom HTTP client
func NewOpenAIWithClient(apiKey string, client *http.Client) *OpenAI {
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: openAIBaseURL,
		client:  client,
	}
}

// Name returns the provider identifier
func (o *OpenAI) Name() string {
	return "openai"
}

// SupportsStreaming returns true as OpenAI supports streaming
func (o *OpenAI) SupportsStreaming() bool {
	return true
}

// openAIRequest is the request format for OpenAI's chat API
type openAIRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponse is the response format from OpenAI's chat API
type openAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *openAIError `json:"error,omitempty"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// openAIStreamResponse is the streaming response format
type openAIStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// Models returns available OpenAI models
func (o *OpenAI) Models(ctx context.Context) ([]string, error) {
	// Return static list for now to avoid unnecessary API calls
	models := make([]string, 0, len(DefaultModels["openai"]))
	for _, m := range DefaultModels["openai"] {
		models = append(models, m.ID)
	}
	return models, nil
}

// Send sends a chat request and returns the complete response
func (o *OpenAI) Send(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if o.apiKey == "" {
		return ChatResponse{}, ErrNoAPIKey
	}

	// Convert to OpenAI format
	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	openAIReq := openAIRequest{
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      false,
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+openAIChatEndpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(httpReq)
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
		var errResp openAIResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return ChatResponse{}, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return ChatResponse{}, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var openAIResp openAIResponse
	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return ChatResponse{}, ErrInvalidResponse
	}

	return ChatResponse{
		Content:      openAIResp.Choices[0].Message.Content,
		Model:        openAIResp.Model,
		FinishReason: openAIResp.Choices[0].FinishReason,
		Usage: Usage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
	}, nil
}

// Stream sends a chat request and streams the response
func (o *OpenAI) Stream(ctx context.Context, req ChatRequest, onDelta func(delta string)) error {
	if o.apiKey == "" {
		return ErrNoAPIKey
	}

	// Convert to OpenAI format
	messages := make([]openAIMessage, len(req.Messages))
	for i, m := range req.Messages {
		messages[i] = openAIMessage{
			Role:    string(m.Role),
			Content: m.Content,
		}
	}

	openAIReq := openAIRequest{
		Model:       req.Model,
		Messages:    messages,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+openAIChatEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := o.client.Do(httpReq)
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
		var errResp openAIResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
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

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			break
		}

		var streamResp openAIStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue // Skip malformed chunks
		}

		if len(streamResp.Choices) > 0 && streamResp.Choices[0].Delta.Content != "" {
			onDelta(streamResp.Choices[0].Delta.Content)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// SetAPIKey updates the API key
func (o *OpenAI) SetAPIKey(key string) {
	o.apiKey = key
}
