package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/provider"
)

// updateModels handles updates in the model selection view
func (m *Model) updateModels(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.modelIndex > 0 {
			m.modelIndex--
		}

	case "down", "j":
		if m.modelIndex < len(m.availableModels)-1 {
			m.modelIndex++
		}

	case "tab":
		// Switch provider
		providers := m.registry.List()
		currentProvider := m.config.GetDefaultProvider()
		for i, p := range providers {
			if p == currentProvider {
				nextProvider := providers[(i+1)%len(providers)]
				m.config.SetDefaultProvider(nextProvider)
				m.initProvider()
				m.modelIndex = 0
				return m, m.loadModels()
			}
		}

	case "enter":
		if len(m.availableModels) > 0 && m.modelIndex < len(m.availableModels) {
			selectedModel := m.availableModels[m.modelIndex]
			m.config.SetDefaultModel(selectedModel)

			// Update current session if exists
			if m.currentSession != nil {
				m.currentSession.Model = selectedModel
				m.currentSession.Provider = m.config.GetDefaultProvider()
				m.store.UpdateSession(m.currentSession)
			}

			m.statusMessage = "Model set to: " + selectedModel
			m.currentView = ViewChat
			m.textarea.Focus()
		}

	case "esc", "q":
		m.currentView = ViewChat
		m.textarea.Focus()
	}

	return m, nil
}

// viewModels renders the model selection view
func (m *Model) viewModels() string {
	var b strings.Builder

	// Title
	b.WriteString(sessionListTitleStyle.Render("🤖 Select Model"))
	b.WriteString("\n\n")

	// Provider tabs
	b.WriteString("Provider: ")
	currentProvider := m.config.GetDefaultProvider()
	for _, p := range m.registry.List() {
		if p == currentProvider {
			b.WriteString(statusProviderStyle.Render(" " + p + " "))
		} else {
			b.WriteString(sessionItemStyle.Render(" " + p + " "))
		}
	}
	b.WriteString("\n\n")

	// Current model info
	b.WriteString("Current: ")
	b.WriteString(statusModelStyle.Render(m.config.GetDefaultModel()))
	b.WriteString("\n\n")

	// Model list
	b.WriteString("Available Models:\n")

	// Get model info for descriptions
	modelInfoMap := make(map[string]provider.ModelInfo)
	if models, ok := provider.DefaultModels[currentProvider]; ok {
		for _, mi := range models {
			modelInfoMap[mi.ID] = mi
		}
	}

	if len(m.availableModels) == 0 {
		b.WriteString(mutedStyle("  Loading models..."))
	} else {
		for i, model := range m.availableModels {
			// Format with description if available
			displayText := model
			if info, ok := modelInfoMap[model]; ok && info.Description != "" {
				displayText = model + " - " + info.Description
			}

			if i == m.modelIndex {
				b.WriteString(sessionSelectedStyle.Render("▶ " + displayText))
			} else {
				b.WriteString(sessionItemStyle.Render("  " + displayText))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: Navigate | Tab: Switch provider | Enter: Select | Esc: Back"))

	return modalStyle.Width(m.width - 4).Render(b.String())
}

// viewHelp renders the help view
func (m *Model) viewHelp() string {
	return modalStyle.Width(m.width - 4).Render(m.helpText)
}
