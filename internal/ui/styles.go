// Package ui implements the Bubble Tea TUI for the chat application.
package ui

import (
	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	primaryColor   = lipgloss.Color("69")
	secondaryColor = lipgloss.Color("99")
	successColor   = lipgloss.Color("42")
	warningColor   = lipgloss.Color("214")
	errorColor     = lipgloss.Color("196")
	mutedColor     = lipgloss.Color("241")
	userColor      = lipgloss.Color("117")
	assistantColor = lipgloss.Color("183")
)

// Styles for UI components
var (
	// Status bar styles
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	statusProviderStyle = lipgloss.NewStyle().
				Background(primaryColor).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Bold(true)

	statusModelStyle = lipgloss.NewStyle().
				Background(secondaryColor).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1)

	statusSessionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)

	// Input area styles
	inputPromptStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	inputTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	inputPlaceholderStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Italic(true)

	// Message styles
	userMessageStyle = lipgloss.NewStyle().
				Foreground(userColor).
				Bold(true)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(assistantColor)

	systemMessageStyle = lipgloss.NewStyle().
				Foreground(mutedColor).
				Italic(true)

	// Role labels
	userLabelStyle = lipgloss.NewStyle().
			Foreground(userColor).
			Bold(true).
			SetString("You")

	assistantLabelStyle = lipgloss.NewStyle().
				Foreground(assistantColor).
				Bold(true).
				SetString("AI")

	// Help and info styles
	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Padding(0, 1)

	infoStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	// Session list styles
	sessionListTitleStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true).
				Padding(0, 1)

	sessionItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Padding(0, 1)

	sessionSelectedStyle = lipgloss.NewStyle().
				Background(primaryColor).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1)

	// Modal/overlay styles
	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	// Viewport border
	viewportStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(mutedColor)

	// Title style
	titleStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true).
			Padding(0, 1)

	// Divider
	dividerStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			SetString("─")

	// Streaming indicator
	streamingStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true).
			Blink(true)
)

// TODO: Add theme support - light/dark mode switching
// TODO: Add custom color configuration from config file
