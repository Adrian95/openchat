package sanitize

import (
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no escape sequences",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "color codes",
			input:    "\x1b[31mRed Text\x1b[0m",
			expected: "Red Text",
		},
		{
			name:     "multiple colors",
			input:    "\x1b[1;32mBold Green\x1b[0m and \x1b[34mBlue\x1b[0m",
			expected: "Bold Green and Blue",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2J\x1b[H",
			expected: "",
		},
		{
			name:     "OSC sequence (terminal title)",
			input:    "\x1b]0;New Title\x07Normal text",
			expected: "Normal text",
		},
		{
			name:     "complex escape",
			input:    "\x1b[38;5;196mExtended color\x1b[0m",
			expected: "Extended color",
		},
		{
			name:     "simple escape",
			input:    "\x1bM",
			expected: "",
		},
		{
			name:     "mixed content",
			input:    "Start \x1b[1mbold\x1b[0m middle \x1b[4munderline\x1b[0m end",
			expected: "Start bold middle underline end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("StripANSI() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestStripControlChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no control chars",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "preserves newline",
			input:    "Line 1\nLine 2",
			expected: "Line 1\nLine 2",
		},
		{
			name:     "preserves tab",
			input:    "Col1\tCol2",
			expected: "Col1\tCol2",
		},
		{
			name:     "removes null byte",
			input:    "Hello\x00World",
			expected: "HelloWorld",
		},
		{
			name:     "removes bell",
			input:    "Hello\x07World",
			expected: "HelloWorld",
		},
		{
			name:     "removes backspace",
			input:    "Hello\x08World",
			expected: "HelloWorld",
		},
		{
			name:     "removes DEL character",
			input:    "Hello\x7fWorld",
			expected: "HelloWorld",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := StripControlChars(tt.input)
			if result != tt.expected {
				t.Errorf("StripControlChars() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean text",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "mixed dangerous content",
			input:    "\x1b[31mRed\x1b[0m with \x00null",
			expected: "Red with null",
		},
		{
			name:     "excessive newlines",
			input:    "Line 1\n\n\n\n\nLine 2",
			expected: "Line 1\n\nLine 2",
		},
		{
			name:     "preserves code formatting",
			input:    "func main() {\n\tfmt.Println(\"hello\")\n}",
			expected: "func main() {\n\tfmt.Println(\"hello\")\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("Sanitize() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestSanitizeTruncatesLargeOutput(t *testing.T) {
	// Create a string larger than MaxOutputSize
	largeInput := strings.Repeat("a", MaxOutputSize+1000)

	result := Sanitize(largeInput)

	if len(result) > MaxOutputSize+50 { // Allow for truncation message
		t.Errorf("Sanitize() did not truncate large output, got len=%d", len(result))
	}

	if !strings.HasSuffix(result, "... [output truncated]") {
		t.Error("Sanitize() did not add truncation message")
	}
}

func TestSanitizeForDisplay(t *testing.T) {
	// Test long line truncation
	longLine := strings.Repeat("x", MaxLineLength+100)
	result := SanitizeForDisplay(longLine)

	if len(result) > MaxLineLength+10 { // Allow for ellipsis
		t.Errorf("SanitizeForDisplay() did not truncate long line, got len=%d", len(result))
	}

	if !strings.HasSuffix(result, "...") {
		t.Error("SanitizeForDisplay() did not add ellipsis to truncated line")
	}
}

func TestIsSafeString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"Hello, World!", true},
		{"Line 1\nLine 2", true},
		{"\x1b[31mRed\x1b[0m", false},
		{"Hello\x00World", false},
		{"Normal text with numbers 123", true},
	}

	for _, tt := range tests {
		result := IsSafeString(tt.input)
		if result != tt.expected {
			t.Errorf("IsSafeString(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello-world", "hello-world"},
		{"Hello World", "Hello World"},
		{"file/with/slashes", "file_with_slashes"},
		{"file:with:colons", "file_with_colons"},
		{".hidden", "_hidden"},
		{"", "unnamed"},
		{"name<>with|special*chars?", "name__with_special_chars_"},
		{strings.Repeat("a", 300), strings.Repeat("a", 200)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTruncateWithEllipsis(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10!", 10, "exactly10!"},
		{"this is a long string", 10, "this is..."},
		{"ab", 5, "ab"},
		{"abcdef", 3, "abc"},
	}

	for _, tt := range tests {
		result := TruncateWithEllipsis(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("TruncateWithEllipsis(%q, %d) = %q, want %q",
				tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestNormalizeWhitespace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  hello   world  ", "hello world"},
		{"no\textra\tspaces", "no extra spaces"},
		{"single", "single"},
		{"   ", ""},
	}

	for _, tt := range tests {
		result := NormalizeWhitespace(tt.input)
		if result != tt.expected {
			t.Errorf("NormalizeWhitespace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRemoveNullBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello\x00world", "helloworld"},
		{"no nulls", "no nulls"},
		{"\x00\x00\x00", ""},
		{"start\x00middle\x00end", "startmiddleend"},
	}

	for _, tt := range tests {
		result := RemoveNullBytes(tt.input)
		if result != tt.expected {
			t.Errorf("RemoveNullBytes(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// Benchmark tests
func BenchmarkStripANSI(b *testing.B) {
	input := "\x1b[31mHello\x1b[0m \x1b[1;32mWorld\x1b[0m with \x1b[4mformatting\x1b[0m"
	for i := 0; i < b.N; i++ {
		StripANSI(input)
	}
}

func BenchmarkSanitize(b *testing.B) {
	input := "\x1b[31mHello\x1b[0m\x00World\n\n\n\nText"
	for i := 0; i < b.N; i++ {
		Sanitize(input)
	}
}
