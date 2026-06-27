package cmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"

	"github.com/fayzkk889/lore/internal/agent"
	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/engine"
)

// testWithConfigDir overrides home so config reads/writes use a temp dir.
func testWithConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir) // Windows
}

// writeTestConfig writes a config with the given provider/key/model.
func writeTestConfig(t *testing.T, provider, apiKey, model string) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Engine = config.EngineConfig{
		Provider: provider,
		APIKey:   apiKey,
		Model:    model,
	}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func resetEngineFlags(t *testing.T) {
	t.Helper()
	oldFlagProvider := flagProvider
	oldFlagModel := flagModel
	oldFlagBaseURL := flagBaseURL
	oldFlagAPIKey := flagAPIKey
	flagProvider, flagModel, flagBaseURL, flagAPIKey = "", "", "", ""
	t.Cleanup(func() {
		flagProvider = oldFlagProvider
		flagModel = oldFlagModel
		flagBaseURL = oldFlagBaseURL
		flagAPIKey = oldFlagAPIKey
	})
}

func TestSlashModelShowsCurrent(t *testing.T) {
	m := chatModelForTest(t)
	m.engineName = "openrouter:deepseek/deepseek-chat-v3.1"

	next, cmd := m.handleSlash("/model")
	got := next.(chatModel)
	_ = got
	if cmd == nil {
		t.Fatal("/model returned nil command")
	}
}

func TestSlashModelRejectsWhileRunning(t *testing.T) {
	m := chatModelForTest(t)
	m.state = stateRunning

	next, _ := m.handleSlash("/model some-model")
	got := next.(chatModel)
	// engineName should be unchanged
	if got.engineName != m.engineName {
		t.Fatal("model changed while running")
	}
}

func TestSlashModelSwitchesProviderAndPreservesHistory(t *testing.T) {
	testWithConfigDir(t)
	// Need a real provider that doesn't need API key — use ollama with a model.
	writeTestConfig(t, "ollama", "", "llama3")

	// Clear flag state that would interfere with resolveEngineSettings.
	oldFlagProvider := flagProvider
	oldFlagModel := flagModel
	oldFlagBaseURL := flagBaseURL
	oldFlagAPIKey := flagAPIKey
	flagProvider, flagModel, flagBaseURL, flagAPIKey = "", "", "", ""
	t.Cleanup(func() {
		flagProvider = oldFlagProvider
		flagModel = oldFlagModel
		flagBaseURL = oldFlagBaseURL
		flagAPIKey = oldFlagAPIKey
	})

	m := chatModelForTest(t)
	// Set up the agent with a real provider.
	cfg, _ := config.LoadConfig()
	prov, err := resolveEngineForDir(cfg, m.projectDir)
	if err != nil {
		t.Fatal(err)
	}
	m.ag.Provider = prov
	m.engineName = prov.Name()

	// Add a fake history entry to prove it survives.
	m.ag.Provider = prov // just setting for the check
	oldHistory := []engine.Message{engine.UserText("hello")}
	// We can't directly set history (unexported), but we can verify the
	// agent pointer itself is NOT replaced — only its Provider field.
	agentBefore := m.ag

	next, _ := m.handleSlash("/model qwen3:4b")
	got := next.(chatModel)

	// Agent pointer must be the same (history lives on the Agent struct).
	if got.ag != agentBefore {
		t.Fatal("agent was replaced — conversation history would be lost")
	}

	// Engine name must reflect new model.
	if !strings.Contains(got.engineName, "qwen3:4b") {
		t.Fatalf("engineName = %q, want it to contain qwen3:4b", got.engineName)
	}

	// Project config must have the new model.
	projectCfg := readLoreProjectConfig(m.projectDir)
	if projectCfg.Model != "qwen3:4b" {
		t.Fatalf("project model = %q, want qwen3:4b", projectCfg.Model)
	}

	// Global config is the default and must not be overwritten by /model.
	reloaded, err := config.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Engine.Model != "llama3" {
		t.Fatalf("global model = %q, want llama3", reloaded.Engine.Model)
	}

	// Credentials must be untouched.
	if reloaded.Engine.Provider != "ollama" {
		t.Fatalf("provider = %q, want ollama", reloaded.Engine.Provider)
	}

	_ = oldHistory // silence unused — the important check is agent identity
}

func TestSlashModelDoesNotResetCredentials(t *testing.T) {
	testWithConfigDir(t)
	writeTestConfig(t, "ollama", "", "llama3")

	oldFlagProvider := flagProvider
	oldFlagModel := flagModel
	oldFlagBaseURL := flagBaseURL
	oldFlagAPIKey := flagAPIKey
	flagProvider, flagModel, flagBaseURL, flagAPIKey = "", "", "", ""
	t.Cleanup(func() {
		flagProvider = oldFlagProvider
		flagModel = oldFlagModel
		flagBaseURL = oldFlagBaseURL
		flagAPIKey = oldFlagAPIKey
	})

	m := chatModelForTest(t)
	cfg, _ := config.LoadConfig()
	prov, _ := resolveEngineForDir(cfg, m.projectDir)
	m.ag.Provider = prov
	m.engineName = prov.Name()

	m.handleSlash("/model mistral:7b")

	reloaded, _ := config.LoadConfig()
	if reloaded.Engine.Provider != "ollama" {
		t.Fatalf("provider = %q, want ollama", reloaded.Engine.Provider)
	}
}

func TestConfigModelCLISubcommand(t *testing.T) {
	testWithConfigDir(t)
	writeTestConfig(t, "ollama", "", "llama3")

	oldFlagProvider := flagProvider
	oldFlagModel := flagModel
	oldFlagBaseURL := flagBaseURL
	oldFlagAPIKey := flagAPIKey
	flagProvider, flagModel, flagBaseURL, flagAPIKey = "", "", "", ""
	t.Cleanup(func() {
		flagProvider = oldFlagProvider
		flagModel = oldFlagModel
		flagBaseURL = oldFlagBaseURL
		flagAPIKey = oldFlagAPIKey
	})

	// Exercise the config model subcommand by finding it on the command tree.
	cmd := newConfigCmd()
	cmd.SetArgs([]string{"model", "gemma2:9b"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config model set failed: %v", err)
	}

	reloaded, _ := config.LoadConfig()
	if reloaded.Engine.Model != "gemma2:9b" {
		t.Fatalf("model = %q, want gemma2:9b", reloaded.Engine.Model)
	}
	if reloaded.Engine.Provider != "ollama" {
		t.Fatalf("provider = %q, want ollama (credentials corrupted)", reloaded.Engine.Provider)
	}
}

func TestConfigModelCLIShowsCurrent(t *testing.T) {
	testWithConfigDir(t)
	writeTestConfig(t, "ollama", "", "llama3")

	oldFlagProvider := flagProvider
	oldFlagModel := flagModel
	oldFlagBaseURL := flagBaseURL
	oldFlagAPIKey := flagAPIKey
	flagProvider, flagModel, flagBaseURL, flagAPIKey = "", "", "", ""
	t.Cleanup(func() {
		flagProvider = oldFlagProvider
		flagModel = oldFlagModel
		flagBaseURL = oldFlagBaseURL
		flagAPIKey = oldFlagAPIKey
	})

	cmd := newConfigCmd()
	cmd.SetArgs([]string{"model"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config model show failed: %v", err)
	}
}

func TestProjectModelOverridesGlobalDefault(t *testing.T) {
	testWithConfigDir(t)
	writeTestConfig(t, "ollama", "", "llama3")
	resetEngineFlags(t)

	projectDir := t.TempDir()
	if err := setProjectModel(projectDir, mustLoadConfig(t), "qwen3:4b"); err != nil {
		t.Fatal(err)
	}

	s, ok := resolveEngineSettingsForDir(mustLoadConfig(t), projectDir)
	if !ok {
		t.Fatal("settings not resolved")
	}
	if s.model != "qwen3:4b" {
		t.Fatalf("model = %q, want qwen3:4b", s.model)
	}

	t.Setenv("LORE_MODEL", "env-model")
	s, ok = resolveEngineSettingsForDir(mustLoadConfig(t), projectDir)
	if !ok {
		t.Fatal("settings not resolved")
	}
	if s.model != "env-model" {
		t.Fatalf("env override model = %q, want env-model", s.model)
	}
}

func TestSlashSuggestionsCompleteCommand(t *testing.T) {
	m := chatModelForTest(t)
	m.ta.SetValue("/mo")
	m.refreshSlashSuggestions()
	if len(m.suggestions) == 0 {
		t.Fatal("expected suggestions")
	}
	m.applySlashSuggestion()
	if got := m.ta.Value(); !strings.HasPrefix(got, "/model") && !strings.HasPrefix(got, "/models") {
		t.Fatalf("completion = %q, want model command", got)
	}
}

func TestFetchProviderModelsUsesModelsEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"alpha/model","name":"Alpha"},{"id":"beta/model"}]}`))
	}))
	defer srv.Close()

	s := engineSettings{
		provider: providers["openrouter"],
		baseURL:  srv.URL + "/v1",
		apiKey:   "test-key",
		model:    "alpha/model",
	}
	models, err := fetchProviderModels(t.Context(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "alpha/model" || models[1].ID != "beta/model" {
		t.Fatalf("models = %#v", models)
	}
}

func TestSetupStillWorks(t *testing.T) {
	testWithConfigDir(t)

	// Verify that the existing setup flow writes a config that SaveModel
	// can then modify without breaking anything.
	cfg := config.DefaultConfig()
	cfg.Engine = config.EngineConfig{
		Provider: "openrouter",
		APIKey:   "sk-or-test",
		Model:    "deepseek/deepseek-chat-v3.1",
	}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Now switch model — simulates post-setup model change.
	if err := config.SaveModel("anthropic/claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}

	reloaded, _ := config.LoadConfig()
	if reloaded.Engine.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", reloaded.Engine.Provider)
	}
	if reloaded.Engine.APIKey != "sk-or-test" {
		t.Fatalf("api_key changed after model switch")
	}
	if reloaded.Engine.Model != "anthropic/claude-sonnet-4-6" {
		t.Fatalf("model = %q, want new model", reloaded.Engine.Model)
	}
}

func TestOldConfigFileStillLoads(t *testing.T) {
	testWithConfigDir(t)

	dir, _ := config.LoreDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Minimal old-style config — no model field at all.
	old := `[engine]
provider = "anthropic"
api_key = "sk-old-key"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("old config failed to load: %v", err)
	}
	if cfg.Engine.Provider != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", cfg.Engine.Provider)
	}
	if cfg.Engine.Model != "" {
		t.Fatalf("model = %q, want empty for old config", cfg.Engine.Model)
	}
}

func chatModelForTest(t *testing.T) chatModel {
	t.Helper()
	ta := textarea.New()
	ta.Focus()
	return chatModel{
		ta:         ta,
		ag:         &agent.Agent{},
		projectDir: t.TempDir(),
		permission: agent.PermissionFullAuto,
		engineName: "test:test-model",
	}
}
