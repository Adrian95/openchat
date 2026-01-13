// Package exporter handles exporting chat sessions to various formats.
// Currently supports Markdown export with optional git auto-commit.
package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/user/openchat/internal/sanitize"
	"github.com/user/openchat/internal/store"
)

// Exporter handles exporting sessions to files
type Exporter struct {
	exportPath    string
	gitAutoCommit bool
}

// New creates a new Exporter
func New(exportPath string, gitAutoCommit bool) *Exporter {
	return &Exporter{
		exportPath:    exportPath,
		gitAutoCommit: gitAutoCommit,
	}
}

// ExportSession exports a session and its messages to a Markdown file
func (e *Exporter) ExportSession(session *store.Session, messages []*store.Message) (string, error) {
	// Ensure export directory exists
	if err := os.MkdirAll(e.exportPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	// Generate filename
	filename := e.generateFilename(session)
	filepath := filepath.Join(e.exportPath, filename)

	// Generate markdown content
	content := e.generateMarkdown(session, messages)

	// Write file
	if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write export file: %w", err)
	}

	// Git auto-commit if enabled
	if e.gitAutoCommit {
		if err := GitAddAndCommit(e.exportPath, filename, session.Name); err != nil {
			// Log but don't fail the export
			// The error will be returned but the file was still created
			return filepath, fmt.Errorf("export succeeded but git commit failed: %w", err)
		}
	}

	return filepath, nil
}

// generateFilename creates a safe filename for the export
func (e *Exporter) generateFilename(session *store.Session) string {
	// Use session name and timestamp
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	safeName := sanitize.SanitizeFilename(session.Name)
	if len(safeName) > 50 {
		safeName = safeName[:50]
	}
	return fmt.Sprintf("%s_%s.md", safeName, timestamp)
}

// generateMarkdown creates the markdown content for a session
func (e *Exporter) generateMarkdown(session *store.Session, messages []*store.Message) string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# %s\n\n", session.Name))

	// Metadata
	sb.WriteString("## Metadata\n\n")
	sb.WriteString(fmt.Sprintf("- **Provider:** %s\n", session.Provider))
	sb.WriteString(fmt.Sprintf("- **Model:** %s\n", session.Model))
	sb.WriteString(fmt.Sprintf("- **Created:** %s\n", session.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Exported:** %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("- **Messages:** %d\n", len(messages)))
	sb.WriteString("\n---\n\n")

	// System prompt if present
	if session.SystemPrompt != "" {
		sb.WriteString("## System Prompt\n\n")
		sb.WriteString("```\n")
		sb.WriteString(session.SystemPrompt)
		sb.WriteString("\n```\n\n---\n\n")
	}

	// Conversation
	sb.WriteString("## Conversation\n\n")

	for _, msg := range messages {
		// Skip system messages in conversation (already shown above)
		if msg.Role == store.RoleSystem {
			continue
		}

		// Role header
		switch msg.Role {
		case store.RoleUser:
			sb.WriteString("### 👤 User\n\n")
		case store.RoleAssistant:
			sb.WriteString("### 🤖 Assistant\n\n")
		case store.RoleTool:
			sb.WriteString("### 🔧 Tool\n\n")
		default:
			sb.WriteString(fmt.Sprintf("### %s\n\n", msg.Role))
		}

		// Timestamp
		sb.WriteString(fmt.Sprintf("*%s*\n\n", msg.CreatedAt.Format("2006-01-02 15:04:05")))

		// Content (sanitized)
		content := sanitize.Sanitize(msg.Content)
		sb.WriteString(content)
		sb.WriteString("\n\n---\n\n")
	}

	return sb.String()
}

// ExportSessionAsText exports a session as plain text
func (e *Exporter) ExportSessionAsText(session *store.Session, messages []*store.Message) (string, error) {
	// Ensure export directory exists
	if err := os.MkdirAll(e.exportPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	// Generate filename
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	safeName := sanitize.SanitizeFilename(session.Name)
	if len(safeName) > 50 {
		safeName = safeName[:50]
	}
	filename := fmt.Sprintf("%s_%s.txt", safeName, timestamp)
	filepath := filepath.Join(e.exportPath, filename)

	// Generate content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session: %s\n", session.Name))
	sb.WriteString(fmt.Sprintf("Provider: %s | Model: %s\n", session.Provider, session.Model))
	sb.WriteString(fmt.Sprintf("Created: %s\n", session.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	for _, msg := range messages {
		if msg.Role == store.RoleSystem {
			continue
		}

		var role string
		switch msg.Role {
		case store.RoleUser:
			role = "USER"
		case store.RoleAssistant:
			role = "ASSISTANT"
		case store.RoleTool:
			role = "TOOL"
		default:
			role = string(msg.Role)
		}

		sb.WriteString(fmt.Sprintf("[%s] %s\n", role, msg.CreatedAt.Format("15:04:05")))
		sb.WriteString(sanitize.Sanitize(msg.Content))
		sb.WriteString("\n\n")
	}

	// Write file
	if err := os.WriteFile(filepath, []byte(sb.String()), 0644); err != nil {
		return "", fmt.Errorf("failed to write export file: %w", err)
	}

	// Git auto-commit if enabled
	if e.gitAutoCommit {
		if err := GitAddAndCommit(e.exportPath, filename, session.Name); err != nil {
			return filepath, fmt.Errorf("export succeeded but git commit failed: %w", err)
		}
	}

	return filepath, nil
}

// ListExports returns a list of exported files in the export directory
func (e *Exporter) ListExports() ([]string, error) {
	entries, err := os.ReadDir(e.exportPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read export directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".md") || strings.HasSuffix(entry.Name(), ".txt")) {
			files = append(files, entry.Name())
		}
	}

	return files, nil
}

// GetExportPath returns the export directory path
func (e *Exporter) GetExportPath() string {
	return e.exportPath
}
