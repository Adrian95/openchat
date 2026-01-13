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
	geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"
)

// Gemini implements the Provider interface for Google's Gemini API
type Gemini struct {
	apiKey        string
	baseURL       string
	client        *http.Client
	enableSearch  bool // Enable Google Search grounding
	enableThought bool // Enable thinking/reasoning mode
}

// NewGemini creates a new Gemini provider
func NewGemini(apiKey string) *Gemini {
	return &Gemini{
		apiKey:        apiKey,
		baseURL:       geminiBaseURL,
		client:        &http.Client{},
		enableSearch:  false,
		enableThought: false,
	}
}

// NewGeminiWithOptions creates a new Gemini provider with options
func NewGeminiWithOptions(apiKey string, enableSearch, enableThought bool) *Gemini {
	return &Gemini{
		apiKey:        apiKey,
		baseURL:       geminiBaseURL,
		client:        &http.Client{},
		enableSearch:  enableSearch,
		enableThought: enableThought,
	}
}

// Name returns the provider identifier
func (g *Gemini) Name() string {
	return "gemini"
}

// SupportsStreaming returns true as Gemini supports streaming
func (g *Gemini) SupportsStreaming() bool {
	return true
}

// SetSearchEnabled enables or disables Google Search grounding
func (g *Gemini) SetSearchEnabled(enabled bool) {
	g.enableSearch = enabled
}

// SetThinkingEnabled enables or disables thinking mode
func (g *Gemini) SetThinkingEnabled(enabled bool) {
	g.enableThought = enabled
}

// Gemini API request/response types
type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []geminiTool           `json:"tools,omitempty"`
}

type geminiContent struct {
	Role  string        `json:"role,omitempty"`
	Parts []geminiPart  `json:"parts"`
}

type geminiPart struct {
	Text    string           `json:"text,omitempty"`
	Thought *geminiThought   `json:"thought,omitempty"`
}

type geminiThought struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	Temperature       float64 `json:"temperature,omitempty"`
	MaxOutputTokens   int     `json:"maxOutputTokens,omitempty"`
	ThinkingConfig    *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingBudget int    `json:"thinkingBudget,omitempty"` // Token budget for thinking (Gemini 2.x)
	ThinkingLevel  string `json:"thinkingLevel,omitempty"`  // Thinking level: "low", "medium", "high" (Gemini 3.x)
}

type geminiTool struct {
	GoogleSearch *geminiGoogleSearch `json:"googleSearch,omitempty"`
}

type geminiGoogleSearch struct {
	// Empty struct enables Google Search grounding
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
	Error      *geminiError          `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content       geminiContent `json:"content"`
	FinishReason  string        `json:"finishReason"`
	GroundingMetadata *geminiGroundingMetadata `json:"groundingMetadata,omitempty"`
}

type geminiGroundingMetadata struct {
	SearchEntryPoint *struct {
		RenderedContent string `json:"renderedContent"`
	} `json:"searchEntryPoint,omitempty"`
	GroundingChunks []struct {
		Web *struct {
			URI   string `json:"uri"`
			Title string `json:"title"`
		} `json:"web,omitempty"`
	} `json:"groundingChunks,omitempty"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Models returns available Gemini models
func (g *Gemini) Models(ctx context.Context) ([]string, error) {
	models := make([]string, 0, len(DefaultModels["gemini"]))
	for _, m := range DefaultModels["gemini"] {
		models = append(models, m.ID)
	}
	return models, nil
}

// Send sends a chat request and returns the complete response
func (g *Gemini) Send(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if g.apiKey == "" {
		return ChatResponse{}, ErrNoAPIKey
	}

	geminiReq := g.buildRequest(req)

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", g.baseURL, req.Model, g.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(httpReq)
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
		var errResp geminiResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return ChatResponse{}, fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return ChatResponse{}, fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	var geminiResp geminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(geminiResp.Candidates) == 0 {
		return ChatResponse{}, ErrInvalidResponse
	}

	// Extract text content and any thinking content
	var content strings.Builder
	var hasThinking bool

	for _, part := range geminiResp.Candidates[0].Content.Parts {
		if part.Thought != nil {
			if !hasThinking {
				content.WriteString("<thinking>\n")
				hasThinking = true
			}
			content.WriteString(part.Thought.Text)
			content.WriteString("\n")
		} else if part.Text != "" {
			if hasThinking {
				content.WriteString("</thinking>\n\n")
				hasThinking = false
			}
			content.WriteString(part.Text)
		}
	}
	if hasThinking {
		content.WriteString("</thinking>\n\n")
	}

	// Append grounding information if available
	if meta := geminiResp.Candidates[0].GroundingMetadata; meta != nil && len(meta.GroundingChunks) > 0 {
		content.WriteString("\n\n---\n**Sources:**\n")
		for _, chunk := range meta.GroundingChunks {
			if chunk.Web != nil {
				content.WriteString(fmt.Sprintf("- [%s](%s)\n", chunk.Web.Title, chunk.Web.URI))
			}
		}
	}

	usage := Usage{}
	if geminiResp.UsageMetadata != nil {
		usage.PromptTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.CompletionTokens = geminiResp.UsageMetadata.CandidatesTokenCount
		usage.TotalTokens = geminiResp.UsageMetadata.TotalTokenCount
	}

	return ChatResponse{
		Content:      content.String(),
		Model:        req.Model,
		FinishReason: geminiResp.Candidates[0].FinishReason,
		Usage:        usage,
	}, nil
}

// Stream sends a chat request and streams the response
func (g *Gemini) Stream(ctx context.Context, req ChatRequest, onDelta func(delta string)) error {
	if g.apiKey == "" {
		return ErrNoAPIKey
	}

	geminiReq := g.buildRequest(req)

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse", g.baseURL, req.Model, g.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.client.Do(httpReq)
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
		var errResp geminiResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != nil {
			return fmt.Errorf("API error: %s", errResp.Error.Message)
		}
		return fmt.Errorf("API error: status %d", resp.StatusCode)
	}

	// Parse SSE stream
	scanner := bufio.NewScanner(resp.Body)
	var inThinking bool
	var groundingChunks []struct {
		Title string
		URI   string
	}

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

		// Parse data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		var streamResp geminiResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue // Skip malformed chunks
		}

		if len(streamResp.Candidates) > 0 {
			candidate := streamResp.Candidates[0]

			// Collect grounding metadata
			if meta := candidate.GroundingMetadata; meta != nil {
				for _, chunk := range meta.GroundingChunks {
					if chunk.Web != nil {
						groundingChunks = append(groundingChunks, struct {
							Title string
							URI   string
						}{Title: chunk.Web.Title, URI: chunk.Web.URI})
					}
				}
			}

			for _, part := range candidate.Content.Parts {
				if part.Thought != nil {
					if !inThinking {
						onDelta("<thinking>\n")
						inThinking = true
					}
					onDelta(part.Thought.Text)
				} else if part.Text != "" {
					if inThinking {
						onDelta("\n</thinking>\n\n")
						inThinking = false
					}
					onDelta(part.Text)
				}
			}
		}
	}

	if inThinking {
		onDelta("\n</thinking>\n\n")
	}

	// Append sources at the end if we have grounding data
	if len(groundingChunks) > 0 {
		onDelta("\n\n---\n**Sources:**\n")
		for _, chunk := range groundingChunks {
			onDelta(fmt.Sprintf("- [%s](%s)\n", chunk.Title, chunk.URI))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}

	return nil
}

// buildRequest constructs a Gemini API request from a ChatRequest
func (g *Gemini) buildRequest(req ChatRequest) geminiRequest {
	var contents []geminiContent
	var systemInstruction *geminiContent

	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == RoleAssistant {
			role = "model"
		} else if msg.Role == RoleSystem {
			// System messages become systemInstruction
			systemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}

		contents = append(contents, geminiContent{
			Role: role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	geminiReq := geminiRequest{
		Contents:          contents,
		SystemInstruction: systemInstruction,
	}

	// Add generation config
	config := &geminiGenerationConfig{}
	if req.Temperature > 0 {
		config.Temperature = req.Temperature
	}
	if req.MaxTokens > 0 {
		config.MaxOutputTokens = req.MaxTokens
	}

	// Enable thinking mode for supported models
	if g.enableThought && isThinkingModel(req.Model) {
		if isGemini3Model(req.Model) {
			// Gemini 3 uses thinking_level parameter
			config.ThinkingConfig = &geminiThinkingConfig{
				ThinkingLevel: "medium", // Options: "low", "medium", "high"
			}
		} else {
			// Gemini 2.x uses thinkingBudget parameter
			config.ThinkingConfig = &geminiThinkingConfig{
				ThinkingBudget: 8192, // Default thinking budget
			}
		}
	}

	if config.Temperature > 0 || config.MaxOutputTokens > 0 || config.ThinkingConfig != nil {
		geminiReq.GenerationConfig = config
	}

	// Add Google Search tool if enabled
	if g.enableSearch {
		geminiReq.Tools = []geminiTool{
			{GoogleSearch: &geminiGoogleSearch{}},
		}
	}

	return geminiReq
}

// isThinkingModel returns true if the model supports thinking mode
func isThinkingModel(model string) bool {
	thinkingModels := []string{
		"gemini-3-pro",
		"gemini-3-flash",
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.0-flash-thinking",
	}
	for _, tm := range thinkingModels {
		if strings.Contains(model, tm) {
			return true
		}
	}
	return false
}

// isGemini3Model returns true if the model is a Gemini 3.x model
func isGemini3Model(model string) bool {
	return strings.Contains(model, "gemini-3")
}

// SetAPIKey updates the API key
func (g *Gemini) SetAPIKey(key string) {
	g.apiKey = key
}
