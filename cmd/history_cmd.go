package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newHistoryCmd returns the `lore history` cobra subcommand.
func newHistoryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "history",
		Short: "Show recent Lore rollback snapshots",
		Long: `history lists recent local rollback snapshots stored under .lore/snapshots.
Older git-based lore-snapshot commits are also shown when present, for
compatibility with projects created by earlier Lore versions.`,
		SilenceUsage: true,
		RunE:         runHistory,
	}
}

func runHistory(_ *cobra.Command, _ []string) error {
	// Reuse the TUI helper; it produces a lipgloss-styled box that renders
	// correctly in a plain terminal as well.
	fmt.Println(snapshotHistory(10))
	return nil
}
