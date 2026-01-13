// Package provider defines the interface for AI providers and common types.
package provider

import (
	"context"
	"errors"
)

// Common errors
var (
	ErrNoAPIKey        = errors.New("no API key configured for provider")
	ErrInvalidResponse = errors.New("invalid response from provider")
	ErrStreamClosed    = errors.New("stream closed unexpectedly")
	ErrRateLimited     = errors.New("rate limited by provider")
	ErrContextCanceled = errors.New("context canceled")
)

// Role represents the role of a message sender
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents a request to an AI provider
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

// ChatResponse represents a response from an AI provider
type ChatResponse struct {
	Content      string `json:"content"`
	Model        string `json:"model"`
	FinishReason string `json:"finish_reason,omitempty"`
	Usage        Usage  `json:"usage,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Provider defines the interface that all AI providers must implement
type Provider interface {
	// Name returns the provider's identifier (e.g., "openai", "anthropic")
	Name() string

	// Models returns a list of available models for this provider
	// Returns nil if the provider doesn't support model listing
	Models(ctx context.Context) ([]string, error)

	// Send sends a chat request and returns the complete response
	Send(ctx context.Context, req ChatRequest) (ChatResponse, error)

	// Stream sends a chat request and calls onDelta for each token received
	// This enables real-time streaming of responses
	Stream(ctx context.Context, req ChatRequest, onDelta func(delta string)) error

	// SupportsStreaming returns true if the provider supports streaming
	SupportsStreaming() bool
}

// ModelInfo contains information about a specific model
type ModelInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	MaxTokens   int    `json:"max_tokens"`
	Description string `json:"description,omitempty"`
}

// Registry holds registered providers
type Registry struct {
	providers map[string]Provider
}

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider to the registry
func (r *Registry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get retrieves a provider by name
func (r *Registry) Get(name string) (Provider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// List returns all registered provider names
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// DefaultModels returns commonly used models for each provider
var DefaultModels = map[string][]ModelInfo{
	"openai": {
		{ID: "gpt-4o", Name: "GPT-4o", Provider: "openai", MaxTokens: 128000, Description: "Most capable GPT-4 model"},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", Provider: "openai", MaxTokens: 128000, Description: "Affordable GPT-4 model"},
		{ID: "gpt-4-turbo", Name: "GPT-4 Turbo", Provider: "openai", MaxTokens: 128000, Description: "GPT-4 Turbo with vision"},
		{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo", Provider: "openai", MaxTokens: 16385, Description: "Fast and cost-effective"},
	},
	"anthropic": {
		{ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", Provider: "anthropic", MaxTokens: 200000, Description: "Most capable Claude model"},
		{ID: "claude-3-5-sonnet-20241022", Name: "Claude 3.5 Sonnet", Provider: "anthropic", MaxTokens: 200000, Description: "Best balance of intelligence and speed"},
		{ID: "claude-3-5-haiku-20241022", Name: "Claude 3.5 Haiku", Provider: "anthropic", MaxTokens: 200000, Description: "Fast and affordable"},
		{ID: "claude-3-opus-20240229", Name: "Claude 3 Opus", Provider: "anthropic", MaxTokens: 200000, Description: "Previous flagship model"},
	},
	"gemini": {
		{ID: "gemini-3-pro-preview", Name: "Gemini 3 Pro", Provider: "gemini", MaxTokens: 1048576, Description: "Most advanced reasoning model with dynamic thinking"},
		{ID: "gemini-3-flash-preview", Name: "Gemini 3 Flash", Provider: "gemini", MaxTokens: 1048576, Description: "Fast model with dynamic thinking, 64k output"},
		{ID: "gemini-2.5-pro-preview-06-05", Name: "Gemini 2.5 Pro", Provider: "gemini", MaxTokens: 1048576, Description: "Most capable Gemini with thinking"},
		{ID: "gemini-2.5-flash-preview-05-20", Name: "Gemini 2.5 Flash", Provider: "gemini", MaxTokens: 1048576, Description: "Fast Gemini with thinking"},
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", Provider: "gemini", MaxTokens: 1048576, Description: "Next-gen fast model"},
		{ID: "gemini-2.0-flash-thinking-exp", Name: "Gemini 2.0 Flash Thinking", Provider: "gemini", MaxTokens: 1048576, Description: "Experimental thinking model"},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", Provider: "gemini", MaxTokens: 2097152, Description: "2M context window"},
		{ID: "gemini-1.5-flash", Name: "Gemini 1.5 Flash", Provider: "gemini", MaxTokens: 1048576, Description: "Fast and efficient"},
	},
}
