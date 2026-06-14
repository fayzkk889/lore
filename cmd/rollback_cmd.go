package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/display"
	"github.com/fayzkk889/lore/internal/snapshot"
)

var flagRollbackLast bool

// newRollbackCmd returns the `lore rollback` cobra subcommand.
func newRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Restore the project to a previous Lore snapshot",
		Long: `rollback lists recent local snapshots created by Lore before applying
changes, then restores tracked text files to the selected point. Files that
Lore deliberately does not snapshot, such as secrets, binaries, and large
artifacts, are preserved.

Older git-based lore-snapshot commits can still be restored for compatibility;
those legacy restores use git reset --hard and discard uncommitted changes.

Examples:
  lore rollback          # interactive: list snapshots, pick by number
  lore rollback --last   # immediately restore the most recent snapshot`,
		SilenceUsage: true,
		RunE:         runRollback,
	}
	cmd.Flags().BoolVar(&flagRollbackLast, "last", false, "Restore the most recent snapshot without prompting")
	return cmd
}

func runRollback(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	snapshots, err := snapshot.ListSnapshots(cwd, 10)
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		fmt.Println(display.DimStyle.Render("No snapshots available. Snapshots are created each time you apply changes inside Lore."))
		return nil
	}

	// ── --last: restore immediately ───────────────────────────────────────────
	if flagRollbackLast {
		s := snapshots[0]
		return doRestore(cwd, s)
	}

	// ── Interactive: list and prompt ──────────────────────────────────────────
	printSnapshotTable(snapshots)

	fmt.Print("\nEnter snapshot number to restore (or q to cancel): ")
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return nil
	}
	input := strings.TrimSpace(scanner.Text())
	if input == "" || input == "q" {
		fmt.Println(display.DimStyle.Render("Cancelled."))
		return nil
	}

	idx, convErr := strconv.Atoi(input)
	if convErr != nil || idx < 1 || idx > len(snapshots) {
		return fmt.Errorf("invalid selection %q — enter a number between 1 and %d", input, len(snapshots))
	}

	return doRestore(cwd, snapshots[idx-1])
}

// doRestore restores the given Lore snapshot and prints confirmation.
func doRestore(cwd string, s snapshot.Snapshot) error {
	fmt.Printf("Restoring to %s (%s)…\n",
		display.AccentStyle.Render(s.ShortHash),
		display.DimStyle.Render(s.Timestamp.Format("2006-01-02 15:04:05")),
	)
	if err := snapshot.RestoreSnapshot(cwd, s.Hash); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	fmt.Println(display.SuccessStyle.Render("✓ Rolled back to: " + s.Message))
	return nil
}

// printSnapshotTable renders a numbered list of snapshots to stdout.
func printSnapshotTable(snapshots []snapshot.Snapshot) {
	hashStyle := lipgloss.NewStyle().Foreground(display.ColorYellow)
	var sb strings.Builder
	sb.WriteString(display.BoldStyle.Render("Lore Snapshots") + "\n\n")
	for i, s := range snapshots {
		sb.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
			display.AccentStyle.Render(fmt.Sprintf("%2d.", i+1)),
			display.DimStyle.Render(s.Timestamp.Format("2006-01-02 15:04:05")),
			hashStyle.Render(s.ShortHash),
			s.Message,
		))
	}
	fmt.Println(display.BoxStyle.Render(sb.String()))
}
