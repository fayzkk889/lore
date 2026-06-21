package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/agent"
	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/display"
)

// newDoCmd returns `lore do "<prompt>"` — the headless agent runner. It
// executes one request to completion (including verification and the fix
// loop) and exits 0 only when the result is verified.
func newDoCmd() *cobra.Command {
	var dir string
	var permission string

	cmd := &cobra.Command{
		Use:   "do \"<request>\"",
		Short: "Run one agent request headlessly (no TUI) and exit",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			perm := configuredPermissionMode(cfg)
			explicitPermission := strings.TrimSpace(permission) != ""
			if explicitPermission {
				parsed, ok := parsePermissionMode(permission)
				if !ok {
					return fmt.Errorf("invalid permission mode %q (use full-auto, auto-safe, ask, or read-only)", permission)
				}
				perm = parsed
			}
			if perm == agent.PermissionAsk || perm == agent.PermissionAutoSafe {
				if explicitPermission {
					return fmt.Errorf("permission mode %q needs interactive approvals; use `lore` TUI, or run `lore do --permission full-auto|read-only`", perm)
				}
				fmt.Printf("permission mode %q needs interactive approvals; using full-auto for this headless run (override with --permission read-only)\n", perm)
				perm = agent.PermissionFullAuto
			}
			provider, err := resolveEngine(cfg)
			if err != nil {
				return err
			}

			workDir := dir
			if workDir == "" {
				workDir, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			workDir, err = filepath.Abs(workDir)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(workDir, 0o755); err != nil {
				return fmt.Errorf("creating project directory: %w", err)
			}
			if _, err := ensureLoreWiki(workDir); err != nil {
				return fmt.Errorf("initializing .lore wiki: %w", err)
			}

			fmt.Printf("lore %s — engine: %s\nproject: %s\n\n", Version, provider.Name(), workDir)
			fmt.Println("probing environment...")
			env := agent.ProbeEnv()
			fmt.Print(env.Describe(), "\n")

			ag := &agent.Agent{
				Provider:     provider,
				Dir:          workDir,
				Env:          env,
				ExtraContext: loadProjectContext(workDir),
				EventSink:    printEvent,
				RequireWork:  true, // headless runs are work requests; a workless stop gets one structural nudge
				Permission:   perm,
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			outcome := ag.Run(ctx, args[0])

			u := ag.Usage()
			fmt.Printf("\n\ntokens: %d in / %d out (cache: %d read, %d write)\n",
				u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens)

			if !outcome.OK {
				fmt.Println(display.ErrorStyle.Render("RESULT: NOT VERIFIED"))
				if outcome.Detail != "" {
					fmt.Println(outcome.Detail)
				}
				os.Exit(1)
			}
			fmt.Println(display.SuccessStyle.Render("RESULT: VERIFIED OK"))
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "project directory (default: current directory)")
	cmd.Flags().StringVar(&permission, "permission", "", "permission mode: full-auto, read-only (ask/auto-safe require the TUI)")
	return cmd
}

// printEvent renders agent events as plain streaming text for headless runs.
func printEvent(ev agent.Event) {
	switch ev.Kind {
	case "text":
		fmt.Print(ev.Text)
	case "tool_start":
		if ev.Detail != "" {
			fmt.Printf("\n[%s] %s\n", ev.Tool, ev.Detail)
		}
	case "tool_output":
		fmt.Println("  | " + ev.Text)
	case "tool_done":
		mark := "ok"
		if !ev.OK {
			mark = "FAIL"
		}
		fmt.Printf("  -> %s: %s\n", mark, ev.Detail)
	case "verify":
		fmt.Println("\n--- verification ---")
		fmt.Println(strings.TrimRight(ev.Detail, "\n"))
		fmt.Println("--------------------")
	case "retry":
		fmt.Printf("\n[engine busy — retrying, attempt %d]\n", ev.Attempt)
	case "info":
		fmt.Printf("\n[harness] %s\n", ev.Text)
	case "error":
		fmt.Printf("\nERROR: %v\n", ev.Err)
	}
}
