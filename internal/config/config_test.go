package config

import (
	"os"
	"path/filepath"
	"testing"
)

// testWithConfigDir overrides the home directory so LoadConfig/SaveConfig
// use a temp dir, avoiding writes to the real ~/.lore/config.toml.
func testWithConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows
}

func TestSaveModelPreservesCredentials(t *testing.T) {
	testWithConfigDir(t)

	// Set up a config with provider + API key + model.
	cfg := DefaultConfig()
	cfg.Engine = EngineConfig{
		Provider: "openrouter",
		APIKey:   "sk-or-v1-test-key-12345",
		BaseURL:  "https://openrouter.ai/api/v1",
		Model:    "deepseek/deepseek-chat-v3.1",
	}
	cfg.Safety.PermissionMode = "ask"
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Switch the model.
	if err := SaveModel("anthropic/claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}

	// Reload and verify credentials are untouched.
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Engine.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", loaded.Engine.Provider)
	}
	if loaded.Engine.APIKey != "sk-or-v1-test-key-12345" {
		t.Fatalf("api_key = %q, want original key", loaded.Engine.APIKey)
	}
	if loaded.Engine.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("base_url = %q, want original", loaded.Engine.BaseURL)
	}
	if loaded.Engine.Model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("model = %q, want anthropic/claude-sonnet-4-6", loaded.Engine.Model)
	}
	if loaded.Safety.PermissionMode != "ask" {
		t.Fatalf("safety.permission_mode = %q, want ask", loaded.Safety.PermissionMode)
	}
}

func TestSaveModelPersistsAcrossLoad(t *testing.T) {
	testWithConfigDir(t)

	cfg := DefaultConfig()
	cfg.Engine = EngineConfig{
		Provider: "openai",
		APIKey:   "sk-test",
		Model:    "gpt-4o",
	}
	if err := SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	if err := SaveModel("gpt-4o-mini"); err != nil {
		t.Fatal(err)
	}

	// Load in a fresh call — simulates a new session.
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Engine.Model != "gpt-4o-mini" {
		t.Fatalf("model = %q, want gpt-4o-mini", loaded.Engine.Model)
	}
}

func TestOldConfigWithoutModelLoadsCorrectly(t *testing.T) {
	testWithConfigDir(t)

	// Write a minimal TOML that has no model field — simulates old config.
	dir, _ := LoreDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	tomlContent := `[engine]
provider = "anthropic"
api_key = "sk-ant-old"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine.Provider != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", cfg.Engine.Provider)
	}
	if cfg.Engine.APIKey != "sk-ant-old" {
		t.Fatalf("api_key = %q, want sk-ant-old", cfg.Engine.APIKey)
	}
	if cfg.Engine.Model != "" {
		t.Fatalf("model = %q, want empty (old config)", cfg.Engine.Model)
	}
}

func TestSaveModelCreatesConfigDirIfMissing(t *testing.T) {
	testWithConfigDir(t)

	// SaveModel on a completely fresh home dir (no ~/.lore yet).
	if err := SaveModel("test-model"); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Engine.Model != "test-model" {
		t.Fatalf("model = %q, want test-model", cfg.Engine.Model)
	}
}
