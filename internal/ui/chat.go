package ui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/provider"
	"github.com/user/openchat/internal/store"
)

// updateChat handles updates in the chat view
func (m *Model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+enter", "ctrl+j":
		// Send message
		return m.sendMessage()

	case "up", "k":
		if !m.textarea.Focused() {
			m.viewport.LineUp(1)
		}

	case "down", "j":
		if !m.textarea.Focused() {
			m.viewport.LineDown(1)
		}

	case "pgup":
		m.viewport.HalfViewUp()

	case "pgdown":
		m.viewport.HalfViewDown()

	case "home":
		m.viewport.GotoTop()

	case "end":
		m.viewport.GotoBottom()

	default:
		// Update textarea
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	return m, nil
}

// sendMessage sends the current input as a message
func (m *Model) sendMessage() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}

	// Clear the input
	m.textarea.Reset()
	m.errorMessage = ""
	m.statusMessage = ""

	// Handle slash commands
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	// Regular message - send to AI
	return m.sendToAI(input)
}

// sendToAI sends a message to the AI provider
func (m *Model) sendToAI(content string) (tea.Model, tea.Cmd) {
	// Ensure we have a session
	if m.currentSession == nil {
		// Auto-create a session
		return m, func() tea.Msg {
			session, err := m.store.CreateSession(
				"New Chat",
				m.config.GetDefaultProvider(),
				m.config.GetDefaultModel(),
				"",
			)
			if err != nil {
				return errorMsg("Failed to create session: " + err.Error())
			}
			return sessionCreatedMsg{session: session}
		}
	}

	// Check provider
	if m.currentProvider == nil {
		m.errorMessage = "No provider configured. Use /connect to set API key."
		return m, nil
	}

	// Check API key
	if !m.config.HasAPIKey(m.currentProvider.Name()) {
		m.errorMessage = "No API key for " + m.currentProvider.Name() + ". Use /connect."
		return m, nil
	}

	// Save user message
	userMsg, err := m.store.AddMessage(m.currentSession.ID, store.RoleUser, content)
	if err != nil {
		m.errorMessage = "Failed to save message: " + err.Error()
		return m, nil
	}
	m.messages = append(m.messages, userMsg)
	m.updateViewportContent()

	// Build request
	messages := make([]provider.Message, 0, len(m.messages)+10)

	// Add system prompt if exists
	if m.currentSession.SystemPrompt != "" {
		messages = append(messages, provider.Message{
			Role:    provider.RoleSystem,
			Content: m.currentSession.SystemPrompt,
		})
	}

	// Add attached file contents as system context
	if m.currentSession != nil {
		attachments, _ := m.store.GetActiveAttachments(m.currentSession.ID)
		if len(attachments) > 0 {
			var contextBuilder strings.Builder
			contextBuilder.WriteString("The following files are attached as context:\n\n")
			for _, att := range attachments {
				contextBuilder.WriteString("--- File: ")
				contextBuilder.WriteString(att.Filename)
				contextBuilder.WriteString(" ---\n")
				contextBuilder.WriteString(att.Content)
				contextBuilder.WriteString("\n--- End of ")
				contextBuilder.WriteString(att.Filename)
				contextBuilder.WriteString(" ---\n\n")
			}
			messages = append(messages, provider.Message{
				Role:    provider.RoleSystem,
				Content: contextBuilder.String(),
			})
		}
	}

	// Add conversation history
	for _, msg := range m.messages {
		// Map summary role to system for the provider
		role := provider.Role(msg.Role)
		if msg.Role == store.RoleSummary {
			role = provider.RoleSystem
		}
		messages = append(messages, provider.Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	req := provider.ChatRequest{
		Model:    m.config.GetDefaultModel(),
		Messages: messages,
	}

	// Start streaming
	m.streaming = true
	m.streamContent.Reset()

	return m, m.streamResponse(req)
}

// streamResponse streams a response from the AI provider
func (m *Model) streamResponse(req provider.ChatRequest) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.streamCancel = cancel

		// Get fresh provider reference with API key
		prov, ok := m.registry.Get(m.config.GetDefaultProvider())
		if !ok {
			return streamCompleteMsg{err: provider.ErrNoAPIKey}
		}

		// Set API key on provider
		apiKey := m.config.GetAPIKey(prov.Name())
		switch p := prov.(type) {
		case *provider.OpenAI:
			p.SetAPIKey(apiKey)
		case *provider.Anthropic:
			p.SetAPIKey(apiKey)
		case *provider.Gemini:
			p.SetAPIKey(apiKey)
			// Apply Gemini-specific settings from model state
			p.SetThinkingEnabled(m.geminiThinking)
			p.SetSearchEnabled(m.geminiGrounding)
		}

		if prov.SupportsStreaming() {
			err := prov.Stream(ctx, req, func(delta string) {
				// Send delta through program
				// This is a workaround since we can't directly update model from here
				// We accumulate in streamContent and signal completion
			})

			// For now, use non-streaming as the stream callback can't easily
			// send messages back to the Update loop without a channel
			// TODO: Implement proper streaming with channels
			if err != nil {
				return streamCompleteMsg{err: err}
			}
		}

		// Fallback to non-streaming
		resp, err := prov.Send(ctx, req)
		if err != nil {
			return streamCompleteMsg{err: err}
		}

		return streamContentMsg(resp.Content)
	}
}

// Message types for async operations
type streamDeltaMsg string
type streamContentMsg string
type streamCompleteMsg struct {
	err error
}
type errorMsg string
type sessionCreatedMsg struct {
	session *store.Session
}
type sessionLoadedMsg struct {
	session  *store.Session
	messages []*store.Message
}
type sessionsLoadedMsg struct {
	sessions []*store.Session
}
type exportCompleteMsg struct {
	path string
	err  error
}

// Update the model to handle streamContentMsg
func (m *Model) handleStreamContent(content string) tea.Cmd {
	return func() tea.Msg {
		m.streamContent.WriteString(content)
		return streamCompleteMsg{err: nil}
	}
}
