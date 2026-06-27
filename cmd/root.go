// Package cmd wires up the cobra command tree for the Lore CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/display"
)

// Version is set by GoReleaser via ldflags at build time.
var Version = "dev"

// SetVersion is called by main() to propagate the build-time version
// into the cmd package and into the cobra --version flag.
func SetVersion(v string) {
	Version = v
	rootCmd.Version = v
}

// rootCmd is the top-level cobra command.
// Running `lore` with no sub-command launches the interactive chat REPL.
var rootCmd = &cobra.Command{
	Use:     "lore",
	Short:   "Lore — open-source AI coding agent, bring your own key",
	Long:    "Lore is an open-source AI coding agent that connects directly to the\nmodel provider of your choice (Anthropic, OpenAI, OpenRouter, DeepSeek,\nlocal Ollama, or any OpenAI-compatible endpoint) using your own API key.",
	Version: "dev",
	// Default action: run the chat REPL.
	RunE: runChat,
	// Don't print usage on RunE errors — the error message is enough.
	// Execute prints the error itself, so silence cobra's duplicate.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// Persistent flags are inherited by all sub-commands.
	rootCmd.PersistentFlags().StringVar(
		&flagProvider, "provider", "",
		"AI provider: anthropic | openai | openrouter | deepseek | ollama | custom",
	)
	rootCmd.PersistentFlags().StringVar(
		&flagBaseURL, "base-url", "",
		"provider endpoint override (e.g. http://localhost:11434/v1)",
	)
	rootCmd.PersistentFlags().StringVar(
		&flagModel, "model", "",
		"model id (e.g. claude-sonnet-4-6, gpt-4o, deepseek-chat, qwen3:4b)",
	)
	rootCmd.PersistentFlags().StringVar(
		&flagAPIKey, "api-key", "",
		"provider API key (overrides env vars and config file)",
	)

	rootCmd.SetVersionTemplate(display.BannerPlain("") + "\nlore version {{.Version}}\n")

	// Register sub-commands.
	rootCmd.AddCommand(newConfigCmd())
	rootCmd.AddCommand(newModelCmd())
	rootCmd.AddCommand(newModelsCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newExportLearningCmd())
	rootCmd.AddCommand(newRollbackCmd())
	rootCmd.AddCommand(newHistoryCmd())
	rootCmd.AddCommand(newDoCmd())
}

// Execute is the package-level entry point called from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
