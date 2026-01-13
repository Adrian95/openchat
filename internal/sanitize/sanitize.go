// Package sanitize provides utilities for sanitizing text content,
// particularly for removing potentially dangerous ANSI escape sequences
// from model output before rendering in the terminal.
package sanitize

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxOutputSize is the maximum allowed output size in bytes (1MB)
	MaxOutputSize = 1024 * 1024
	// MaxLineLength is the maximum allowed line length
	MaxLineLength = 10000
)

var (
	// ansiEscapePattern matches ANSI escape sequences
	// This includes:
	// - CSI sequences: ESC [ ... final byte
	// - OSC sequences: ESC ] ... ST or BEL
	// - Simple escapes: ESC followed by single character
	ansiEscapePattern = regexp.MustCompile(`\x1b(?:\[[0-9;]*[a-zA-Z]|\][^\x07\x1b]*(?:\x07|\x1b\\)?|[a-zA-Z])`)

	// controlCharPattern matches control characters except common whitespace
	controlCharPattern = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)

	// multipleNewlinesPattern matches 3+ consecutive newlines
	multipleNewlinesPattern = regexp.MustCompile(`\n{3,}`)
)

// StripANSI removes ANSI escape sequences from the input string
// This is critical for security to prevent terminal injection attacks
func StripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

// StripControlChars removes control characters except standard whitespace (newline, tab, carriage return)
func StripControlChars(s string) string {
	return controlCharPattern.ReplaceAllString(s, "")
}

// Sanitize performs full sanitization of model output:
// 1. Strips ANSI escape sequences
// 2. Removes dangerous control characters
// 3. Limits output size
// 4. Normalizes excessive newlines
func Sanitize(s string) string {
	// First strip ANSI escapes
	s = StripANSI(s)

	// Remove control characters
	s = StripControlChars(s)

	// Limit total size
	if len(s) > MaxOutputSize {
		s = s[:MaxOutputSize] + "\n... [output truncated]"
	}

	// Normalize excessive newlines (more than 2 consecutive)
	s = multipleNewlinesPattern.ReplaceAllString(s, "\n\n")

	return s
}

// SanitizeForDisplay prepares text for terminal display
// Applies all sanitization and additionally handles line length
func SanitizeForDisplay(s string) string {
	s = Sanitize(s)

	// Split into lines and truncate overly long lines
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if len(line) > MaxLineLength {
			lines[i] = line[:MaxLineLength] + "..."
		}
	}

	return strings.Join(lines, "\n")
}

// IsSafeString checks if a string contains any potentially dangerous sequences
func IsSafeString(s string) bool {
	// Check for ANSI escapes
	if ansiEscapePattern.MatchString(s) {
		return false
	}

	// Check for control characters
	if controlCharPattern.MatchString(s) {
		return false
	}

	return true
}

// SanitizeFilename sanitizes a string for use as a filename
func SanitizeFilename(s string) string {
	// Remove or replace invalid characters
	var result strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			result.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			result.WriteRune(r)
		case r == '.':
			result.WriteRune(r)
		default:
			result.WriteRune('_')
		}
	}

	filename := result.String()

	// Limit length
	if len(filename) > 200 {
		filename = filename[:200]
	}

	// Ensure it doesn't start with a dot (hidden file)
	if strings.HasPrefix(filename, ".") {
		filename = "_" + filename[1:]
	}

	// Ensure it's not empty
	if filename == "" {
		filename = "unnamed"
	}

	return filename
}

// TruncateWithEllipsis truncates a string to maxLen and adds ellipsis if needed
func TruncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// NormalizeWhitespace collapses multiple spaces/tabs into single spaces
// and trims leading/trailing whitespace
func NormalizeWhitespace(s string) string {
	// Split on whitespace and rejoin with single spaces
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// RemoveNullBytes removes null bytes from a string
func RemoveNullBytes(s string) string {
	return strings.ReplaceAll(s, "\x00", "")
}
