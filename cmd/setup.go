package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/display"
)

// stdinIsTerminal reports whether stdin is an interactive terminal.
func stdinIsTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

// setupOrder fixes the menu order for first-run setup.
var setupOrder = []string{"anthropic", "openai", "openrouter", "deepseek", "ollama", "custom"}

// runFirstRunSetup interactively asks for the provider and API key (and, for
// endpoints without a sensible default, the model), then persists the result
// to ~/.lore/config.toml. It asks for nothing else — no account, no email.
func runFirstRunSetup(cfg *config.Config) error {
	fmt.Print(display.Banner(Version))
	fmt.Println()
	fmt.Println(display.BoldStyle.Render("First-run setup") + display.DimStyle.Render(" — Lore connects directly to an AI provider with your own key."))
	fmt.Println(display.DimStyle.Render("Saved to ~/.lore/config.toml. Prefer env vars? Ctrl+C and set e.g. OPENAI_API_KEY instead."))
	fmt.Println()

	for i, name := range setupOrder {
		p := providers[name]
		note := ""
		if p.envKey != "" {
			note = display.DimStyle.Render("  (key env var: " + p.envKey + ")")
		} else if name == "ollama" {
			note = display.DimStyle.Render("  (no key needed)")
		}
		fmt.Printf("  %d) %-36s%s\n", i+1, p.label, note)
	}
	fmt.Println()

	in := bufio.NewReader(os.Stdin)

	idx := 0
	for {
		choice, err := prompt(in, fmt.Sprintf("Provider [1-%d]: ", len(setupOrder)))
		if err != nil {
			return fmt.Errorf("setup aborted: %w", err)
		}
		n := 0
		if _, err := fmt.Sscanf(strings.TrimSpace(choice), "%d", &n); err == nil && n >= 1 && n <= len(setupOrder) {
			idx = n - 1
			break
		}
		fmt.Println(display.DimStyle.Render("  enter a number between 1 and " + fmt.Sprint(len(setupOrder))))
	}
	p := providers[setupOrder[idx]]

	cfg.Engine = config.EngineConfig{Provider: p.name}

	// Base URL: only asked when the preset has none (custom) or for Ollama
	// (confirm the local endpoint).
	switch p.name {
	case "custom":
		for cfg.Engine.BaseURL == "" {
			url, err := prompt(in, "Endpoint base URL (e.g. https://api.example.com/v1): ")
			if err != nil {
				return fmt.Errorf("setup aborted: %w", err)
			}
			cfg.Engine.BaseURL = strings.TrimSpace(url)
		}
	case "ollama":
		url, err := prompt(in, fmt.Sprintf("Ollama endpoint [%s]: ", p.baseURL))
		if err != nil {
			return fmt.Errorf("setup aborted: %w", err)
		}
		url = strings.TrimSpace(url)
		if url != "" && url != p.baseURL {
			cfg.Engine.BaseURL = url
		}
	}

	// API key.
	if p.needsKey || p.name == "custom" {
		label := "API key"
		if p.name == "custom" {
			label = "API key (leave empty if the endpoint needs none)"
		}
		for {
			key, err := readSecret(in, label+": ")
			if err != nil {
				return fmt.Errorf("setup aborted: %w", err)
			}
			key = strings.TrimSpace(key)
			if key == "" && p.name == "custom" {
				break
			}
			if key != "" {
				cfg.Engine.APIKey = key
				break
			}
		}
	}

	// Model: presets with a default just confirm it; otherwise ask.
	if p.defaultModel != "" {
		model, err := prompt(in, fmt.Sprintf("Model [%s]: ", p.defaultModel))
		if err != nil {
			return fmt.Errorf("setup aborted: %w", err)
		}
		if model = strings.TrimSpace(model); model != "" {
			cfg.Engine.Model = model
		}
	} else {
		for cfg.Engine.Model == "" {
			model, err := prompt(in, "Model id (e.g. qwen3:4b): ")
			if err != nil {
				return fmt.Errorf("setup aborted: %w", err)
			}
			cfg.Engine.Model = strings.TrimSpace(model)
		}
	}

	if err := config.SaveConfig(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println()
	model := cfg.Engine.Model
	if model == "" {
		model = p.defaultModel
	}
	fmt.Println(display.SuccessStyle.Render("✓ Configured: ") + p.label + display.DimStyle.Render(" · "+model))
	fmt.Println()
	return nil
}

// prompt prints label and reads one line from in. A read error (EOF on a
// closed stdin) is returned so callers can abort instead of spinning.
func prompt(in *bufio.Reader, label string) (string, error) {
	fmt.Print(label)
	line, err := in.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readSecret reads a line without echoing it when stdin is a real terminal;
// otherwise it falls back to a plain read (pipes, tests).
func readSecret(in *bufio.Reader, label string) (string, error) {
	if term.IsTerminal(os.Stdin.Fd()) {
		fmt.Print(label)
		data, err := term.ReadPassword(os.Stdin.Fd())
		fmt.Println()
		if err == nil {
			return string(data), nil
		}
		// Fall back to a plain read below.
	}
	return prompt(in, label)
}
