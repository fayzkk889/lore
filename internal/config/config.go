// Package config handles loading and saving Lore's TOML configuration
// from ~/.lore/config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// -----------------------------------------------------------------
// Config types
// -----------------------------------------------------------------

// Config is the top-level configuration structure.
type Config struct {
	Engine  EngineConfig  `toml:"engine"`
	Display DisplayConfig `toml:"display"`
	Safety  SafetyConfig  `toml:"safety"`
}

// EngineConfig selects which AI provider powers the agent. Lore connects
// directly to the provider with the user's own API key — there is no
// hosted middle layer.
type EngineConfig struct {
	// Provider is one of: "anthropic", "openai", "openrouter", "deepseek",
	// "together", "ollama", or "custom" (any OpenAI-compatible endpoint).
	Provider string `toml:"provider"`
	// BaseURL overrides the provider's default endpoint. Required for
	// "custom"; optional everywhere else (e.g. a self-hosted gateway).
	BaseURL string `toml:"base_url"`
	// Model is the model id to use (e.g. "claude-sonnet-4-6", "gpt-4o",
	// "deepseek-chat", "qwen3:4b").
	Model string `toml:"model"`
	// APIKey authenticates against the provider. Leave empty to source it
	// from the environment instead (ANTHROPIC_API_KEY, OPENAI_API_KEY,
	// OPENROUTER_API_KEY, ... or LORE_API_KEY). Not needed for Ollama.
	APIKey string `toml:"api_key"`
}

// DisplayConfig controls visual preferences.
type DisplayConfig struct {
	ShowTokenMeter bool `toml:"show_token_meter"`
}

// SafetyConfig controls local execution and write-safety defaults.
type SafetyConfig struct {
	// PermissionMode is one of: full-auto, auto-safe, ask, read-only.
	PermissionMode string `toml:"permission_mode"`
}

// -----------------------------------------------------------------
// Defaults
// -----------------------------------------------------------------

// DefaultConfig returns a Config pre-populated with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Display: DisplayConfig{
			ShowTokenMeter: true,
		},
		Safety: SafetyConfig{
			PermissionMode: "full-auto",
		},
	}
}

// -----------------------------------------------------------------
// Paths
// -----------------------------------------------------------------

// LoreDir returns the path to the ~/.lore global directory.
func LoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".lore"), nil
}

func configPath() (string, error) {
	dir, err := LoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// -----------------------------------------------------------------
// Load / Save
// -----------------------------------------------------------------

// LoadConfig loads ~/.lore/config.toml and returns the parsed Config.
// A missing file is not an error: defaults are returned and nothing is
// written (first-run setup decides what to persist).
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		return DefaultConfig(), nil
	}

	cfg := DefaultConfig()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decoding config (%s): %w", path, err)
	}
	return cfg, nil
}

// SaveConfig writes cfg to ~/.lore/config.toml, creating the directory
// if it does not already exist. The file is created with 0600 since it
// may contain an API key.
func SaveConfig(cfg *Config) error {
	dir, err := LoreDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	path, err := configPath()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	return nil
}

// SaveModel updates only the engine.model field in the existing config,
// preserving all other fields (provider, api_key, base_url, display, safety, etc.).
func SaveModel(model string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Engine.Model = model
	return SaveConfig(cfg)
}
