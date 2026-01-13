# ChatUI

A multi-provider AI chat TUI (Terminal User Interface) application built with Go and [Bubble Tea](https://github.com/charmbracelet/bubbletea). ChatUI provides a secure, local-first chat experience with support for multiple AI providers.

## Features

- **Multi-Provider Support**: OpenAI and Anthropic built-in, with an extensible provider interface
- **BYOK (Bring Your Own Key)**: Use your own API keys, stored securely
- **Local-First**: All data stored locally in SQLite - no telemetry, no cloud sync
- **Streaming Responses**: Real-time token-by-token output (where supported)
- **Session Management**: Create, switch between, and manage multiple chat sessions
- **Markdown Export**: Export conversations to Markdown files
- **Git Integration**: Optional auto-commit for exported transcripts
- **Security-Focused**: ANSI escape sanitization, secure key storage, no logging of sensitive data

## Screenshots

```
┌─────────────────────────────────────────────────────────────────┐
│ openai │ gpt-4o │ My Chat Session                               │
├─────────────────────────────────────────────────────────────────┤
│ You                                                             │
│ What is the capital of France?                                  │
│                                                                 │
│ AI                                                              │
│ The capital of France is Paris.                                 │
│                                                                 │
├─────────────────────────────────────────────────────────────────┤
│ > Type your message...                                          │
├─────────────────────────────────────────────────────────────────┤
│ Ctrl+Enter: Send | /help: Commands | Ctrl+Q: Quit               │
└─────────────────────────────────────────────────────────────────┘
```

## Installation

### Prerequisites

- Go 1.21 or later
- GCC (for SQLite compilation with CGO)

### Build from Source

```bash
# Clone the repository
git clone https://github.com/user/openchat.git
cd openchat

# Download dependencies
make deps

# Build
make build

# Or install to GOPATH/bin
make install
```

### Quick Start

```bash
# Set your API key (recommended method)
export OPENAI_API_KEY="sk-..."
# or
export ANTHROPIC_API_KEY="sk-ant-..."

# Run ChatUI
./build/chatui
```

## Usage

### Command Line Options

```bash
chatui [OPTIONS]

Options:
  --help      Show help information
  --version   Show version information
  --debug     Enable debug mode (logs to ~/.chatui/debug.log)
```

### In-App Commands

| Command | Description |
|---------|-------------|
| `/new [name]` | Create a new chat session |
| `/switch` | Open session switcher |
| `/connect` | Configure API keys |
| `/model` | Select provider and model |
| `/export` | Export current session to Markdown |
| `/clear` | Clear current session messages |
| `/rename <name>` | Rename current session |
| `/system <text>` | Set system prompt |
| `/help` | Show help screen |

### Keybindings

| Key | Action |
|-----|--------|
| `Ctrl+Enter` | Send message |
| `Ctrl+C` | Cancel streaming / Quit |
| `Ctrl+Q` | Quit application |
| `Esc` | Close modal / Cancel |
| `Up/Down` | Scroll chat / Navigate lists |
| `PgUp/PgDn` | Page scroll |
| `Tab` | Cycle options in dialogs |

## Configuration

ChatUI stores its configuration in `~/.chatui/`:

```
~/.chatui/
├── config.json    # Application configuration
├── chatui.db      # SQLite database
└── exports/       # Exported conversations
```

### Configuration File

`~/.chatui/config.json`:

```json
{
  "default_provider": "openai",
  "default_model": "gpt-4o",
  "export_path": "",
  "enable_tools": false,
  "git_auto_commit": false,
  "api_keys": {
    "openai": "",
    "anthropic": ""
  }
}
```

### Environment Variables

Environment variables take precedence over config file values:

| Variable | Description |
|----------|-------------|
| `OPENAI_API_KEY` | OpenAI API key |
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `GROQ_API_KEY` | Groq API key (future) |
| `OPENROUTER_API_KEY` | OpenRouter API key (future) |

**Recommended**: Use environment variables for API keys rather than storing them in the config file.

## Supported Providers

### OpenAI

- GPT-4o
- GPT-4o Mini
- GPT-4 Turbo
- GPT-3.5 Turbo

### Anthropic

- Claude Sonnet 4
- Claude 3.5 Sonnet
- Claude 3.5 Haiku
- Claude 3 Opus

## Security

ChatUI is designed with security in mind:

1. **API Key Protection**
   - Keys from environment variables are never written to disk
   - Config file is created with `0600` permissions (owner read/write only)
   - Keys are never logged or displayed in plain text
   - Use `/connect` to see masked key status

2. **Output Sanitization**
   - All model output is sanitized to remove ANSI escape sequences
   - Prevents terminal injection attacks
   - Control characters are stripped

3. **Local-Only Storage**
   - All data stored locally in SQLite
   - No telemetry or analytics
   - No data sent to third parties (except AI provider API calls)

4. **Disabled by Default**
   - Tool/function calling features are disabled by default
   - Shell execution features are stubbed and require explicit enablement

## Architecture

```
cmd/chatui/           # Application entry point
internal/
├── config/           # Configuration management
├── exporter/         # Markdown export and git integration
├── provider/         # AI provider interface and implementations
│   ├── provider.go   # Provider interface
│   ├── openai.go     # OpenAI implementation
│   └── anthropic.go  # Anthropic implementation
├── sanitize/         # Output sanitization
├── store/            # SQLite persistence
└── ui/               # Bubble Tea UI components
```

### Provider Interface

```go
type Provider interface {
    Name() string
    Models(ctx context.Context) ([]string, error)
    Send(ctx context.Context, req ChatRequest) (ChatResponse, error)
    Stream(ctx context.Context, req ChatRequest, onDelta func(string)) error
    SupportsStreaming() bool
}
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run short tests only
make test-short
```

### Linting

```bash
make lint
```

### Building for Development

```bash
# Build with debug symbols
make build-debug

# Run in debug mode
make run-debug
```

## Roadmap

- [ ] Additional providers (Groq, OpenRouter, local models via Ollama)
- [ ] Function/tool calling support
- [ ] Themes and custom color schemes
- [ ] Image input support (for vision models)
- [ ] Conversation search
- [ ] Import from other chat applications
- [ ] Plugin system for custom commands

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Acknowledgments

- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [go-sqlite3](https://github.com/mattn/go-sqlite3) - SQLite driver
