// ChatUI - A multi-provider AI chat TUI application
//
// ChatUI is a terminal-based chat application that supports multiple AI providers
// (OpenAI, Anthropic) with BYOK (Bring Your Own Key) support. It features:
//   - Local-first design with SQLite storage
//   - Streaming responses where supported
//   - Session management and export
//   - Secure API key handling
//
// Usage:
//
//	chatui [flags]
//
// Environment Variables:
//
//	OPENAI_API_KEY     - OpenAI API key
//	ANTHROPIC_API_KEY  - Anthropic API key
//	GROQ_API_KEY       - Groq API key (future)
//	OPENROUTER_API_KEY - OpenRouter API key (future)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/user/openchat/internal/config"
	"github.com/user/openchat/internal/exporter"
	"github.com/user/openchat/internal/provider"
	"github.com/user/openchat/internal/store"
	"github.com/user/openchat/internal/ui"
)

var (
	version = "0.1.0"
)

func main() {
	// Parse command line flags
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help information")
	debugMode := flag.Bool("debug", false, "Enable debug mode (logs to file)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ChatUI v%s\n", version)
		os.Exit(0)
	}

	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Set up logging
	if *debugMode {
		configDir, err := config.GetConfigDir()
		if err != nil {
			log.Fatal("Failed to get config directory:", err)
		}
		logFile, err := os.OpenFile(configDir+"/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			log.Fatal("Failed to open log file:", err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
		log.Println("ChatUI starting in debug mode")
	} else {
		// Disable logging in normal mode (security: don't log API keys)
		log.SetOutput(os.Stderr)
		log.SetFlags(0)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	dbPath, err := config.GetDBPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get database path: %v\n", err)
		os.Exit(1)
	}

	st, err := store.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	// Initialize exporter
	exportPath, err := cfg.GetExportPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get export path: %v\n", err)
		os.Exit(1)
	}
	exp := exporter.New(exportPath, cfg.GitAutoCommit)

	// Initialize provider registry
	registry := provider.NewRegistry()

	// Register OpenAI provider
	openaiKey := cfg.GetAPIKey("openai")
	openaiProvider := provider.NewOpenAI(openaiKey)
	registry.Register(openaiProvider)

	// Register Anthropic provider
	anthropicKey := cfg.GetAPIKey("anthropic")
	anthropicProvider := provider.NewAnthropic(anthropicKey)
	registry.Register(anthropicProvider)

	// TODO: Register additional providers (Groq, OpenRouter, local models)

	// Create UI model
	model := ui.NewModel(cfg, st, exp, registry)

	// Create and run Bubble Tea program
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	help := `
ChatUI - Multi-Provider AI Chat TUI

USAGE:
    chatui [OPTIONS]

OPTIONS:
    -h, --help      Show this help message
    -v, --version   Show version information
    --debug         Enable debug mode (logs to ~/.chatui/debug.log)

ENVIRONMENT VARIABLES:
    OPENAI_API_KEY      OpenAI API key
    ANTHROPIC_API_KEY   Anthropic API key
    GROQ_API_KEY        Groq API key (future support)
    OPENROUTER_API_KEY  OpenRouter API key (future support)

CONFIGURATION:
    Config file: ~/.chatui/config.json
    Database:    ~/.chatui/chatui.db
    Exports:     ~/.chatui/exports/

COMMANDS (in-app):
    /new [name]     Create a new chat session
    /switch         Switch between sessions
    /connect        Configure API keys
    /model          Select provider/model
    /export         Export session to Markdown
    /clear          Clear current session
    /rename <name>  Rename current session
    /system <text>  Set system prompt
    /help           Show help

KEYBINDINGS:
    Ctrl+Enter      Send message
    Ctrl+C          Cancel streaming / Quit
    Ctrl+Q          Quit application
    Esc             Close modal / Cancel
    Up/Down         Scroll chat / Navigate lists
    Tab             Cycle options in dialogs

SECURITY:
    - API keys from environment variables override config file
    - Config file is created with 0600 permissions
    - API keys are never logged or displayed in plain text
    - Model output is sanitized to prevent terminal injection

For more information, visit: https://github.com/user/openchat
`
	fmt.Println(help)
}
