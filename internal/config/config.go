// Package config handles application configuration loading, saving, and API key management.
// Configuration is stored in ~/.chatui/config.json with restrictive permissions.
// Environment variables override config file values for API keys.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// DefaultConfigDir is the directory name for config files
	DefaultConfigDir = ".chatui"
	// DefaultConfigFile is the config file name
	DefaultConfigFile = "config.json"
	// DefaultExportDir is the default export directory name
	DefaultExportDir = "exports"
	// DefaultDBFile is the default database file name
	DefaultDBFile = "chatui.db"

	// Environment variable names for API keys
	EnvOpenAIKey    = "OPENAI_API_KEY"
	EnvAnthropicKey = "ANTHROPIC_API_KEY"
	EnvGroqKey      = "GROQ_API_KEY"
	EnvOpenRouterKey = "OPENROUTER_API_KEY"
)

// Config represents the application configuration
type Config struct {
	// DefaultProvider is the default AI provider (openai, anthropic, etc.)
	DefaultProvider string `json:"default_provider"`
	// DefaultModel is the default model to use
	DefaultModel string `json:"default_model"`
	// ExportPath is the directory for markdown exports
	ExportPath string `json:"export_path"`
	// EnableTools enables tool/function calling features (disabled by default for security)
	EnableTools bool `json:"enable_tools"`
	// GitAutoCommit enables automatic git commits for exports
	GitAutoCommit bool `json:"git_auto_commit"`
	// APIKeys stores API keys (use env vars instead when possible)
	APIKeys APIKeys `json:"api_keys,omitempty"`

	// Runtime-only fields (not persisted)
	configPath string
	mu         sync.RWMutex
}

// APIKeys holds API keys for various providers
// Note: Prefer environment variables over storing keys in config
type APIKeys struct {
	OpenAI     string `json:"openai,omitempty"`
	Anthropic  string `json:"anthropic,omitempty"`
	Groq       string `json:"groq,omitempty"`
	OpenRouter string `json:"openrouter,omitempty"`
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
		ExportPath:      "",
		EnableTools:     false,
		GitAutoCommit:   false,
		APIKeys:         APIKeys{},
	}
}

// GetConfigDir returns the configuration directory path
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, DefaultConfigDir), nil
}

// GetConfigPath returns the full path to the config file
func GetConfigPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultConfigFile), nil
}

// GetDBPath returns the full path to the database file
func GetDBPath() (string, error) {
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultDBFile), nil
}

// GetExportPath returns the export directory path
func (c *Config) GetExportPath() (string, error) {
	if c.ExportPath != "" {
		return c.ExportPath, nil
	}
	dir, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DefaultExportDir), nil
}

// Load reads configuration from the config file and applies environment variable overrides
func Load() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	cfg.configPath = configPath

	// Ensure config directory exists with restrictive permissions
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Try to read existing config
	data, err := os.ReadFile(configPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Config doesn't exist, use defaults and apply env overrides
			cfg.applyEnvOverrides()
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply environment variable overrides (env vars take precedence)
	cfg.applyEnvOverrides()

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the config
func (c *Config) applyEnvOverrides() {
	if key := os.Getenv(EnvOpenAIKey); key != "" {
		c.APIKeys.OpenAI = key
	}
	if key := os.Getenv(EnvAnthropicKey); key != "" {
		c.APIKeys.Anthropic = key
	}
	if key := os.Getenv(EnvGroqKey); key != "" {
		c.APIKeys.Groq = key
	}
	if key := os.Getenv(EnvOpenRouterKey); key != "" {
		c.APIKeys.OpenRouter = key
	}
}

// Save writes the configuration to the config file with restrictive permissions
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.configPath == "" {
		var err error
		c.configPath, err = GetConfigPath()
		if err != nil {
			return err
		}
	}

	// Create config without runtime fields
	toSave := &Config{
		DefaultProvider: c.DefaultProvider,
		DefaultModel:    c.DefaultModel,
		ExportPath:      c.ExportPath,
		EnableTools:     c.EnableTools,
		GitAutoCommit:   c.GitAutoCommit,
		APIKeys:         c.APIKeys,
	}

	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write with restrictive permissions (0600 = owner read/write only)
	if err := os.WriteFile(c.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetAPIKey returns the API key for the specified provider
// It checks environment variables first, then falls back to config
func (c *Config) GetAPIKey(provider string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	switch provider {
	case "openai":
		if key := os.Getenv(EnvOpenAIKey); key != "" {
			return key
		}
		return c.APIKeys.OpenAI
	case "anthropic":
		if key := os.Getenv(EnvAnthropicKey); key != "" {
			return key
		}
		return c.APIKeys.Anthropic
	case "groq":
		if key := os.Getenv(EnvGroqKey); key != "" {
			return key
		}
		return c.APIKeys.Groq
	case "openrouter":
		if key := os.Getenv(EnvOpenRouterKey); key != "" {
			return key
		}
		return c.APIKeys.OpenRouter
	default:
		return ""
	}
}

// SetAPIKey sets the API key for the specified provider
// Pass persist=true to save to config file, false to keep in memory only
func (c *Config) SetAPIKey(provider, key string, persist bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch provider {
	case "openai":
		c.APIKeys.OpenAI = key
	case "anthropic":
		c.APIKeys.Anthropic = key
	case "groq":
		c.APIKeys.Groq = key
	case "openrouter":
		c.APIKeys.OpenRouter = key
	default:
		return fmt.Errorf("unknown provider: %s", provider)
	}

	if persist {
		c.mu.Unlock()
		err := c.Save()
		c.mu.Lock()
		return err
	}
	return nil
}

// HasAPIKey checks if an API key is configured for the provider
func (c *Config) HasAPIKey(provider string) bool {
	return c.GetAPIKey(provider) != ""
}

// SetDefaultProvider updates the default provider
func (c *Config) SetDefaultProvider(provider string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DefaultProvider = provider
}

// SetDefaultModel updates the default model
func (c *Config) SetDefaultModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DefaultModel = model
}

// GetDefaultProvider returns the default provider
func (c *Config) GetDefaultProvider() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DefaultProvider
}

// GetDefaultModel returns the default model
func (c *Config) GetDefaultModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.DefaultModel
}

// MaskKey returns a masked version of an API key for display
// Shows first 4 and last 4 characters only
func MaskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
