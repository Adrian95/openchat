package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

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
