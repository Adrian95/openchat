package ui

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/store"
)

// Message types for attachments
type attachmentsLoadedMsg struct {
	attachments []*store.Attachment
	err         error
}

type attachmentAddedMsg struct {
	attachment *store.Attachment
	err        error
}

type filePreviewMsg struct {
	content  string
	filename string
	filepath string
	size     int64
	err      error
}

// updateAttachments handles key events in the attachments view
func (m *Model) updateAttachments(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentView = ViewChat
		m.textarea.Focus()
		return m, nil

	case "up", "k":
		if m.attachmentIndex > 0 {
			m.attachmentIndex--
		}
		return m, nil

	case "down", "j":
		if m.attachmentIndex < len(m.attachments)-1 {
			m.attachmentIndex++
		}
		return m, nil

	case "t": // Toggle inclusion in context
		if len(m.attachments) > 0 && m.attachmentIndex < len(m.attachments) {
			att := m.attachments[m.attachmentIndex]
			return m, m.toggleAttachment(att.ID)
		}
		return m, nil

	case "d": // Delete attachment
		if len(m.attachments) > 0 && m.attachmentIndex < len(m.attachments) {
			att := m.attachments[m.attachmentIndex]
			return m, m.deleteAttachment(att.ID)
		}
		return m, nil

	case "enter": // View content preview
		if len(m.attachments) > 0 && m.attachmentIndex < len(m.attachments) {
			att := m.attachments[m.attachmentIndex]
			m.attachmentPreview = att.Content
			if len(m.attachmentPreview) > 2000 {
				m.attachmentPreview = m.attachmentPreview[:2000] + "\n\n... (truncated)"
			}
		}
		return m, nil
	}

	return m, nil
}

// updateAttachConfirm handles key events in the attachment confirmation view
func (m *Model) updateAttachConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		// Confirm and add attachment
		if m.pendingAttachment != nil {
			return m, m.confirmAttachment()
		}
		m.currentView = ViewChat
		m.textarea.Focus()
		return m, nil

	case "n", "N", "esc":
		// Cancel
		m.pendingAttachment = nil
		m.attachmentPreview = ""
		m.currentView = ViewChat
		m.textarea.Focus()
		return m, nil
	}

	return m, nil
}

// toggleAttachment toggles an attachment's inclusion in context
func (m *Model) toggleAttachment(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.store.ToggleAttachmentContext(id)
		if err != nil {
			return errorMsg("Failed to toggle attachment: " + err.Error())
		}
		// Reload attachments
		atts, err := m.store.GetAttachments(m.currentSession.ID)
		return attachmentsLoadedMsg{attachments: atts, err: err}
	}
}

// deleteAttachment removes an attachment
func (m *Model) deleteAttachment(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.store.DeleteAttachment(id)
		if err != nil {
			return errorMsg("Failed to delete attachment: " + err.Error())
		}
		// Reload attachments
		atts, err := m.store.GetAttachments(m.currentSession.ID)
		return attachmentsLoadedMsg{attachments: atts, err: err}
	}
}

// confirmAttachment adds the pending attachment to the database
func (m *Model) confirmAttachment() tea.Cmd {
	att := m.pendingAttachment
	return func() tea.Msg {
		newAtt, err := m.store.AddAttachment(
			att.SessionID,
			att.Filename,
			att.Filepath,
			att.Content,
			att.MimeType,
			att.SizeBytes,
		)
		if err != nil {
			return attachmentAddedMsg{attachment: nil, err: err}
		}
		return attachmentAddedMsg{attachment: newAtt, err: nil}
	}
}

// loadAttachments loads attachments for the current session
func (m *Model) loadAttachments() tea.Cmd {
	if m.currentSession == nil {
		return nil
	}
	return func() tea.Msg {
		atts, err := m.store.GetAttachments(m.currentSession.ID)
		return attachmentsLoadedMsg{attachments: atts, err: err}
	}
}

// previewFile reads a file and prepares it for attachment
func (m *Model) previewFile(path string) tea.Cmd {
	return func() tea.Msg {
		// Resolve path
		absPath := path
		if !filepath.IsAbs(path) {
			cwd, _ := os.Getwd()
			absPath = filepath.Join(cwd, path)
		}

		// Check if file exists
		info, err := os.Stat(absPath)
		if err != nil {
			return filePreviewMsg{err: err}
		}

		if info.IsDir() {
			return filePreviewMsg{err: os.ErrInvalid}
		}

		// Check size limit (1MB default)
		if info.Size() > 1024*1024 {
			return filePreviewMsg{err: os.ErrInvalid}
		}

		// Read file content
		file, err := os.Open(absPath)
		if err != nil {
			return filePreviewMsg{err: err}
		}
		defer file.Close()

		content, err := io.ReadAll(file)
		if err != nil {
			return filePreviewMsg{err: err}
		}

		return filePreviewMsg{
			content:  string(content),
			filename: filepath.Base(absPath),
			filepath: absPath,
			size:     info.Size(),
			err:      nil,
		}
	}
}

// viewAttachments renders the attachments management view
func (m *Model) viewAttachments() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Context Vault - Attachments")
	b.WriteString(title)
	b.WriteString("\n")

	// Session info
	if m.currentSession != nil {
		sessionInfo := mutedTextStyle.Render("Session: " + m.currentSession.Name)
		b.WriteString(sessionInfo)
	}
	b.WriteString("\n\n")

	// Total size
	if m.currentSession != nil {
		totalSize, _ := m.store.GetAttachmentsTotalSize(m.currentSession.ID)
		sizeInfo := mutedTextStyle.Render("Total size: " + formatSize(totalSize) + " / 1MB limit")
		b.WriteString(sizeInfo)
		b.WriteString("\n\n")
	}

	// Attachments list
	if len(m.attachments) == 0 {
		b.WriteString(mutedTextStyle.Render("No attachments. Use /attach <path> to add files."))
		b.WriteString("\n")
	} else {
		for i, att := range m.attachments {
			var marker, status string

			if att.IncludedInContext {
				marker = "[x]"
				status = attachmentActiveStyle.Render("(in context)")
			} else {
				marker = "[ ]"
				status = attachmentInactiveStyle.Render("(excluded)")
			}

			line := att.Filename + " (" + formatSize(att.SizeBytes) + ") " + status

			var style = attachmentItemStyle
			if i == m.attachmentIndex {
				style = attachmentSelectedStyle
			}

			b.WriteString(style.Render(marker + " " + line))
			b.WriteString("\n")
		}
	}

	// Preview area
	if m.attachmentPreview != "" {
		b.WriteString("\n")
		b.WriteString(titleStyle.Render("Preview:"))
		b.WriteString("\n")
		b.WriteString(mutedTextStyle.Render(m.attachmentPreview))
		b.WriteString("\n")
	}

	// Help
	b.WriteString("\n")
	helpText := helpStyle.Render("↑/↓: Navigate | t: Toggle context | d: Delete | Enter: Preview | Esc: Close")
	b.WriteString(helpText)

	return modalStyle.Width(m.width - 4).Render(b.String())
}

// viewAttachConfirm renders the attachment confirmation view
func (m *Model) viewAttachConfirm() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Confirm Attachment")
	b.WriteString(title)
	b.WriteString("\n\n")

	if m.pendingAttachment != nil {
		b.WriteString("File: ")
		b.WriteString(infoStyle.Render(m.pendingAttachment.Filename))
		b.WriteString("\n")

		b.WriteString("Path: ")
		b.WriteString(mutedTextStyle.Render(m.pendingAttachment.Filepath))
		b.WriteString("\n")

		b.WriteString("Size: ")
		b.WriteString(mutedTextStyle.Render(formatSize(m.pendingAttachment.SizeBytes)))
		b.WriteString("\n\n")

		// Preview (first 500 chars)
		b.WriteString(titleStyle.Render("Content Preview:"))
		b.WriteString("\n")
		preview := m.pendingAttachment.Content
		if len(preview) > 500 {
			preview = preview[:500] + "\n... (truncated)"
		}
		b.WriteString(mutedTextStyle.Render(preview))
		b.WriteString("\n\n")

		b.WriteString(warningStyle.Render("This content will be sent to the AI when you send messages."))
		b.WriteString("\n\n")
	}

	// Confirmation prompt
	b.WriteString("Add this file to the context vault? ")
	b.WriteString(successStyle.Render("[Y]es"))
	b.WriteString(" / ")
	b.WriteString(errorStyle.Render("[N]o"))

	return modalStyle.Width(m.width - 4).Render(b.String())
}

// formatSize formats a file size in bytes to human readable form
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)

	switch {
	case bytes >= MB:
		return formatFloat(float64(bytes)/float64(MB)) + " MB"
	case bytes >= KB:
		return formatFloat(float64(bytes)/float64(KB)) + " KB"
	default:
		return formatInt(int(bytes)) + " B"
	}
}

// formatFloat formats a float with one decimal place
func formatFloat(f float64) string {
	whole := int(f)
	frac := int((f - float64(whole)) * 10)
	if frac == 0 {
		return formatInt(whole)
	}
	return formatInt(whole) + "." + formatInt(frac)
}
