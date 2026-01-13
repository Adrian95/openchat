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
	"github.com/user/openchat/internal/tokens"
)

// View represents the current view mode
type View int

const (
	ViewChat View = iota
	ViewSessions
	ViewConnect
	ViewModels
	ViewHelp
	ViewSearch
	ViewAttachments
	ViewAttachConfirm
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

	// Token estimation
	tokenEstimator *tokens.Estimator
	contextInfo    tokens.ContextInfo

	// Search state
	searchQuery       string
	searchResults     []*store.SearchResult
	searchIndex       int
	selectedSnippets  map[string]bool // Track selected snippets by message ID

	// Attachment state
	attachments         []*store.Attachment
	attachmentIndex     int
	pendingAttachment   *store.Attachment
	attachmentPreview   string
	attachMaxSize       int64 // Max file size in bytes (default 1MB)

	// Gemini-specific state
	geminiThinking  bool // Enable thinking mode for Gemini
	geminiGrounding bool // Enable Google Search grounding for Gemini
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
		config:           cfg,
		store:            st,
		exporter:         exp,
		registry:         reg,
		currentView:      ViewChat,
		textarea:         ta,
		messages:         make([]*store.Message, 0),
		sessions:         make([]*store.Session, 0),
		helpText:         generateHelpText(),
		tokenEstimator:   tokens.NewEstimator(cfg.GetDefaultProvider()),
		selectedSnippets: make(map[string]bool),
		attachMaxSize:    1024 * 1024, // 1MB default
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
		case ViewSearch:
			return m.updateSearch(msg)
		case ViewAttachments:
			return m.updateAttachments(msg)
		case ViewAttachConfirm:
			return m.updateAttachConfirm(msg)
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

	case searchResultsMsg:
		if msg.err != nil {
			m.errorMessage = "Search failed: " + msg.err.Error()
		} else {
			m.searchResults = msg.results
			m.searchIndex = 0
		}

	case jumpToMessageMsg:
		// Load the session and find the message
		m.currentView = ViewChat
		m.textarea.Focus()
		return m, m.loadSessionAndJumpTo(msg.sessionID, msg.messageID)

	case attachmentsLoadedMsg:
		if msg.err != nil {
			m.errorMessage = "Failed to load attachments: " + msg.err.Error()
		} else {
			m.attachments = msg.attachments
		}

	case attachmentAddedMsg:
		if msg.err != nil {
			m.errorMessage = "Failed to add attachment: " + msg.err.Error()
		} else {
			m.statusMessage = "Added: " + msg.attachment.Filename
			m.pendingAttachment = nil
			m.attachmentPreview = ""
		}
		m.currentView = ViewChat
		m.textarea.Focus()

	case filePreviewMsg:
		if msg.err != nil {
			m.errorMessage = "Failed to read file: " + msg.err.Error()
			return m, nil
		}
		// Create pending attachment for confirmation
		m.pendingAttachment = &store.Attachment{
			SessionID: m.currentSession.ID,
			Filename:  msg.filename,
			Filepath:  msg.filepath,
			Content:   msg.content,
			SizeBytes: msg.size,
			MimeType:  detectMimeType(msg.filename),
		}
		m.currentView = ViewAttachConfirm

	case summarizeRequestMsg:
		// Send the summary request to the AI
		return m, m.executeSummarize(msg)

	case summarizeCompleteMsg:
		if msg.err != nil {
			m.errorMessage = "Summarization failed: " + msg.err.Error()
		} else {
			// Store the summary
			_, err := m.store.AddSummary(
				m.currentSession.ID,
				msg.startMessageID,
				msg.endMessageID,
				msg.summary,
				msg.originalTokens,
				msg.summaryTokens,
			)
			if err != nil {
				m.errorMessage = "Failed to save summary: " + err.Error()
			} else {
				// Add summary as a system message for context
				summaryMsg, _ := m.store.AddMessage(m.currentSession.ID, store.RoleSummary,
					"[Summary of earlier conversation]\n"+msg.summary)
				if summaryMsg != nil {
					// Prepend summary to messages (after removing summarized ones)
					m.messages = append([]*store.Message{summaryMsg}, m.messages[len(m.messages)-4:]...)
				}
				m.statusMessage = "Created summary (saved ~" + formatInt(msg.originalTokens-msg.summaryTokens) + " tokens)"
				m.updateViewportContent()
			}
		}
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
	case ViewSearch:
		return m.viewSearch()
	case ViewAttachments:
		return m.viewAttachments()
	case ViewAttachConfirm:
		return m.viewAttachConfirm()
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

	// Context meter
	m.updateContextInfo()
	contextMeter := m.renderContextMeter()
	if contextMeter != "" {
		parts = append(parts, contextMeter)
	}

	// Attachments indicator
	if m.currentSession != nil {
		if atts, _ := m.store.GetActiveAttachments(m.currentSession.ID); len(atts) > 0 {
			parts = append(parts, attachmentIndicatorStyle.Render("📎 "+formatInt(len(atts))))
		}
	}

	// Gemini features indicator
	if m.currentProvider != nil && m.currentProvider.Name() == "gemini" {
		var geminiFeatures []string
		if m.geminiThinking {
			geminiFeatures = append(geminiFeatures, "🧠")
		}
		if m.geminiGrounding {
			geminiFeatures = append(geminiFeatures, "🔍")
		}
		if len(geminiFeatures) > 0 {
			parts = append(parts, geminiFeatureStyle.Render(strings.Join(geminiFeatures, "")))
		}
	}

	// Streaming indicator
	if m.streaming {
		parts = append(parts, streamingStyle.Render(" ● STREAMING"))
	}

	statusContent := lipgloss.JoinHorizontal(lipgloss.Left, parts...)
	return statusBarStyle.Width(m.width).Render(statusContent)
}

// updateContextInfo updates the context usage information
func (m *Model) updateContextInfo() {
	if m.tokenEstimator == nil {
		return
	}

	// Calculate tokens for all messages
	var msgs []tokens.Message
	for _, msg := range m.messages {
		msgs = append(msgs, tokens.Message{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}

	// Add system prompt tokens if present
	if m.currentSession != nil && m.currentSession.SystemPrompt != "" {
		msgs = append([]tokens.Message{{
			Role:    "system",
			Content: m.currentSession.SystemPrompt,
		}}, msgs...)
	}

	// Add attachment tokens if any
	if m.currentSession != nil {
		if atts, _ := m.store.GetActiveAttachments(m.currentSession.ID); len(atts) > 0 {
			for _, att := range atts {
				msgs = append(msgs, tokens.Message{
					Role:    "system",
					Content: att.Content,
				})
			}
		}
	}

	usedTokens := m.tokenEstimator.EstimateMessages(msgs)
	maxTokens := tokens.GetMaxTokensForModel(m.config.GetDefaultProvider(), m.config.GetDefaultModel())

	m.contextInfo = m.tokenEstimator.GetContextInfo(usedTokens, maxTokens)
}

// renderContextMeter renders the context usage meter
func (m *Model) renderContextMeter() string {
	if m.contextInfo.MaxTokens == 0 {
		return ""
	}

	// Create a visual bar
	barWidth := 10
	filled := int(m.contextInfo.UsagePercent / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := ""
	for i := 0; i < barWidth; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// Choose color based on warning level
	var style lipgloss.Style
	switch m.contextInfo.WarningLevel {
	case tokens.WarningCritical:
		style = contextCriticalStyle
	case tokens.WarningHigh:
		style = contextHighStyle
	case tokens.WarningMedium:
		style = contextMediumStyle
	case tokens.WarningLow:
		style = contextLowStyle
	default:
		style = contextNormalStyle
	}

	tokenStr := tokens.FormatTokenCount(m.contextInfo.UsedTokens)
	maxStr := tokens.FormatTokenCount(m.contextInfo.MaxTokens)

	return style.Render("[" + bar + "] " + tokenStr + "/" + maxStr)
}

// formatInt converts an int to string
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
		case store.RoleSummary:
			content.WriteString(summaryLabelStyle.String())
			content.WriteString("\n")
			content.WriteString(summaryMessageStyle.Render(sanitize.SanitizeForDisplay(msg.Content)))
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
╭───────────────────────────────────────────────────────╮
│                    ChatUI Help                        │
├───────────────────────────────────────────────────────┤
│                                                       │
│  SESSION COMMANDS                                     │
│  ────────────────                                     │
│  /new [name]       Create new chat session            │
│  /switch           Switch between sessions            │
│  /rename <name>    Rename current session             │
│  /delete           Delete current session             │
│  /export           Export session to Markdown         │
│                                                       │
│  PROVIDER & MODEL                                     │
│  ───────────────                                      │
│  /connect          Set API keys                       │
│  /model            Select provider/model              │
│  /system <prompt>  Set system prompt                  │
│                                                       │
│  SEARCH & RECALL                                      │
│  ──────────────                                       │
│  /search [query]   Search across all chats            │
│  /context          Show context usage info            │
│                                                       │
│  ATTACHMENTS                                          │
│  ───────────                                          │
│  /attach <path>    Attach file to context vault       │
│  /vault            Manage attached files              │
│                                                       │
│  SUMMARIZATION                                        │
│  ─────────────                                        │
│  /summarize [n]    Summarize older messages           │
│                    (keeps last n messages, default 4) │
│                                                       │
│  GEMINI FEATURES                                      │
│  ───────────────                                      │
│  /thinking         Toggle thinking mode (reasoning)   │
│  /grounding        Toggle Google Search grounding     │
│                                                       │
│  KEYBINDINGS                                          │
│  ──────────                                           │
│  Ctrl+Enter        Send message                       │
│  Ctrl+C            Cancel streaming / Quit            │
│  Ctrl+Q            Quit application                   │
│  Esc               Close modal / Cancel               │
│  Up/Down           Scroll chat history                │
│  PgUp/PgDn         Scroll page up/down                │
│                                                       │
│  Press 'q' or Esc to close this help                  │
╰───────────────────────────────────────────────────────╯
`
}

// loadSessionAndJumpTo loads a session and scrolls to a specific message
func (m *Model) loadSessionAndJumpTo(sessionID, messageID string) tea.Cmd {
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

		// Find the message index to scroll to
		// (for now we just load the session, scrolling would require viewport adjustment)
		return sessionLoadedMsg{session: session, messages: messages}
	}
}

// executeSummarize sends the summarization request to the AI provider
func (m *Model) executeSummarize(req summarizeRequestMsg) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Get provider
		prov, ok := m.registry.Get(m.config.GetDefaultProvider())
		if !ok {
			return summarizeCompleteMsg{err: provider.ErrNoAPIKey}
		}

		// Set API key
		apiKey := m.config.GetAPIKey(prov.Name())
		switch p := prov.(type) {
		case *provider.OpenAI:
			p.SetAPIKey(apiKey)
		case *provider.Anthropic:
			p.SetAPIKey(apiKey)
		case *provider.Gemini:
			p.SetAPIKey(apiKey)
		}

		// Create the summarization request
		chatReq := provider.ChatRequest{
			Model: m.config.GetDefaultModel(),
			Messages: []provider.Message{
				{
					Role:    provider.RoleUser,
					Content: req.summaryPrompt,
				},
			},
		}

		// Send request
		resp, err := prov.Send(ctx, chatReq)
		if err != nil {
			return summarizeCompleteMsg{err: err}
		}

		// Estimate token counts
		originalTokens := m.tokenEstimator.EstimateTokens(req.summaryPrompt)
		summaryTokens := m.tokenEstimator.EstimateTokens(resp.Content)

		return summarizeCompleteMsg{
			summary:        resp.Content,
			startMessageID: req.startMessageID,
			endMessageID:   req.endMessageID,
			originalTokens: originalTokens,
			summaryTokens:  summaryTokens,
			err:            nil,
		}
	}
}

// detectMimeType detects the MIME type based on file extension
func detectMimeType(filename string) string {
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	}

	switch ext {
	case ".txt":
		return "text/plain"
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".ts":
		return "application/typescript"
	case ".go":
		return "text/x-go"
	case ".py":
		return "text/x-python"
	case ".rs":
		return "text/x-rust"
	case ".java":
		return "text/x-java"
	case ".c", ".h":
		return "text/x-c"
	case ".cpp", ".hpp", ".cc":
		return "text/x-c++"
	case ".sh", ".bash":
		return "text/x-shellscript"
	case ".yaml", ".yml":
		return "text/x-yaml"
	case ".toml":
		return "text/x-toml"
	case ".sql":
		return "text/x-sql"
	case ".csv":
		return "text/csv"
	default:
		return "text/plain"
	}
}
