package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/agent"
	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/display"
)

// ── "lore config" command group ───────────────────────────────────────────────

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or change the configured AI provider",
		Long:  "Lore connects directly to an AI provider with your own API key.\nThis command shows the active provider/model and where the key comes from.",
		RunE:  runConfigShow,
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "set",
		Short: "Interactively choose the provider, API key, and model",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			return runFirstRunSetup(cfg)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "clear",
		Short: "Remove the stored provider configuration and API key",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			cfg.Engine = config.EngineConfig{}
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Println(display.DimStyle.Render("Provider configuration cleared."))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "permission [full-auto|auto-safe|ask|read-only]",
		Short: "Show or set the default permission mode",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			if len(args) == 0 {
				fmt.Println("permission mode: " + string(configuredPermissionMode(cfg)))
				return nil
			}
			mode, ok := parsePermissionMode(args[0])
			if !ok {
				return fmt.Errorf("invalid permission mode %q (use full-auto, auto-safe, ask, or read-only)", args[0])
			}
			cfg.Safety.PermissionMode = string(mode)
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}
			fmt.Println(display.DimStyle.Render("Default permission mode set to " + string(mode) + "."))
			if mode == agent.PermissionAsk || mode == agent.PermissionAutoSafe {
				fmt.Println(display.DimStyle.Render("Note: `lore do` cannot prompt; use the TUI or override with `--permission full-auto|read-only`."))
			}
			return nil
		},
	})

	return cmd
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	s, ok := resolveEngineSettings(cfg)
	if !ok {
		fmt.Println(display.DimStyle.Render("No provider configured."))
		fmt.Println(display.DimStyle.Render("Run `lore config set`, or set an API key env var (e.g. OPENAI_API_KEY, OPENROUTER_API_KEY, ANTHROPIC_API_KEY)."))
		return nil
	}

	label := s.provider.label
	if label == "" {
		label = s.provider.name
	}

	keyDesc := "not set"
	switch {
	case s.keySource == "none" && !s.provider.needsKey:
		keyDesc = "not needed"
	case s.keySource == "flag":
		keyDesc = redactKey(s.apiKey) + "  (from --api-key)"
	case strings.HasPrefix(s.keySource, "env:"):
		keyDesc = redactKey(s.apiKey) + "  (from " + strings.TrimPrefix(s.keySource, "env:") + ")"
	case s.keySource == "config":
		keyDesc = redactKey(s.apiKey) + "  (from ~/.lore/config.toml)"
	}

	var sb strings.Builder
	sb.WriteString(display.BoldStyle.Render("Engine") + "\n\n")
	sb.WriteString(fmt.Sprintf("  %s %s\n", display.DimStyle.Render("Provider:"), display.AccentStyle.Render(label)))
	sb.WriteString(fmt.Sprintf("  %s %s\n", display.DimStyle.Render("Model:   "), display.TealStyle.Render(emptyDash(s.model))))
	sb.WriteString(fmt.Sprintf("  %s %s\n", display.DimStyle.Render("Endpoint:"), emptyDash(s.baseURL)))
	sb.WriteString(fmt.Sprintf("  %s %s\n", display.DimStyle.Render("API key: "), keyDesc))
	sb.WriteString(fmt.Sprintf("  %s %s\n", display.DimStyle.Render("Safety: "), display.TealStyle.Render(string(configuredPermissionMode(cfg)))))
	fmt.Println(display.BoxStyle.Render(sb.String()))

	if _, err := buildProvider(s); err != nil {
		fmt.Println(display.ErrorStyle.Render("✗ " + err.Error()))
	}
	return nil
}

// redactKey shows just enough of a key to recognise it.
func redactKey(key string) string {
	if len(key) <= 8 {
		return "•••"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
