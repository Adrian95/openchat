package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/user/openchat/internal/store"
)

// Message types for search
type searchResultsMsg struct {
	results []*store.SearchResult
	err     error
}

type jumpToMessageMsg struct {
	sessionID string
	messageID string
}

// updateSearch handles key events in the search view
func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.currentView = ViewChat
		m.searchQuery = ""
		m.searchResults = nil
		m.searchIndex = 0
		m.selectedSnippets = make(map[string]bool)
		m.textarea.Focus()
		return m, nil

	case "up", "k":
		if m.searchIndex > 0 {
			m.searchIndex--
		}
		return m, nil

	case "down", "j":
		if m.searchIndex < len(m.searchResults)-1 {
			m.searchIndex++
		}
		return m, nil

	case "enter":
		// Jump to selected result
		if len(m.searchResults) > 0 && m.searchIndex < len(m.searchResults) {
			result := m.searchResults[m.searchIndex]
			return m, m.jumpToMessage(result.SessionID, result.MessageID)
		}
		return m, nil

	case " ": // Space to toggle selection
		if len(m.searchResults) > 0 && m.searchIndex < len(m.searchResults) {
			result := m.searchResults[m.searchIndex]
			if m.selectedSnippets[result.MessageID] {
				delete(m.selectedSnippets, result.MessageID)
			} else {
				m.selectedSnippets[result.MessageID] = true
			}
		}
		return m, nil

	case "s": // Send selected snippets to model
		return m.sendSelectedSnippets()

	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			return m, m.performSearch()
		}
		return m, nil

	default:
		// Add character to search query
		if len(msg.String()) == 1 {
			m.searchQuery += msg.String()
			return m, m.performSearch()
		}
	}

	return m, nil
}

// performSearch executes the full-text search
func (m *Model) performSearch() tea.Cmd {
	query := strings.TrimSpace(m.searchQuery)
	if query == "" {
		m.searchResults = nil
		return nil
	}

	return func() tea.Msg {
		results, err := m.store.FullTextSearch(query, 50)
		return searchResultsMsg{results: results, err: err}
	}
}

// jumpToMessage loads the session and scrolls to the specific message
func (m *Model) jumpToMessage(sessionID, messageID string) tea.Cmd {
	return func() tea.Msg {
		return jumpToMessageMsg{sessionID: sessionID, messageID: messageID}
	}
}

// sendSelectedSnippets sends the selected search results as context to the model
func (m *Model) sendSelectedSnippets() (tea.Model, tea.Cmd) {
	if len(m.selectedSnippets) == 0 {
		m.errorMessage = "No snippets selected. Use Space to select."
		return m, nil
	}

	// Build context from selected snippets
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Here are some relevant snippets from previous conversations:\n\n")

	for _, result := range m.searchResults {
		if m.selectedSnippets[result.MessageID] {
			contextBuilder.WriteString("From session \"")
			contextBuilder.WriteString(result.SessionName)
			contextBuilder.WriteString("\" (")
			contextBuilder.WriteString(string(result.Role))
			contextBuilder.WriteString("):\n")
			contextBuilder.WriteString("\"")
			contextBuilder.WriteString(result.Content)
			contextBuilder.WriteString("\"\n\n")
		}
	}

	// Store the context in the textarea for the user to review/edit before sending
	m.textarea.SetValue(contextBuilder.String())

	// Return to chat view
	m.currentView = ViewChat
	m.searchQuery = ""
	m.searchResults = nil
	m.searchIndex = 0
	m.selectedSnippets = make(map[string]bool)
	m.textarea.Focus()

	m.statusMessage = "Snippets added to input. Review and send when ready."

	return m, nil
}

// viewSearch renders the search view
func (m *Model) viewSearch() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Search Across Chats")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Search input
	searchPrompt := inputPromptStyle.Render("Search: ")
	b.WriteString(searchPrompt)
	b.WriteString(m.searchQuery)
	b.WriteString("_") // Cursor
	b.WriteString("\n\n")

	// Selected count
	if len(m.selectedSnippets) > 0 {
		selectedInfo := infoStyle.Render("Selected: " + formatInt(len(m.selectedSnippets)) + " snippet(s)")
		b.WriteString(selectedInfo)
		b.WriteString("\n\n")
	}

	// Results
	if len(m.searchResults) == 0 && m.searchQuery != "" {
		b.WriteString(mutedTextStyle.Render("No results found."))
		b.WriteString("\n")
	} else {
		// Calculate how many results we can show
		maxResults := m.height - 12
		if maxResults < 5 {
			maxResults = 5
		}

		startIdx := 0
		if m.searchIndex >= maxResults {
			startIdx = m.searchIndex - maxResults + 1
		}

		for i := startIdx; i < len(m.searchResults) && i < startIdx+maxResults; i++ {
			result := m.searchResults[i]

			// Selection marker
			var marker string
			if m.selectedSnippets[result.MessageID] {
				marker = "[x] "
			} else {
				marker = "[ ] "
			}

			// Session name and role
			header := result.SessionName + " (" + string(result.Role) + ")"

			// Snippet with highlighting
			snippet := formatSearchSnippet(result.Snippet)

			// Apply styles
			var style lipgloss.Style
			if i == m.searchIndex {
				style = searchSelectedStyle
			} else {
				style = searchResultStyle
			}

			line := style.Render(marker + header)
			b.WriteString(line)
			b.WriteString("\n")
			b.WriteString("  ")
			b.WriteString(searchSnippetStyle.Render(snippet))
			b.WriteString("\n\n")
		}
	}

	// Help
	helpText := helpStyle.Render("↑/↓: Navigate | Space: Select | Enter: Jump to | s: Send selected | Esc: Close")
	b.WriteString("\n")
	b.WriteString(helpText)

	return modalStyle.Width(m.width - 4).Render(b.String())
}

// formatSearchSnippet formats a snippet with highlight markers
func formatSearchSnippet(snippet string) string {
	// Replace our highlight markers with styled text
	// The FTS5 query uses >>> and <<< as markers
	result := strings.ReplaceAll(snippet, ">>>", "**")
	result = strings.ReplaceAll(result, "<<<", "**")

	// Truncate if too long
	if len(result) > 200 {
		result = result[:197] + "..."
	}

	return result
}

// mutedTextStyle for empty states
var mutedTextStyle = lipgloss.NewStyle().Foreground(mutedColor)
