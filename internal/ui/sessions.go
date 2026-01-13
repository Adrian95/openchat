package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// updateSessions handles updates in the sessions view
func (m *Model) updateSessions(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.sessionIndex > 0 {
			m.sessionIndex--
		}

	case "down", "j":
		if m.sessionIndex < len(m.sessions)-1 {
			m.sessionIndex++
		}

	case "enter":
		if len(m.sessions) > 0 && m.sessionIndex < len(m.sessions) {
			selectedSession := m.sessions[m.sessionIndex]
			m.currentView = ViewChat
			m.textarea.Focus()
			return m, m.loadSession(selectedSession.ID)
		}

	case "n":
		// Quick new session
		m.currentView = ViewChat
		m.textarea.Focus()
		return m.cmdNew(nil)

	case "d":
		// Delete selected session
		if len(m.sessions) > 0 && m.sessionIndex < len(m.sessions) {
			sessionToDelete := m.sessions[m.sessionIndex]
			// Don't delete if it's the current session
			if m.currentSession != nil && sessionToDelete.ID == m.currentSession.ID {
				m.currentSession = nil
				m.messages = nil
			}
			return m, func() tea.Msg {
				err := m.store.DeleteSession(sessionToDelete.ID)
				if err != nil {
					return errorMsg("Failed to delete: " + err.Error())
				}
				sessions, _ := m.store.ListSessions()
				return sessionsLoadedMsg{sessions: sessions}
			}
		}

	case "esc", "q":
		m.currentView = ViewChat
		m.textarea.Focus()
	}

	return m, nil
}

// viewSessions renders the session list view
func (m *Model) viewSessions() string {
	var b strings.Builder

	// Title
	title := sessionListTitleStyle.Render("📚 Sessions")
	b.WriteString(title)
	b.WriteString("\n\n")

	if !m.sessionsLoaded {
		b.WriteString("Loading sessions...")
		return modalStyle.Width(m.width - 4).Render(b.String())
	}

	if len(m.sessions) == 0 {
		b.WriteString(mutedStyle("No sessions yet. Press 'n' to create one."))
	} else {
		// Session list
		for i, session := range m.sessions {
			// Format: [index] Name (provider/model) - message count
			messageCount, _ := m.store.GetMessageCount(session.ID)
			item := fmt.Sprintf("%s (%s/%s) - %d messages",
				session.Name,
				session.Provider,
				truncateString(session.Model, 20),
				messageCount,
			)

			// Highlight current session
			isCurrent := m.currentSession != nil && session.ID == m.currentSession.ID

			if i == m.sessionIndex {
				// Selected
				marker := "▶ "
				if isCurrent {
					marker = "▶★"
				}
				b.WriteString(sessionSelectedStyle.Render(marker + item))
			} else {
				// Not selected
				marker := "  "
				if isCurrent {
					marker = " ★"
				}
				b.WriteString(sessionItemStyle.Render(marker + item))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: Navigate | Enter: Select | n: New | d: Delete | Esc: Back"))

	return modalStyle.Width(m.width - 4).Render(b.String())
}

// Helper to create muted text
func mutedStyle(s string) string {
	return lipgloss.NewStyle().Foreground(mutedColor).Render(s)
}

// truncateString truncates a string to maxLen
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
