// Package tokens provides token estimation for different AI providers.
package tokens

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Estimator provides token estimation for a specific provider
type Estimator struct {
	provider string
}

// NewEstimator creates a new token estimator for the given provider
func NewEstimator(provider string) *Estimator {
	return &Estimator{provider: provider}
}

// EstimateTokens estimates the number of tokens in the given text.
// Uses provider-specific heuristics for more accurate estimates.
func (e *Estimator) EstimateTokens(text string) int {
	if text == "" {
		return 0
	}

	switch e.provider {
	case "openai":
		return e.estimateOpenAI(text)
	case "anthropic":
		return e.estimateAnthropic(text)
	default:
		// Fallback to a general estimation
		return e.estimateGeneral(text)
	}
}

// estimateOpenAI estimates tokens for OpenAI models (GPT-4, GPT-3.5).
// OpenAI uses cl100k_base tokenizer for GPT-4/3.5.
// Average: ~4 characters per token for English, less for code/non-English.
func (e *Estimator) estimateOpenAI(text string) int {
	// Count different character types for more accurate estimation
	var (
		asciiChars    int
		nonAsciiChars int
		whitespace    int
		punctuation   int
		digits        int
	)

	for _, r := range text {
		switch {
		case r <= 127:
			if unicode.IsSpace(r) {
				whitespace++
			} else if unicode.IsPunct(r) {
				punctuation++
			} else if unicode.IsDigit(r) {
				digits++
			}
			asciiChars++
		default:
			nonAsciiChars++
		}
	}

	// Heuristic based on character composition
	// - ASCII text: ~4 chars per token
	// - Non-ASCII (CJK, etc.): ~1.5 chars per token
	// - Code/punctuation heavy: ~3 chars per token
	// - Numbers: ~2 chars per token

	totalChars := len(text)
	if totalChars == 0 {
		return 0
	}

	// Base estimate from ASCII
	estimate := float64(asciiChars) / 4.0

	// Add non-ASCII (they typically take more tokens)
	estimate += float64(nonAsciiChars) / 1.5

	// Adjust for code/punctuation (more tokens per character)
	punctRatio := float64(punctuation) / float64(totalChars)
	if punctRatio > 0.1 {
		estimate *= 1.2
	}

	// Add some overhead for tokenization boundaries
	words := len(strings.Fields(text))
	estimate += float64(words) * 0.1

	if estimate < 1 && totalChars > 0 {
		estimate = 1
	}

	return int(estimate)
}

// estimateAnthropic estimates tokens for Anthropic models (Claude).
// Claude uses a similar BPE tokenizer to OpenAI but with some differences.
func (e *Estimator) estimateAnthropic(text string) int {
	// Claude's tokenizer is similar to OpenAI's but handles some edge cases differently
	// Generally ~4 characters per token for English

	runeCount := utf8.RuneCountInString(text)
	if runeCount == 0 {
		return 0
	}

	// Count words and special characters
	words := len(strings.Fields(text))

	// Estimate based on characters and words
	// Average English word is ~4-5 chars, becomes ~1.3 tokens
	charEstimate := float64(len(text)) / 4.0
	wordEstimate := float64(words) * 1.3

	// Use the average of both estimates
	estimate := (charEstimate + wordEstimate) / 2.0

	// Adjust for non-ASCII characters
	for _, r := range text {
		if r > 127 {
			estimate += 0.5 // Non-ASCII chars typically need more tokens
		}
	}

	if estimate < 1 && runeCount > 0 {
		estimate = 1
	}

	return int(estimate)
}

// estimateGeneral provides a general token estimation that works across providers.
func (e *Estimator) estimateGeneral(text string) int {
	// Simple heuristic: ~4 characters per token
	chars := len(text)
	if chars == 0 {
		return 0
	}

	estimate := chars / 4
	if estimate < 1 {
		estimate = 1
	}

	return estimate
}

// EstimateMessages estimates total tokens for a list of messages.
func (e *Estimator) EstimateMessages(messages []Message) int {
	total := 0
	for _, msg := range messages {
		// Add overhead for message structure (role, separators, etc.)
		total += 4 // approximate overhead per message
		total += e.EstimateTokens(msg.Content)
	}
	return total
}

// Message represents a message for token estimation
type Message struct {
	Role    string
	Content string
}

// ContextInfo contains information about context usage
type ContextInfo struct {
	UsedTokens      int
	MaxTokens       int
	UsagePercent    float64
	RemainingTokens int
	WarningLevel    WarningLevel
}

// WarningLevel indicates how close to the context limit we are
type WarningLevel int

const (
	WarningNone     WarningLevel = iota // < 50%
	WarningLow                          // 50-75%
	WarningMedium                       // 75-90%
	WarningHigh                         // 90-95%
	WarningCritical                     // > 95%
)

// GetContextInfo calculates context usage information
func (e *Estimator) GetContextInfo(usedTokens, maxTokens int) ContextInfo {
	if maxTokens <= 0 {
		maxTokens = 128000 // Default to GPT-4's context
	}

	remaining := maxTokens - usedTokens
	if remaining < 0 {
		remaining = 0
	}

	percent := float64(usedTokens) / float64(maxTokens) * 100

	var level WarningLevel
	switch {
	case percent >= 95:
		level = WarningCritical
	case percent >= 90:
		level = WarningHigh
	case percent >= 75:
		level = WarningMedium
	case percent >= 50:
		level = WarningLow
	default:
		level = WarningNone
	}

	return ContextInfo{
		UsedTokens:      usedTokens,
		MaxTokens:       maxTokens,
		UsagePercent:    percent,
		RemainingTokens: remaining,
		WarningLevel:    level,
	}
}

// GetMaxTokensForModel returns the maximum context tokens for a given model
func GetMaxTokensForModel(provider, model string) int {
	switch provider {
	case "openai":
		switch {
		case strings.Contains(model, "gpt-4o"), strings.Contains(model, "gpt-4-turbo"):
			return 128000
		case strings.Contains(model, "gpt-4"):
			return 8192
		case strings.Contains(model, "gpt-3.5-turbo-16k"):
			return 16385
		case strings.Contains(model, "gpt-3.5"):
			return 4096
		default:
			return 8192
		}
	case "anthropic":
		// Claude models generally have 200k context
		return 200000
	default:
		return 8192
	}
}

// FormatTokenCount formats a token count for display
func FormatTokenCount(tokens int) string {
	if tokens >= 1000000 {
		return strings.TrimRight(strings.TrimRight(
			strings.Replace(string(rune(tokens/100000)+'0')+"."+string(rune((tokens%100000)/10000)+'0')+"M", ".0M", "M", 1),
			"0"), ".")
	}
	if tokens >= 1000 {
		k := float64(tokens) / 1000.0
		if k == float64(int(k)) {
			return string(rune(int(k)+'0')) + "k"
		}
		// Format as X.Yk
		whole := int(k)
		frac := int((k - float64(whole)) * 10)
		if frac == 0 {
			return string(rune(whole+'0')) + "k"
		}
		return string(rune(whole+'0')) + "." + string(rune(frac+'0')) + "k"
	}
	return formatInt(tokens)
}

// formatInt converts an int to string without importing strconv
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + formatInt(-n)
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte(n%10) + '0'}, digits...)
		n /= 10
	}
	return string(digits)
}
