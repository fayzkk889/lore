package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/engine"
)

// Engine selection flags (registered on the root command).
var (
	flagProvider string // anthropic | openai | openrouter | deepseek | ollama | custom
	flagBaseURL  string
	flagModel    string
	flagAPIKey   string
)

// providerInfo describes one built-in provider preset.
type providerInfo struct {
	name         string // canonical provider id
	label        string // human-readable name for menus
	baseURL      string // default endpoint ("" = must be supplied)
	envKey       string // provider-specific API key env var
	defaultModel string // model used when none is configured
	needsKey     bool   // false for local endpoints (Ollama)
	anthropic    bool   // true = native Anthropic API, false = OpenAI-compatible
}

// providers is the table of built-in presets. Anything else OpenAI-compatible
// (Together, Groq, Mistral, a self-hosted gateway, ...) works via "custom".
var providers = map[string]providerInfo{
	"anthropic": {
		name: "anthropic", label: "Anthropic",
		baseURL: engine.DefaultAnthropicURL, envKey: "ANTHROPIC_API_KEY",
		defaultModel: "claude-sonnet-4-6", needsKey: true, anthropic: true,
	},
	"openai": {
		name: "openai", label: "OpenAI",
		baseURL: "https://api.openai.com/v1", envKey: "OPENAI_API_KEY",
		defaultModel: "gpt-4o", needsKey: true,
	},
	"openrouter": {
		name: "openrouter", label: "OpenRouter",
		baseURL: "https://openrouter.ai/api/v1", envKey: "OPENROUTER_API_KEY",
		defaultModel: "deepseek/deepseek-chat-v3.1", needsKey: true,
	},
	"deepseek": {
		name: "deepseek", label: "DeepSeek",
		baseURL: "https://api.deepseek.com/v1", envKey: "DEEPSEEK_API_KEY",
		defaultModel: "deepseek-chat", needsKey: true,
	},
	"ollama": {
		name: "ollama", label: "Ollama (local)",
		baseURL: "http://localhost:11434/v1", envKey: "",
		defaultModel: "", needsKey: false,
	},
	"custom": {
		name: "custom", label: "Custom OpenAI-compatible endpoint",
		baseURL: "", envKey: "",
		defaultModel: "", needsKey: false, // key optional: many gateways need one, local ones don't
	},
}

// keyEnvOrder is the order in which provider key env vars are checked when
// no provider has been chosen yet (so `OPENROUTER_API_KEY=... lore` just works).
var keyEnvOrder = []string{"anthropic", "openai", "openrouter", "deepseek"}

// engineSettings is the fully-resolved engine selection.
type engineSettings struct {
	provider providerInfo
	baseURL  string
	model    string
	apiKey   string
	// keySource records where the key came from, for `lore config` display.
	keySource string // "flag" | "env:NAME" | "config" | "none"
}

// resolveEngineSettings applies the precedence flag > env > config file and
// fills provider defaults. It does NOT prompt; callers decide what to do when
// nothing is configured (returned bool is false).
func resolveEngineSettings(cfg *config.Config) (engineSettings, bool) {
	var s engineSettings

	name := cfg.Engine.Provider
	if v := os.Getenv("LORE_PROVIDER"); v != "" {
		name = v
	}
	if flagProvider != "" {
		name = flagProvider
	}

	// No provider chosen anywhere: infer from which key env var is set.
	if name == "" {
		for _, p := range keyEnvOrder {
			if os.Getenv(providers[p].envKey) != "" {
				name = p
				break
			}
		}
	}
	if name == "" {
		return s, false
	}

	info, ok := providers[strings.ToLower(name)]
	if !ok {
		// Leave validation errors to buildProvider so the message lists options.
		info = providerInfo{name: strings.ToLower(name)}
	}
	s.provider = info

	s.baseURL = info.baseURL
	if cfg.Engine.BaseURL != "" {
		s.baseURL = cfg.Engine.BaseURL
	}
	if v := os.Getenv("LORE_BASE_URL"); v != "" {
		s.baseURL = v
	}
	if flagBaseURL != "" {
		s.baseURL = flagBaseURL
	}

	s.model = info.defaultModel
	if cfg.Engine.Model != "" {
		s.model = cfg.Engine.Model
	}
	if v := os.Getenv("LORE_MODEL"); v != "" {
		s.model = v
	}
	if flagModel != "" {
		s.model = flagModel
	}

	s.apiKey, s.keySource = resolveAPIKey(cfg, info)
	return s, true
}

// resolveAPIKey applies the key precedence: flag > provider env var >
// LORE_API_KEY > config file.
func resolveAPIKey(cfg *config.Config, info providerInfo) (key, source string) {
	if flagAPIKey != "" {
		return flagAPIKey, "flag"
	}
	if info.envKey != "" {
		if v := os.Getenv(info.envKey); v != "" {
			return v, "env:" + info.envKey
		}
	}
	if v := os.Getenv("LORE_API_KEY"); v != "" {
		return v, "env:LORE_API_KEY"
	}
	if cfg.Engine.APIKey != "" {
		return cfg.Engine.APIKey, "config"
	}
	return "", "none"
}

// providerNames returns the canonical provider ids, sorted.
func providerNames() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// buildProvider validates resolved settings and constructs the Provider.
func buildProvider(s engineSettings) (engine.Provider, error) {
	if _, ok := providers[s.provider.name]; !ok {
		return nil, fmt.Errorf("unknown provider %q (use one of: %s)", s.provider.name, strings.Join(providerNames(), ", "))
	}
	if s.baseURL == "" {
		return nil, fmt.Errorf("provider %q needs a base URL — set --base-url, LORE_BASE_URL, or [engine] base_url in ~/.lore/config.toml", s.provider.name)
	}
	if s.model == "" {
		return nil, fmt.Errorf("provider %q needs a model id — set --model, LORE_MODEL, or [engine] model in ~/.lore/config.toml", s.provider.name)
	}
	if s.apiKey == "" && s.provider.needsKey {
		hint := "LORE_API_KEY"
		if s.provider.envKey != "" {
			hint = s.provider.envKey
		}
		return nil, fmt.Errorf("no API key for %s — set %s, pass --api-key, or run `lore config set`", s.provider.label, hint)
	}

	if s.provider.anthropic {
		return engine.NewAnthropicProvider(s.baseURL, s.apiKey, s.model), nil
	}
	return engine.NewCompatProvider(s.provider.name, s.baseURL, s.apiKey, s.model), nil
}

// resolveEngine returns the configured Provider, running first-run setup if
// nothing is configured and stdin is interactive.
func resolveEngine(cfg *config.Config) (engine.Provider, error) {
	s, ok := resolveEngineSettings(cfg)
	if !ok {
		if !stdinIsTerminal() {
			return nil, fmt.Errorf("no AI provider configured — set an API key env var (e.g. OPENAI_API_KEY, OPENROUTER_API_KEY, ANTHROPIC_API_KEY) or run `lore config set`")
		}
		if err := runFirstRunSetup(cfg); err != nil {
			return nil, err
		}
		s, ok = resolveEngineSettings(cfg)
		if !ok {
			return nil, fmt.Errorf("setup did not produce a usable provider configuration")
		}
	}
	return buildProvider(s)
}
