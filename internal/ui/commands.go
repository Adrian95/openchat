package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/provider"
	"github.com/user/openchat/internal/store"
)

// handleCommand processes slash commands
func (m *Model) handleCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return m, nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "/new":
		return m.cmdNew(args)
	case "/switch":
		return m.cmdSwitch()
	case "/connect":
		return m.cmdConnect(args)
	case "/model":
		return m.cmdModel(args)
	case "/export":
		return m.cmdExport()
	case "/help":
		return m.cmdHelp()
	case "/clear":
		return m.cmdClear()
	case "/delete":
		return m.cmdDelete()
	case "/rename":
		return m.cmdRename(args)
	case "/system":
		return m.cmdSystem(args)
	case "/search":
		return m.cmdSearch(args)
	case "/attach":
		return m.cmdAttach(args)
	case "/attachments", "/vault":
		return m.cmdAttachments()
	case "/summarize":
		return m.cmdSummarize(args)
	case "/context":
		return m.cmdContext()
	case "/thinking":
		return m.cmdThinking()
	case "/grounding", "/search-grounding":
		return m.cmdGrounding()
	default:
		m.errorMessage = "Unknown command: " + cmd + ". Type /help for available commands."
		return m, nil
	}
}

// cmdNew creates a new session
func (m *Model) cmdNew(args []string) (tea.Model, tea.Cmd) {
	name := "New Chat"
	if len(args) > 0 {
		name = strings.Join(args, " ")
	}

	return m, func() tea.Msg {
		session, err := m.store.CreateSession(
			name,
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

// cmdSwitch opens the session switcher
func (m *Model) cmdSwitch() (tea.Model, tea.Cmd) {
	m.currentView = ViewSessions
	m.sessionIndex = 0
	return m, m.loadSessions()
}

// cmdConnect opens the API key configuration
func (m *Model) cmdConnect(args []string) (tea.Model, tea.Cmd) {
	m.currentView = ViewConnect
	m.connectError = ""
	m.connectKey = ""
	m.connectPersist = false

	// Default to current provider or openai
	if len(args) > 0 {
		m.connectProvider = strings.ToLower(args[0])
	} else {
		m.connectProvider = m.config.GetDefaultProvider()
	}

	return m, nil
}

// cmdModel opens the model selector
func (m *Model) cmdModel(args []string) (tea.Model, tea.Cmd) {
	// If args provided, try to set directly
	if len(args) >= 1 {
		// Check if it's provider/model format
		if strings.Contains(args[0], "/") {
			parts := strings.SplitN(args[0], "/", 2)
			providerName := parts[0]
			modelName := parts[1]

			if _, ok := m.registry.Get(providerName); ok {
				m.config.SetDefaultProvider(providerName)
				m.config.SetDefaultModel(modelName)
				m.initProvider()
				m.statusMessage = "Set provider: " + providerName + ", model: " + modelName

				// Update current session if exists
				if m.currentSession != nil {
					m.currentSession.Provider = providerName
					m.currentSession.Model = modelName
					m.store.UpdateSession(m.currentSession)
				}
			} else {
				m.errorMessage = "Unknown provider: " + providerName
			}
		} else {
			// Just model name - use current provider
			m.config.SetDefaultModel(args[0])
			m.statusMessage = "Set model: " + args[0]

			if m.currentSession != nil {
				m.currentSession.Model = args[0]
				m.store.UpdateSession(m.currentSession)
			}
		}
		return m, nil
	}

	// Open model selection view
	m.currentView = ViewModels
	m.modelIndex = 0
	return m, m.loadModels()
}

// cmdExport exports the current session
func (m *Model) cmdExport() (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session to export"
		return m, nil
	}

	return m, func() tea.Msg {
		path, err := m.exporter.ExportSession(m.currentSession, m.messages)
		return exportCompleteMsg{path: path, err: err}
	}
}

// cmdHelp shows the help screen
func (m *Model) cmdHelp() (tea.Model, tea.Cmd) {
	m.currentView = ViewHelp
	return m, nil
}

// cmdClear clears the current session messages
func (m *Model) cmdClear() (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session to clear"
		return m, nil
	}

	// Clear messages from database
	for _, msg := range m.messages {
		m.store.DeleteMessage(msg.ID)
	}

	m.messages = make([]*store.Message, 0)
	m.updateViewportContent()
	m.statusMessage = "Session cleared"
	return m, nil
}

// cmdDelete deletes the current session
func (m *Model) cmdDelete() (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session to delete"
		return m, nil
	}

	sessionID := m.currentSession.ID
	m.currentSession = nil
	m.messages = make([]*store.Message, 0)

	return m, func() tea.Msg {
		err := m.store.DeleteSession(sessionID)
		if err != nil {
			return errorMsg("Failed to delete session: " + err.Error())
		}
		return sessionsLoadedMsg{sessions: nil} // Will trigger reload
	}
}

// cmdRename renames the current session
func (m *Model) cmdRename(args []string) (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session to rename"
		return m, nil
	}

	if len(args) == 0 {
		m.errorMessage = "Usage: /rename <new name>"
		return m, nil
	}

	newName := strings.Join(args, " ")
	m.currentSession.Name = newName

	return m, func() tea.Msg {
		err := m.store.UpdateSession(m.currentSession)
		if err != nil {
			return errorMsg("Failed to rename session: " + err.Error())
		}
		return sessionsLoadedMsg{sessions: nil}
	}
}

// cmdSystem sets the system prompt for the current session
func (m *Model) cmdSystem(args []string) (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session selected"
		return m, nil
	}

	if len(args) == 0 {
		// Show current system prompt
		if m.currentSession.SystemPrompt != "" {
			m.statusMessage = "System prompt: " + m.currentSession.SystemPrompt
		} else {
			m.statusMessage = "No system prompt set"
		}
		return m, nil
	}

	prompt := strings.Join(args, " ")
	m.currentSession.SystemPrompt = prompt

	return m, func() tea.Msg {
		err := m.store.UpdateSession(m.currentSession)
		if err != nil {
			return errorMsg("Failed to set system prompt: " + err.Error())
		}
		return errorMsg("") // Success, clear any error
	}
}

// loadSessions loads all sessions from the database
func (m *Model) loadSessions() tea.Cmd {
	return func() tea.Msg {
		sessions, err := m.store.ListSessions()
		if err != nil {
			return errorMsg("Failed to load sessions: " + err.Error())
		}
		return sessionsLoadedMsg{sessions: sessions}
	}
}

// loadMostRecentSession loads the most recent session
func (m *Model) loadMostRecentSession() tea.Cmd {
	return func() tea.Msg {
		session, err := m.store.GetMostRecentSession()
		if err != nil {
			return errorMsg("Failed to load session: " + err.Error())
		}
		if session == nil {
			return sessionLoadedMsg{session: nil, messages: nil}
		}

		messages, err := m.store.GetMessages(session.ID)
		if err != nil {
			return errorMsg("Failed to load messages: " + err.Error())
		}

		return sessionLoadedMsg{session: session, messages: messages}
	}
}

// loadSession loads a specific session
func (m *Model) loadSession(sessionID string) tea.Cmd {
	return func() tea.Msg {
		session, err := m.store.GetSession(sessionID)
		if err != nil {
			return errorMsg("Failed to load session: " + err.Error())
		}
		if session == nil {
			return errorMsg("Session not found")
		}

		messages, err := m.store.GetMessages(session.ID)
		if err != nil {
			return errorMsg("Failed to load messages: " + err.Error())
		}

		return sessionLoadedMsg{session: session, messages: messages}
	}
}

// loadModels loads available models for the current provider
func (m *Model) loadModels() tea.Cmd {
	return func() tea.Msg {
		if m.currentProvider == nil {
			return errorMsg("No provider selected")
		}
		// Use static model list
		models, _ := m.currentProvider.Models(nil)
		m.availableModels = models
		return nil
	}
}

// cmdSearch opens the search view
func (m *Model) cmdSearch(args []string) (tea.Model, tea.Cmd) {
	m.currentView = ViewSearch
	m.searchQuery = ""
	m.searchResults = nil
	m.searchIndex = 0
	m.selectedSnippets = make(map[string]bool)

	// If args provided, use as initial search query
	if len(args) > 0 {
		m.searchQuery = strings.Join(args, " ")
		return m, m.performSearch()
	}

	return m, nil
}

// cmdAttach attaches a file to the current session's context vault
func (m *Model) cmdAttach(args []string) (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session selected. Create a session first."
		return m, nil
	}

	if len(args) == 0 {
		m.errorMessage = "Usage: /attach <path>"
		return m, nil
	}

	path := strings.Join(args, " ")
	return m, m.previewFile(path)
}

// cmdAttachments opens the attachments management view
func (m *Model) cmdAttachments() (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session selected"
		return m, nil
	}

	m.currentView = ViewAttachments
	m.attachmentIndex = 0
	m.attachmentPreview = ""
	return m, m.loadAttachments()
}

// cmdSummarize summarizes older messages to reduce context size
func (m *Model) cmdSummarize(args []string) (tea.Model, tea.Cmd) {
	if m.currentSession == nil {
		m.errorMessage = "No session selected"
		return m, nil
	}

	if len(m.messages) < 4 {
		m.errorMessage = "Not enough messages to summarize (need at least 4)"
		return m, nil
	}

	// Determine how many messages to summarize
	// Default: keep last 4 messages, summarize the rest
	keepCount := 4
	if len(args) > 0 {
		// Parse the keep count
		n := 0
		for _, c := range args[0] {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		if n > 0 && n < len(m.messages) {
			keepCount = n
		}
	}

	if len(m.messages) <= keepCount {
		m.errorMessage = "Not enough messages to summarize"
		return m, nil
	}

	// Get messages to summarize (all except the last keepCount)
	toSummarize := m.messages[:len(m.messages)-keepCount]
	if len(toSummarize) == 0 {
		m.errorMessage = "No messages to summarize"
		return m, nil
	}

	m.statusMessage = "Generating summary..."

	return m, m.generateSummary(toSummarize, keepCount)
}

// generateSummary creates a summary of the given messages using the AI
func (m *Model) generateSummary(messages []*store.Message, keepCount int) tea.Cmd {
	return func() tea.Msg {
		// Build a prompt asking for a summary
		var contentBuilder strings.Builder
		contentBuilder.WriteString("Please provide a concise summary of the following conversation. ")
		contentBuilder.WriteString("Capture the key points, decisions, and any important context that should be preserved:\n\n")

		for _, msg := range messages {
			contentBuilder.WriteString(string(msg.Role))
			contentBuilder.WriteString(": ")
			contentBuilder.WriteString(msg.Content)
			contentBuilder.WriteString("\n\n")
		}

		// Create summary request message
		return summarizeRequestMsg{
			messages:         messages,
			summaryPrompt:    contentBuilder.String(),
			keepRecentCount:  keepCount,
			startMessageID:   messages[0].ID,
			endMessageID:     messages[len(messages)-1].ID,
		}
	}
}

// cmdContext shows current context information
func (m *Model) cmdContext() (tea.Model, tea.Cmd) {
	m.updateContextInfo()

	var info strings.Builder
	info.WriteString("Context Usage:\n")
	info.WriteString("  Tokens: ")
	info.WriteString(formatInt(m.contextInfo.UsedTokens))
	info.WriteString(" / ")
	info.WriteString(formatInt(m.contextInfo.MaxTokens))
	info.WriteString(" (")
	info.WriteString(formatInt(int(m.contextInfo.UsagePercent)))
	info.WriteString("%)\n")
	info.WriteString("  Remaining: ")
	info.WriteString(formatInt(m.contextInfo.RemainingTokens))
	info.WriteString(" tokens")

	// Add attachment info
	if m.currentSession != nil {
		atts, _ := m.store.GetActiveAttachments(m.currentSession.ID)
		if len(atts) > 0 {
			var totalSize int64
			for _, att := range atts {
				totalSize += att.SizeBytes
			}
			info.WriteString("\n  Attachments: ")
			info.WriteString(formatInt(len(atts)))
			info.WriteString(" file(s), ")
			info.WriteString(formatSize(totalSize))
		}
	}

	// Add summary info
	if m.currentSession != nil {
		summaries, _ := m.store.GetSummaries(m.currentSession.ID)
		if len(summaries) > 0 {
			var savedTokens int
			for _, s := range summaries {
				savedTokens += s.OriginalTokenCount - s.SummaryTokenCount
			}
			info.WriteString("\n  Summaries: ")
			info.WriteString(formatInt(len(summaries)))
			info.WriteString(" (saved ~")
			info.WriteString(formatInt(savedTokens))
			info.WriteString(" tokens)")
		}
	}

	m.statusMessage = info.String()
	return m, nil
}

// summarizeRequestMsg is sent when a summary needs to be generated
type summarizeRequestMsg struct {
	messages        []*store.Message
	summaryPrompt   string
	keepRecentCount int
	startMessageID  string
	endMessageID    string
}

// summarizeCompleteMsg is sent when a summary has been generated
type summarizeCompleteMsg struct {
	summary          string
	startMessageID   string
	endMessageID     string
	originalTokens   int
	summaryTokens    int
	err              error
}

// cmdThinking toggles Gemini thinking mode
func (m *Model) cmdThinking() (tea.Model, tea.Cmd) {
	// Check if current provider is Gemini
	if m.currentProvider == nil || m.currentProvider.Name() != "gemini" {
		m.errorMessage = "Thinking mode is only available for Gemini models"
		return m, nil
	}

	// Toggle thinking mode
	m.geminiThinking = !m.geminiThinking

	// Update the provider
	if gemini, ok := m.currentProvider.(*provider.Gemini); ok {
		gemini.SetThinkingEnabled(m.geminiThinking)
	}

	if m.geminiThinking {
		m.statusMessage = "Thinking mode enabled (Gemini will show reasoning)"
	} else {
		m.statusMessage = "Thinking mode disabled"
	}

	return m, nil
}

// cmdGrounding toggles Gemini Google Search grounding
func (m *Model) cmdGrounding() (tea.Model, tea.Cmd) {
	// Check if current provider is Gemini
	if m.currentProvider == nil || m.currentProvider.Name() != "gemini" {
		m.errorMessage = "Search grounding is only available for Gemini models"
		return m, nil
	}

	// Toggle grounding
	m.geminiGrounding = !m.geminiGrounding

	// Update the provider
	if gemini, ok := m.currentProvider.(*provider.Gemini); ok {
		gemini.SetSearchEnabled(m.geminiGrounding)
	}

	if m.geminiGrounding {
		m.statusMessage = "Search grounding enabled (Gemini will use Google Search)"
	} else {
		m.statusMessage = "Search grounding disabled"
	}

	return m, nil
}
