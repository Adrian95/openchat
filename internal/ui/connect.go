package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/config"
)

// updateConnect handles updates in the connect/API key view
func (m *Model) updateConnect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		// Cycle through providers
		providers := []string{"openai", "anthropic", "groq", "openrouter"}
		for i, p := range providers {
			if p == m.connectProvider {
				m.connectProvider = providers[(i+1)%len(providers)]
				break
			}
		}

	case "enter":
		// Save the key
		if m.connectKey == "" {
			m.connectError = "Please enter an API key"
			return m, nil
		}

		err := m.config.SetAPIKey(m.connectProvider, m.connectKey, m.connectPersist)
		if err != nil {
			m.connectError = "Failed to save key: " + err.Error()
			return m, nil
		}

		// Update provider
		m.initProvider()
		m.statusMessage = "API key saved for " + m.connectProvider

		// Return to chat
		m.currentView = ViewChat
		m.textarea.Focus()
		m.connectKey = "" // Clear key from memory

	case "p":
		// Toggle persist
		m.connectPersist = !m.connectPersist

	case "esc", "q":
		m.currentView = ViewChat
		m.textarea.Focus()
		m.connectKey = "" // Clear key from memory

	case "backspace":
		if len(m.connectKey) > 0 {
			m.connectKey = m.connectKey[:len(m.connectKey)-1]
		}

	default:
		// Add character to key (only printable)
		if len(msg.String()) == 1 && msg.String()[0] >= 32 && msg.String()[0] < 127 {
			m.connectKey += msg.String()
		}
	}

	return m, nil
}

// viewConnect renders the API key configuration view
func (m *Model) viewConnect() string {
	var b strings.Builder

	// Title
	b.WriteString(sessionListTitleStyle.Render("🔑 Connect Provider"))
	b.WriteString("\n\n")

	// Provider selection
	b.WriteString("Provider: ")
	providers := []string{"openai", "anthropic", "groq", "openrouter"}
	for _, p := range providers {
		if p == m.connectProvider {
			b.WriteString(statusProviderStyle.Render(" " + p + " "))
		} else {
			b.WriteString(sessionItemStyle.Render(" " + p + " "))
		}
	}
	b.WriteString("\n\n")

	// Current status
	if m.config.HasAPIKey(m.connectProvider) {
		currentKey := m.config.GetAPIKey(m.connectProvider)
		b.WriteString(successStyle.Render("✓ Key configured: " + config.MaskKey(currentKey)))
	} else {
		b.WriteString(warningStyle.Render("✗ No key configured"))
	}
	b.WriteString("\n\n")

	// Key input (masked)
	b.WriteString("Enter API Key: ")
	if m.connectKey == "" {
		b.WriteString(inputPlaceholderStyle.Render("(paste or type key)"))
	} else {
		// Show masked key
		masked := strings.Repeat("•", len(m.connectKey))
		if len(masked) > 40 {
			masked = masked[:40] + "..."
		}
		b.WriteString(inputTextStyle.Render(masked))
	}
	b.WriteString("\n\n")

	// Persist option
	persistStatus := "[ ]"
	if m.connectPersist {
		persistStatus = "[✓]"
	}
	b.WriteString(sessionItemStyle.Render(persistStatus + " Save to config file (press 'p' to toggle)"))
	b.WriteString("\n")

	if !m.connectPersist {
		b.WriteString(mutedStyle("  Key will only be kept in memory for this session"))
	} else {
		b.WriteString(warningStyle.Render("  ⚠ Key will be saved to ~/.chatui/config.json"))
	}
	b.WriteString("\n\n")

	// Error message
	if m.connectError != "" {
		b.WriteString(errorStyle.Render("Error: " + m.connectError))
		b.WriteString("\n\n")
	}

	// Help
	b.WriteString(helpStyle.Render("Tab: Switch provider | Enter: Save | p: Toggle persist | Esc: Cancel"))

	return modalStyle.Width(m.width - 4).Render(b.String())
}
