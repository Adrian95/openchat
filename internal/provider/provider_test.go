package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAISend(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected 'Bearer test-key', got '%s'", auth)
		}

		// Parse request body
		var req openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "gpt-4o" {
			t.Errorf("expected model 'gpt-4o', got '%s'", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Errorf("expected 1 message, got %d", len(req.Messages))
		}

		// Send response
		resp := openAIResponse{
			ID:    "chatcmpl-123",
			Model: "gpt-4o",
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Index:        0,
					Message:      openAIMessage{Role: "assistant", Content: "Hello! How can I help you?"},
					FinishReason: "stop",
				},
			},
			Usage: struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			}{
				PromptTokens:     10,
				CompletionTokens: 20,
				TotalTokens:      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with mock server
	provider := NewOpenAI("test-key")
	provider.baseURL = server.URL

	// Test Send
	req := ChatRequest{
		Model: "gpt-4o",
		Messages: []Message{
			{Role: RoleUser, Content: "Hello"},
		},
	}

	resp, err := provider.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("expected 'Hello! How can I help you?', got '%s'", resp.Content)
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got '%s'", resp.Model)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got '%s'", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 30 {
		t.Errorf("expected total_tokens 30, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIStream(t *testing.T) {
	// Create mock SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		// Parse request to verify streaming is requested
		var req openAIRequest
		json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			t.Error("expected stream=true in request")
		}

		// Send streaming response
		chunks := []string{
			`{"id":"chatcmpl-123","choices":[{"delta":{"role":"assistant"},"index":0}]}`,
			`{"id":"chatcmpl-123","choices":[{"delta":{"content":"Hello"},"index":0}]}`,
			`{"id":"chatcmpl-123","choices":[{"delta":{"content":" World"},"index":0}]}`,
			`{"id":"chatcmpl-123","choices":[{"delta":{"content":"!"},"index":0,"finish_reason":"stop"}]}`,
		}

		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			w.(http.Flusher).Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAI("test-key")
	provider.baseURL = server.URL

	var received strings.Builder
	err := provider.Stream(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	}, func(delta string) {
		received.WriteString(delta)
	})

	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if received.String() != "Hello World!" {
		t.Errorf("expected 'Hello World!', got '%s'", received.String())
	}
}

func TestOpenAINoAPIKey(t *testing.T) {
	provider := NewOpenAI("")

	_, err := provider.Send(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	})

	if err != ErrNoAPIKey {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestOpenAIRateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	provider := NewOpenAI("test-key")
	provider.baseURL = server.URL

	_, err := provider.Send(context.Background(), ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	})

	if err != ErrRateLimited {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
}

func TestAnthropicSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/messages" {
			t.Errorf("expected /messages, got %s", r.URL.Path)
		}

		apiKey := r.Header.Get("x-api-key")
		if apiKey != "test-key" {
			t.Errorf("expected 'test-key', got '%s'", apiKey)
		}

		version := r.Header.Get("anthropic-version")
		if version != anthropicAPIVersion {
			t.Errorf("expected '%s', got '%s'", anthropicAPIVersion, version)
		}

		// Parse request
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "claude-3-5-sonnet-20241022" {
			t.Errorf("expected model 'claude-3-5-sonnet-20241022', got '%s'", req.Model)
		}
		if req.System != "You are helpful" {
			t.Errorf("expected system 'You are helpful', got '%s'", req.System)
		}

		// Send response
		resp := anthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "Hello! I'm Claude."},
			},
			StopReason: "end_turn",
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{
				InputTokens:  15,
				OutputTokens: 25,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := NewAnthropic("test-key")
	provider.baseURL = server.URL

	req := ChatRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			{Role: RoleSystem, Content: "You are helpful"},
			{Role: RoleUser, Content: "Hello"},
		},
	}

	resp, err := provider.Send(context.Background(), req)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if resp.Content != "Hello! I'm Claude." {
		t.Errorf("expected 'Hello! I'm Claude.', got '%s'", resp.Content)
	}
	if resp.Model != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected model 'claude-3-5-sonnet-20241022', got '%s'", resp.Model)
	}
}

func TestAnthropicStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		events := []struct {
			event string
			data  string
		}{
			{"message_start", `{"type":"message_start","message":{"id":"msg_123"}}`},
			{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi"}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`},
			{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}`},
			{"content_block_stop", `{"type":"content_block_stop","index":0}`},
			{"message_stop", `{"type":"message_stop"}`},
		}

		for _, e := range events {
			w.Write([]byte("event: " + e.event + "\n"))
			w.Write([]byte("data: " + e.data + "\n\n"))
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	provider := NewAnthropic("test-key")
	provider.baseURL = server.URL

	var received strings.Builder
	err := provider.Stream(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	}, func(delta string) {
		received.WriteString(delta)
	})

	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	if received.String() != "Hi there!" {
		t.Errorf("expected 'Hi there!', got '%s'", received.String())
	}
}

func TestAnthropicNoAPIKey(t *testing.T) {
	provider := NewAnthropic("")

	_, err := provider.Send(context.Background(), ChatRequest{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	})

	if err != ErrNoAPIKey {
		t.Errorf("expected ErrNoAPIKey, got %v", err)
	}
}

func TestProviderRegistry(t *testing.T) {
	registry := NewRegistry()

	// Register providers
	openai := NewOpenAI("key1")
	anthropic := NewAnthropic("key2")

	registry.Register(openai)
	registry.Register(anthropic)

	// Test Get
	p, ok := registry.Get("openai")
	if !ok {
		t.Error("expected to find openai provider")
	}
	if p.Name() != "openai" {
		t.Errorf("expected name 'openai', got '%s'", p.Name())
	}

	p, ok = registry.Get("anthropic")
	if !ok {
		t.Error("expected to find anthropic provider")
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got '%s'", p.Name())
	}

	// Test non-existent
	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("expected not to find nonexistent provider")
	}

	// Test List
	names := registry.List()
	if len(names) != 2 {
		t.Errorf("expected 2 providers, got %d", len(names))
	}
}

func TestProviderModels(t *testing.T) {
	ctx := context.Background()

	openai := NewOpenAI("test-key")
	models, err := openai.Models(ctx)
	if err != nil {
		t.Fatalf("OpenAI.Models failed: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected OpenAI models")
	}

	anthropic := NewAnthropic("test-key")
	models, err = anthropic.Models(ctx)
	if err != nil {
		t.Fatalf("Anthropic.Models failed: %v", err)
	}
	if len(models) == 0 {
		t.Error("expected Anthropic models")
	}
}

func TestProviderSupportsStreaming(t *testing.T) {
	openai := NewOpenAI("test-key")
	if !openai.SupportsStreaming() {
		t.Error("expected OpenAI to support streaming")
	}

	anthropic := NewAnthropic("test-key")
	if !anthropic.SupportsStreaming() {
		t.Error("expected Anthropic to support streaming")
	}
}

func TestContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		select {
		case <-r.Context().Done():
			return
		}
	}))
	defer server.Close()

	provider := NewOpenAI("test-key")
	provider.baseURL = server.URL

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := provider.Send(ctx, ChatRequest{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	})

	if err != ErrContextCanceled {
		t.Errorf("expected ErrContextCanceled, got %v", err)
	}
}

func TestAPIErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Invalid model specified",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAI("test-key")
	provider.baseURL = server.URL

	_, err := provider.Send(context.Background(), ChatRequest{
		Model:    "invalid-model",
		Messages: []Message{{Role: RoleUser, Content: "Hi"}},
	})

	if err == nil {
		t.Error("expected error for invalid request")
	}
	if !strings.Contains(err.Error(), "Invalid model") {
		t.Errorf("expected error message to contain 'Invalid model', got: %v", err)
	}
}
