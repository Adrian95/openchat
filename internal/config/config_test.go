package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DefaultProvider != "openai" {
		t.Errorf("expected default provider 'openai', got '%s'", cfg.DefaultProvider)
	}
	if cfg.DefaultModel != "gpt-4o" {
		t.Errorf("expected default model 'gpt-4o', got '%s'", cfg.DefaultModel)
	}
	if cfg.EnableTools {
		t.Error("expected EnableTools to be false by default")
	}
	if cfg.GitAutoCommit {
		t.Error("expected GitAutoCommit to be false by default")
	}
}

func TestLoadNonExistent(t *testing.T) {
	// Create a temporary directory for test
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Should return default config
	if cfg.DefaultProvider != "openai" {
		t.Errorf("expected default provider 'openai', got '%s'", cfg.DefaultProvider)
	}
}

func TestLoadExistingConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create config directory and file
	configDir := filepath.Join(tmpDir, DefaultConfigDir)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	testConfig := &Config{
		DefaultProvider: "anthropic",
		DefaultModel:    "claude-3-5-sonnet-20241022",
		ExportPath:      "/custom/export/path",
		EnableTools:     true,
		GitAutoCommit:   true,
	}

	data, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	configPath := filepath.Join(configDir, DefaultConfigFile)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", cfg.DefaultProvider)
	}
	if cfg.DefaultModel != "claude-3-5-sonnet-20241022" {
		t.Errorf("expected model 'claude-3-5-sonnet-20241022', got '%s'", cfg.DefaultModel)
	}
	if cfg.ExportPath != "/custom/export/path" {
		t.Errorf("expected export path '/custom/export/path', got '%s'", cfg.ExportPath)
	}
	if !cfg.EnableTools {
		t.Error("expected EnableTools to be true")
	}
	if !cfg.GitAutoCommit {
		t.Error("expected GitAutoCommit to be true")
	}
}

func TestEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Create config with API keys
	configDir := filepath.Join(tmpDir, DefaultConfigDir)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	testConfig := &Config{
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o",
		APIKeys: APIKeys{
			OpenAI:    "config-openai-key",
			Anthropic: "config-anthropic-key",
		},
	}

	data, err := json.Marshal(testConfig)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}

	configPath := filepath.Join(configDir, DefaultConfigFile)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Set environment variable to override
	oldEnvKey := os.Getenv(EnvOpenAIKey)
	os.Setenv(EnvOpenAIKey, "env-openai-key")
	defer os.Setenv(EnvOpenAIKey, oldEnvKey)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// OpenAI key should be from env
	if key := cfg.GetAPIKey("openai"); key != "env-openai-key" {
		t.Errorf("expected OpenAI key from env 'env-openai-key', got '%s'", key)
	}

	// Anthropic key should be from config (no env override)
	if key := cfg.GetAPIKey("anthropic"); key != "config-anthropic-key" {
		t.Errorf("expected Anthropic key from config 'config-anthropic-key', got '%s'", key)
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Modify config
	cfg.DefaultProvider = "anthropic"
	cfg.DefaultModel = "claude-3-opus-20240229"
	cfg.EnableTools = true

	// Save
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Load again and verify
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() failed after save: %v", err)
	}

	if cfg2.DefaultProvider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", cfg2.DefaultProvider)
	}
	if cfg2.DefaultModel != "claude-3-opus-20240229" {
		t.Errorf("expected model 'claude-3-opus-20240229', got '%s'", cfg2.DefaultModel)
	}
	if !cfg2.EnableTools {
		t.Error("expected EnableTools to be true")
	}
}

func TestSetAPIKey(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	// Clear any existing env vars
	oldEnvKey := os.Getenv(EnvOpenAIKey)
	os.Unsetenv(EnvOpenAIKey)
	defer os.Setenv(EnvOpenAIKey, oldEnvKey)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Set key in memory only
	if err := cfg.SetAPIKey("openai", "test-key", false); err != nil {
		t.Fatalf("SetAPIKey() failed: %v", err)
	}

	if key := cfg.GetAPIKey("openai"); key != "test-key" {
		t.Errorf("expected key 'test-key', got '%s'", key)
	}

	// Load fresh config - key should not be persisted
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if key := cfg2.GetAPIKey("openai"); key != "" {
		t.Errorf("expected empty key (not persisted), got '%s'", key)
	}

	// Now set with persist=true
	if err := cfg.SetAPIKey("anthropic", "persist-key", true); err != nil {
		t.Fatalf("SetAPIKey() with persist failed: %v", err)
	}

	// Load fresh config - key should be persisted
	cfg3, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if key := cfg3.GetAPIKey("anthropic"); key != "persist-key" {
		t.Errorf("expected key 'persist-key' (persisted), got '%s'", key)
	}
}

func TestHasAPIKey(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.HasAPIKey("openai") {
		t.Error("expected no OpenAI key")
	}

	cfg.APIKeys.OpenAI = "test-key"
	if !cfg.HasAPIKey("openai") {
		t.Error("expected OpenAI key to be present")
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"", "****"},
		{"short", "****"},
		{"12345678", "****"},
		{"sk-1234567890abcdef", "sk-1...cdef"},
		{"anthropic-key-very-long-string", "anth...ring"},
	}

	for _, tt := range tests {
		result := MaskKey(tt.key)
		if result != tt.expected {
			t.Errorf("MaskKey(%q) = %q, want %q", tt.key, result, tt.expected)
		}
	}
}

func TestUnknownProvider(t *testing.T) {
	cfg := DefaultConfig()

	// Should return empty string for unknown provider
	if key := cfg.GetAPIKey("unknown"); key != "" {
		t.Errorf("expected empty key for unknown provider, got '%s'", key)
	}

	// Should return error for unknown provider
	if err := cfg.SetAPIKey("unknown", "test", false); err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestConfigFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", oldHome)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Save config to create the file
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Check config directory permissions
	configDir := filepath.Join(tmpDir, DefaultConfigDir)
	dirInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("failed to stat config dir: %v", err)
	}

	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf("expected config dir permissions 0700, got %o", dirPerm)
	}

	// Check config file permissions
	configPath := filepath.Join(configDir, DefaultConfigFile)
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}

	filePerm := fileInfo.Mode().Perm()
	if filePerm != 0600 {
		t.Errorf("expected config file permissions 0600, got %o", filePerm)
	}
}
