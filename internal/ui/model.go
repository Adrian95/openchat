package ui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/user/openchat/internal/config"
	"github.com/user/openchat/internal/exporter"
	"github.com/user/openchat/internal/provider"
	"github.com/user/openchat/internal/sanitize"
	"github.com/user/openchat/internal/store"
)

// View represents the current view mode
type View int

const (
	ViewChat View = iota
	ViewSessions
	ViewConnect
	ViewModels
	ViewHelp
)

// Model is the main Bubble Tea model for the chat UI
type Model struct {
	// Core dependencies
	config   *config.Config
	store    *store.Store
	exporter *exporter.Exporter
	registry *provider.Registry

	// Current state
	currentView     View
	currentSession  *store.Session
	currentProvider provider.Provider
	messages        []*store.Message

	// UI components
	viewport viewport.Model
	textarea textarea.Model
	width    int
	height   int
	ready    bool

	// Session list state
	sessions        []*store.Session
	sessionIndex    int
	sessionsLoaded  bool

	// Connect screen state
	connectProvider string
	connectKey      string
	connectPersist  bool
	connectError    string

	// Model selection state
	availableModels []string
	modelIndex      int

	// Streaming state
	streaming       bool
	streamContent   strings.Builder
	streamCancel    context.CancelFunc

	// Status and errors
	statusMessage string
	errorMessage  string

	// Help text
	helpText string
}

// NewModel creates a new chat UI model
func NewModel(cfg *config.Config, st *store.Store, exp *exporter.Exporter, reg *provider.Registry) *Model {
	// Initialize textarea
	ta := textarea.New()
	ta.Placeholder = "Type your message... (Ctrl+Enter to send, /help for commands)"
	ta.Focus()
	ta.CharLimit = 10000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false

	m := &Model{
		config:       cfg,
		store:        st,
		exporter:     exp,
		registry:     reg,
		currentView:  ViewChat,
		textarea:     ta,
		messages:     make([]*store.Message, 0),
		sessions:     make([]*store.Session, 0),
		helpText:     generateHelpText(),
	}

	// Set up default provider
	m.initProvider()

	return m
}

// initProvider initializes the current provider based on config
func (m *Model) initProvider() {
	providerName := m.config.GetDefaultProvider()
	if p, ok := m.registry.Get(providerName); ok {
		m.currentProvider = p
	}
}

// Init implements tea.Model
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.loadSessions(),
		m.loadMostRecentSession(),
	)
}

// Update implements tea.Model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global key bindings
		switch msg.String() {
		case "ctrl+c":
			if m.streaming {
				m.cancelStream()
				return m, nil
			}
			return m, tea.Quit

		case "ctrl+q":
			return m, tea.Quit

		case "esc":
			if m.currentView != ViewChat {
				m.currentView = ViewChat
				m.textarea.Focus()
				return m, nil
			}
			if m.streaming {
				m.cancelStream()
				return m, nil
			}
		}

		// View-specific handling
		switch m.currentView {
		case ViewChat:
			return m.updateChat(msg)
		case ViewSessions:
			return m.updateSessions(msg)
		case ViewConnect:
			return m.updateConnect(msg)
		case ViewModels:
			return m.updateModels(msg)
		case ViewHelp:
			if msg.String() == "q" || msg.String() == "esc" {
				m.currentView = ViewChat
				m.textarea.Focus()
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3 // Status bar
		inputHeight := 5  // Input area
		footerHeight := 1 // Help line

		vpHeight := m.height - headerHeight - inputHeight - footerHeight
		if vpHeight < 1 {
			vpHeight = 1
		}

		if !m.ready {
			m.viewport = viewport.New(m.width-2, vpHeight)
			m.viewport.HighPerformanceRendering = false
			m.ready = true
		} else {
			m.viewport.Width = m.width - 2
			m.viewport.Height = vpHeight
		}

		m.textarea.SetWidth(m.width - 4)
		m.updateViewportContent()

	case sessionsLoadedMsg:
		m.sessions = msg.sessions
		m.sessionsLoaded = true

	case sessionLoadedMsg:
		if msg.session != nil {
			m.currentSession = msg.session
			m.messages = msg.messages
			m.updateViewportContent()
		}

	case streamDeltaMsg:
		if m.streaming {
			m.streamContent.WriteString(string(msg))
			m.updateViewportContent()
		}

	case streamCompleteMsg:
		m.streaming = false
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
		} else {
			// Save assistant message
			content := sanitize.Sanitize(m.streamContent.String())
			if m.currentSession != nil && content != "" {
				dbMsg, err := m.store.AddMessage(m.currentSession.ID, store.RoleAssistant, content)
				if err == nil {
					m.messages = append(m.messages, dbMsg)
				}
			}
		}
		m.streamContent.Reset()
		m.updateViewportContent()

	case sessionCreatedMsg:
		m.currentSession = msg.session
		m.messages = make([]*store.Message, 0)
		m.statusMessage = "New session created: " + msg.session.Name
		cmds = append(cmds, m.loadSessions())
		m.updateViewportContent()

	case exportCompleteMsg:
		if msg.err != nil {
			m.errorMessage = "Export failed: " + msg.err.Error()
		} else {
			m.statusMessage = "Exported to: " + msg.path
		}

	case errorMsg:
		m.errorMessage = string(msg)
	}

	// Update viewport scrolling
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// View implements tea.Model
func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	switch m.currentView {
	case ViewSessions:
		return m.viewSessions()
	case ViewConnect:
		return m.viewConnect()
	case ViewModels:
		return m.viewModels()
	case ViewHelp:
		return m.viewHelp()
	default:
		return m.viewChat()
	}
}

// viewChat renders the main chat view
func (m *Model) viewChat() string {
	var b strings.Builder

	// Status bar
	b.WriteString(m.renderStatusBar())
	b.WriteString("\n")

	// Chat viewport with border
	viewportContent := viewportStyle.Render(m.viewport.View())
	b.WriteString(viewportContent)
	b.WriteString("\n")

	// Input area
	b.WriteString(m.renderInputArea())
	b.WriteString("\n")

	// Footer with help and status
	b.WriteString(m.renderFooter())

	return b.String()
}

// renderStatusBar renders the status bar
func (m *Model) renderStatusBar() string {
	var parts []string

	// Provider
	providerName := "none"
	if m.currentProvider != nil {
		providerName = m.currentProvider.Name()
	}
	parts = append(parts, statusProviderStyle.Render(providerName))

	// Model
	modelName := m.config.GetDefaultModel()
	parts = append(parts, statusModelStyle.Render(modelName))

	// Session
	sessionName := "No session"
	if m.currentSession != nil {
		sessionName = m.currentSession.Name
	}
	parts = append(parts, statusSessionStyle.Render(sessionName))

	// Streaming indicator
	if m.streaming {
		parts = append(parts, streamingStyle.Render(" ● STREAMING"))
	}

	statusContent := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	return statusBarStyle.Width(m.width).Render(statusContent)
}

// renderInputArea renders the input text area
func (m *Model) renderInputArea() string {
	prompt := inputPromptStyle.Render("> ")
	return prompt + m.textarea.View()
}

// renderFooter renders the footer with help text and status
func (m *Model) renderFooter() string {
	helpKeys := helpStyle.Render("Ctrl+Enter: Send | /help: Commands | Ctrl+Q: Quit")

	var status string
	if m.errorMessage != "" {
		status = errorStyle.Render("Error: " + m.errorMessage)
	} else if m.statusMessage != "" {
		status = infoStyle.Render(m.statusMessage)
	}

	if status != "" {
		return lipgloss.JoinHorizontal(lipgloss.Left, helpKeys, "  ", status)
	}
	return helpKeys
}

// updateViewportContent updates the chat viewport content
func (m *Model) updateViewportContent() {
	var content strings.Builder

	for _, msg := range m.messages {
		switch msg.Role {
		case store.RoleUser:
			content.WriteString(userLabelStyle.String())
			content.WriteString("\n")
			content.WriteString(sanitize.SanitizeForDisplay(msg.Content))
			content.WriteString("\n\n")
		case store.RoleAssistant:
			content.WriteString(assistantLabelStyle.String())
			content.WriteString("\n")
			content.WriteString(sanitize.SanitizeForDisplay(msg.Content))
			content.WriteString("\n\n")
		case store.RoleSystem:
			content.WriteString(systemMessageStyle.Render("System: " + sanitize.SanitizeForDisplay(msg.Content)))
			content.WriteString("\n\n")
		}
	}

	// Streaming content
	if m.streaming && m.streamContent.Len() > 0 {
		content.WriteString(assistantLabelStyle.String())
		content.WriteString(" ")
		content.WriteString(streamingStyle.Render("●"))
		content.WriteString("\n")
		content.WriteString(sanitize.SanitizeForDisplay(m.streamContent.String()))
	}

	m.viewport.SetContent(content.String())
	m.viewport.GotoBottom()
}

// cancelStream cancels the current streaming operation
func (m *Model) cancelStream() {
	if m.streamCancel != nil {
		m.streamCancel()
		m.streamCancel = nil
	}
	m.streaming = false
}

func generateHelpText() string {
	return `
╭─────────────────────────────────────────────────╮
│              ChatUI Help                        │
├─────────────────────────────────────────────────┤
│                                                 │
│  COMMANDS                                       │
│  ────────                                       │
│  /new [name]     Create new chat session        │
│  /switch         Switch between sessions        │
│  /connect        Set API keys                   │
│  /model          Select provider/model          │
│  /export         Export session to Markdown     │
│  /help           Show this help                 │
│                                                 │
│  KEYBINDINGS                                    │
│  ──────────                                     │
│  Ctrl+Enter      Send message                   │
│  Ctrl+C          Cancel streaming / Quit        │
│  Ctrl+Q          Quit application               │
│  Esc             Close modal / Cancel           │
│  Up/Down         Scroll chat history            │
│  PgUp/PgDn       Scroll page up/down            │
│                                                 │
│  Press 'q' or Esc to close this help            │
╰─────────────────────────────────────────────────╯
`
}
