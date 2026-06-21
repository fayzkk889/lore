// Lore's interactive TUI: a Bubble Tea front-end over the tool-calling
// agent harness. The agent runs in its own goroutine and reports progress
// through a channel of events; the UI renders a transcript (streamed prose,
// in-place tool status lines, bordered diffs, verification reports) plus an
// in-place status line and token/cost footer that never touch scrollback.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/fayzkk889/lore/internal/agent"
	"github.com/fayzkk889/lore/internal/config"
	"github.com/fayzkk889/lore/internal/display"
	"github.com/fayzkk889/lore/internal/engine"
	"github.com/fayzkk889/lore/internal/lorefs"
	"github.com/fayzkk889/lore/internal/shell"
	"github.com/fayzkk889/lore/internal/snapshot"
)

// ────────────────────────────────────────────────────────────────────────────
// Messages
// ────────────────────────────────────────────────────────────────────────────

type agentEvMsg struct{ ev agent.Event }
type agentDoneMsg struct{ outcome agent.Outcome }
type approveReqMsg struct{ req *approveRequest }
type shellDoneMsg struct {
	command string
	output  string
	exit    int
}

type approveRequest struct {
	path     string
	command  string
	action   string
	detail   string
	oldC     string
	newC     string
	response chan agent.ApprovalDecision
}

type uiApprover struct{ events chan tea.Msg }

func (a *uiApprover) ApproveWrite(path, oldContent, newContent string) agent.ApprovalDecision {
	req := &approveRequest{path: path, oldC: oldContent, newC: newContent, response: make(chan agent.ApprovalDecision, 1)}
	a.events <- approveReqMsg{req: req}
	return <-req.response
}

func (a *uiApprover) ApproveCommand(command string) agent.ApprovalDecision {
	req := &approveRequest{command: command, response: make(chan agent.ApprovalDecision, 1)}
	a.events <- approveReqMsg{req: req}
	return <-req.response
}

func (a *uiApprover) ApproveAction(action, detail string) agent.ApprovalDecision {
	req := &approveRequest{action: action, detail: detail, response: make(chan agent.ApprovalDecision, 1)}
	a.events <- approveReqMsg{req: req}
	return <-req.response
}

// ────────────────────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────────────────────

type uiState int

const (
	stateIdle uiState = iota
	stateRunning
)

type chatModel struct {
	ta   textarea.Model
	sp   spinner.Model
	w, h int

	state    uiState
	runStart time.Time

	ag        *agent.Agent
	events    chan tea.Msg
	cancelRun context.CancelFunc
	approver  *uiApprover

	// status line context
	curTool   string
	curDetail string
	lastLine  string // most recent shell output line
	note      string // transient note (retry, etc.)

	activeProse string // currently streaming text
	lastProse   string // for /copy

	showBusyWarning bool // transient warning if user types while busy

	approval    *approveRequest
	approveMode bool
	permission  agent.PermissionMode

	usage       engine.Usage
	meter       display.TokenMeter
	prevUsage   engine.Usage
	engineName  string
	projectDir  string
	sessionTime time.Time

	quitting bool
}

// ────────────────────────────────────────────────────────────────────────────
// Entry point
// ────────────────────────────────────────────────────────────────────────────

func runChat(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	if _, err := ensureLoreWiki(cwd); err != nil {
		return fmt.Errorf("initializing .lore wiki: %w", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	provider, err := resolveEngine(cfg)
	if err != nil {
		return err
	}

	env := agent.ProbeEnv()
	events := make(chan tea.Msg, 512)

	ag := &agent.Agent{
		Provider:     provider,
		Dir:          cwd,
		Env:          env,
		ExtraContext: loadProjectContext(cwd),
		EventSink:    func(ev agent.Event) { events <- agentEvMsg{ev: ev} },
	}

	sp := spinner.New()
	sp.Spinner = spinner.MiniDot
	sp.Style = lipgloss.NewStyle().Foreground(display.ColorCyan)

	ta := textarea.New()
	ta.Placeholder = "Ask Lore anything…  (/help for commands)"
	ta.ShowLineNumbers = false
	ta.SetHeight(1)
	ta.MaxHeight = 6
	ta.CharLimit = 0
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.Focus()

	m := chatModel{
		ta:          ta,
		sp:          sp,
		ag:          ag,
		events:      events,
		approver:    &uiApprover{events: events},
		permission:  configuredPermissionMode(cfg),
		engineName:  provider.Name(),
		projectDir:  cwd,
		sessionTime: time.Now(),
	}

	// ANSI wipe for a "Clean Start"
	fmt.Print("\033[2J\033[H")

	p := tea.NewProgram(m, tea.WithInputTTY())
	final, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := final.(chatModel); ok && fm.meter.TotalSessionTokens() > 0 {
		fmt.Println("\n" + display.BoldStyle.Render("Session summary"))
		fmt.Println(fm.meter.SessionSummary(time.Since(fm.sessionTime)))
	}
	return nil
}

func loadProjectContext(cwd string) string {
	var b strings.Builder
	const maxContextBytes = 96 * 1024
	for _, name := range []string{"LORE.md", "lore.md"} {
		if data, err := os.ReadFile(filepath.Join(cwd, name)); err == nil {
			b.WriteString("### Project instructions (LORE.md)\n")
			b.WriteString(trimLen(string(data), 24*1024))
			b.WriteString("\n")
			break
		}
	}
	for _, doc := range loreMarkdownDocs(cwd, 16*1024) {
		if b.Len() >= maxContextBytes {
			break
		}
		fmt.Fprintf(&b, "### Project wiki: %s\n", doc.Rel)
		b.WriteString(trimLen(doc.Body, max(0, maxContextBytes-b.Len())))
		b.WriteString("\n")
	}
	return b.String()
}

// ────────────────────────────────────────────────────────────────────────────
// Init / Update
// ────────────────────────────────────────────────────────────────────────────

func projectMemory(cwd string) string {
	data, err := os.ReadFile(filepath.Join(cwd, ".lore", "memory.md"))
	if err != nil {
		return "No project memory yet. Use `lore init`, then `/remember <note>`."
	}
	s := strings.TrimSpace(string(data))
	if s == "" {
		return "Project memory is empty."
	}
	return s
}

func appendProjectMemory(cwd, note string) error {
	dir := filepath.Join(cwd, ".lore")
	if err := lorefs.MkdirPrivate(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, "memory.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := lorefs.WritePrivate(path, []byte(memoryFileHeader)); err != nil {
			return err
		}
	}
	_ = os.Chmod(path, lorefs.PrivateFileMode)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, lorefs.PrivateFileMode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n- %s\n", strings.TrimSpace(note))
	return err
}

const memoryFileHeader = "# Project Memory\n\nPersistent notes Lore should remember across sessions.\n"

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		tea.Printf("%s\n%s\n\n", display.Banner(Version), display.DimStyle.Render(fmt.Sprintf(" ◆ Lore %s · %s · engine: %s", Version, filepath.Base(m.projectDir), m.engineName))),
		func() tea.Msg { return tea.EnableBracketedPaste() },
		m.sp.Tick,
		m.waitEvent(),
	)
}

func (m chatModel) waitEvent() tea.Cmd {
	ch := m.events
	return func() tea.Msg { return <-ch }
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.ta.SetWidth(max(20, m.w-6))
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd

	case agentEvMsg:
		return m.handleAgentEvent(msg.ev)

	case agentDoneMsg:
		m.state = stateIdle
		m.curTool, m.curDetail, m.lastLine, m.note = "", "", "", ""
		m.showBusyWarning = false
		m.cancelRun = nil
		var cmds []tea.Cmd
		if !msg.outcome.OK && msg.outcome.Detail != "" && msg.outcome.Detail != "cancelled" {
			cmds = append(cmds, tea.Printf("\n%s\n", display.ErrorStyle.Render("✗ "+msg.outcome.Detail)))
		}
		if msg.outcome.Detail == "cancelled" {
			cmds = append(cmds, tea.Printf("%s\n", display.DimStyle.Render("· cancelled")))
		}
		cmds = append(cmds, m.waitEvent())
		return m, tea.Batch(cmds...)

	case approveReqMsg:
		m.approval = msg.req
		if msg.req.path != "" {
			return m, tea.Batch(
				tea.Printf("\n%s\n", renderFileDiff(msg.req.path, msg.req.oldC, msg.req.newC, m.w)),
				m.waitEvent(),
			)
		}
		if msg.req.action != "" {
			return m, tea.Batch(
				tea.Printf("\n%s\n", display.DimStyle.Render(msg.req.action+": "+msg.req.detail)),
				m.waitEvent(),
			)
		}
		return m, tea.Batch(
			tea.Printf("\n%s\n", display.DimStyle.Render("$ "+msg.req.command)),
			m.waitEvent(),
		)

	case shellDoneMsg:
		body := display.DimStyle.Render("$ "+msg.command) + "\n" + msg.output
		if msg.exit != 0 {
			body += display.ErrorStyle.Render(fmt.Sprintf("\nexit code %d", msg.exit))
		}
		return m, tea.Printf("%s\n", body)

	case tea.KeyMsg:
		m.showBusyWarning = false
		return m.handleKey(msg)
	}

	return m, nil
}

// ────────────────────────────────────────────────────────────────────────────
// Key handling
// ────────────────────────────────────────────────────────────────────────────

func (m chatModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.approval != nil {
		switch msg.String() {
		case "a", "enter":
			m.approval.response <- agent.ApproveOnce
			m.approval = nil
		case "A":
			m.approval.response <- agent.ApproveAlways
			m.approval = nil
		case "n", "N", "esc":
			m.approval.response <- agent.ApproveDeny
			m.approval = nil
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.state == stateRunning && m.cancelRun != nil {
			m.cancelRun()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEsc:
		if m.state == stateRunning && m.cancelRun != nil {
			m.cancelRun()
		}
		return m, nil

	case tea.KeyCtrlL:
		m.ag.Reset()
		return m, tea.ClearScreen

	case tea.KeyCtrlV:
		if text, err := clipboard.ReadAll(); err == nil && text != "" {
			text = strings.ReplaceAll(text, "\r\n", "\n")
			m.ta.InsertString(text)
			m.syncTaHeight()
		}
		return m, nil

	case tea.KeyCtrlD:
		return m.submit()

	case tea.KeyEnter, tea.KeyCtrlJ:
		if msg.Paste || msg.Alt {
			m.ta.InsertString("\n")
			m.syncTaHeight()
			return m, nil
		}
		return m.submit()
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.syncTaHeight()
	return m, cmd
}

func (m *chatModel) syncTaHeight() {
	lines := strings.Count(m.ta.Value(), "\n") + 1
	m.ta.SetHeight(max(1, min(6, lines)))
}

// ────────────────────────────────────────────────────────────────────────────
// Submitting input
// ────────────────────────────────────────────────────────────────────────────

func (m chatModel) submit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.ta.Value())
	if input == "" {
		return m, nil
	}

	if strings.HasPrefix(input, "/") {
		m.ta.Reset()
		m.ta.SetHeight(1)
		return m.handleSlash(input)
	}

	if m.state == stateRunning {
		m.showBusyWarning = true
		return m, nil
	}

	// Safe to submit: clear input box
	m.ta.Reset()
	m.ta.SetHeight(1)

	var cmds []tea.Cmd
	cmds = append(cmds, tea.Printf("\n%s%s\n", display.PromptStyle.Render("❯ "), display.BoldStyle.Render(input)))

	if hash, warn, _ := snapshot.CreateSnapshot(m.projectDir); hash != "" {
		cmds = append(cmds, tea.Printf("%s\n", display.DimStyle.Render("· snapshot "+hash+" saved (undo with /rollback)")))
	} else if warn != "" {
		cmds = append(cmds, tea.Printf("%s\n", display.DimStyle.Render("· "+warn)))
	}

	m.ag.Permission = m.permission
	if m.permission == agent.PermissionAsk || m.permission == agent.PermissionAutoSafe || m.approveMode {
		m.ag.Approver = m.approver
	} else {
		m.ag.Approver = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancelRun = cancel
	m.state = stateRunning
	m.runStart = time.Now()

	ag, events, in := m.ag, m.events, input
	go func() {
		outcome := ag.Run(ctx, in)
		events <- agentDoneMsg{outcome: outcome}
	}()

	return m, tea.Batch(cmds...)
}

// ────────────────────────────────────────────────────────────────────────────
// Agent events → transcript
// ────────────────────────────────────────────────────────────────────────────

func (m chatModel) handleAgentEvent(ev agent.Event) (chatModel, tea.Cmd) {
	var cmd tea.Cmd

	switch ev.Kind {

	case "text":
		m.lastProse += ev.Text
		m.activeProse += ev.Text
		m.curTool, m.curDetail = "", ""

	case "turn":
		if m.activeProse != "" {
			cmd = tea.Println(renderMarkdown(m.activeProse, m.w))
			m.activeProse = ""
		}

	case "tool_start":
		m.curTool, m.curDetail, m.lastLine = ev.Tool, ev.Detail, ""

	case "tool_output":
		m.lastLine = ev.Text

	case "tool_done":
		mark := display.SuccessStyle.Render("✓")
		if !ev.OK {
			mark = display.ErrorStyle.Render("✗")
		}
		detail := ev.Detail
		if detail != "" {
			detail = " " + display.DimStyle.Render(detail)
		}
		cmd = tea.Printf("  %s %s%s\n", mark, toolNameStyle.Render(ev.Tool), detail)
		m.curTool, m.curDetail, m.lastLine = "", "", ""

	case "file_diff":
		if !m.approveMode {
			cmd = tea.Printf("\n%s\n", renderFileDiff(ev.Path, ev.OldContent, ev.NewContent, m.w))
		}

	case "verify":
		if strings.HasSuffix(strings.TrimSpace(ev.Detail), "...") {
			m.curTool, m.curDetail = "verify", "build · vet · tests · runtime checks"
			break
		}
		cmd = tea.Printf("\n%s\n", renderVerify(ev.OK, ev.Detail, m.w))

	case "retry":
		m.note = fmt.Sprintf("engine busy — retrying (attempt %d)", ev.Attempt)

	case "usage":
		dIn := ev.Usage.InputTokens + ev.Usage.CacheReadTokens + ev.Usage.CacheWriteTokens - (m.prevUsage.InputTokens + m.prevUsage.CacheReadTokens + m.prevUsage.CacheWriteTokens)
		dOut := ev.Usage.OutputTokens - m.prevUsage.OutputTokens
		if dIn > 0 || dOut > 0 {
			m.meter.Add(max(0, dIn), max(0, dOut))
		}
		m.prevUsage = ev.Usage
		m.usage = ev.Usage

	case "info":
		cmd = tea.Printf("%s\n", display.DimStyle.Render("· "+ev.Text))

	case "done":
		// Final prose already streamed
		break

	case "error":
		cmd = tea.Printf("\n%s\n", display.ErrorStyle.Render("✗ "+friendlyError(ev.Err)))
	}

	if cmd == nil {
		return m, m.waitEvent()
	}
	return m, tea.Batch(cmd, m.waitEvent())
}

// ────────────────────────────────────────────────────────────────────────────
// Slash commands
// ────────────────────────────────────────────────────────────────────────────

func (m chatModel) handleSlash(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := parts[0]

	var infos []string
	addInfo := func(s string) {
		infos = append(infos, s)
	}

	switch cmd {
	case "/exit", "/quit":
		m.quitting = true
		return m, tea.Quit

	case "/clear":
		if m.state == stateRunning {
			addInfo("cannot clear while a task is running")
			break
		}
		m.ag.Reset()
		return m, tea.ClearScreen

	case "/help":
		addInfo(helpText())

	case "/tokens", "/cost":
		addInfo(fmt.Sprintf("session tokens: %s (in %s / out %s)",
			display.FormatCommas(m.meter.TotalSessionTokens()),
			display.FormatCommas(m.meter.SessionInputTokens),
			display.FormatCommas(m.meter.SessionOutputTokens)))

	case "/status":
		addInfo(projectStatus(m))

	case "/copy":
		if m.lastProse == "" {
			addInfo("nothing to copy yet")
		} else if err := clipboard.WriteAll(m.lastProse); err != nil {
			addInfo("copy failed: " + err.Error())
		} else {
			addInfo(fmt.Sprintf("copied %d characters to the clipboard", len(m.lastProse)))
		}

	case "/history":
		addInfo(snapshotHistory(10))

	case "/runs":
		addInfo(verificationRuns(m.projectDir, 10))

	case "/audit":
		addInfo(auditTrail(m.projectDir, 12))

	case "/memory":
		addInfo(projectMemory(m.projectDir))

	case "/wiki":
		addInfo(wikiIndex(m.projectDir, 40))

	case "/recall":
		raw := strings.TrimSpace(strings.TrimPrefix(input, "/recall"))
		if raw == "" {
			addInfo("usage: /recall <query>")
			break
		}
		addInfo(recallWiki(m.projectDir, raw, 8))

	case "/remember":
		raw := strings.TrimSpace(strings.TrimPrefix(input, "/remember"))
		if raw == "" {
			addInfo("usage: /remember <note>")
			break
		}
		if err := appendProjectMemory(m.projectDir, raw); err != nil {
			addInfo("memory update failed: " + err.Error())
		} else {
			addInfo("saved to .lore/memory.md")
		}

	case "/rollback":
		if m.state == stateRunning {
			addInfo("cannot roll back while a task is running")
			break
		}
		if len(parts) < 2 {
			addInfo("usage: /rollback <n> — see /history for snapshot numbers")
			break
		}
		addInfo(doRollback(m.projectDir, parts[1]))

	case "/approve":
		m.approveMode = !m.approveMode
		if m.approveMode {
			m.permission = agent.PermissionAsk
			addInfo("approval mode ON — file writes and shell commands will ask for approval")
		} else {
			m.permission = agent.PermissionFullAuto
			addInfo("approval mode OFF — writes apply automatically (undo with /rollback)")
		}

	case "/permissions":
		if len(parts) == 1 {
			addInfo("permission mode: " + string(m.permission) + " (full-auto, auto-safe, ask, read-only)")
			break
		}
		mode, ok := parsePermissionMode(parts[1])
		if !ok {
			addInfo("usage: /permissions full-auto|auto-safe|ask|read-only")
			break
		}
		m.permission = mode
		m.approveMode = mode == agent.PermissionAsk
		addInfo("permission mode set to " + string(mode))

	case "/engine":
		addInfo("engine: " + m.engineName)

	case "/sh":
		raw := strings.TrimSpace(strings.TrimPrefix(input, "/sh"))
		if raw == "" {
			addInfo("usage: /sh <command>")
			break
		}
		return m, runUserShell(m.projectDir, raw)

	default:
		addInfo(fmt.Sprintf("unknown command %s — see /help", cmd))
	}

	var cmds []tea.Cmd
	for _, info := range infos {
		cmds = append(cmds, tea.Printf("%s\n", display.DimStyle.Render("· "+info)))
	}
	return m, tea.Batch(cmds...)
}

func helpText() string {
	return strings.TrimSpace(`
commands:
  /help            show this help
  /clear           clear the conversation (Ctrl+L clears the screen only)
  /tokens          session token usage
  /status          show engine, safety, memory, and run state
  /copy            copy the last response to the clipboard
  /memory          show project memory
  /wiki            list Lore wiki documents
  /recall <query>  search Lore memory and wiki
  /remember <note> save a note to project memory
  /history         list undo snapshots
  /rollback <n>    restore the project to snapshot n
  /runs            list recent verification ledgers
  /audit           show recent tool activity
  /approve         toggle file-write and shell-command approval mode
  /permissions     show/set full-auto, auto-safe, ask, or read-only
  /engine          show the active AI engine
  /sh <command>    run a shell command yourself
  /exit            quit

keys:
  Enter            send
  Alt+Enter        insert a newline
  Ctrl+D           send
  Ctrl+V           paste
  Esc              cancel the running task
  Ctrl+C           cancel task / quit`)
}

func runUserShell(dir, command string) tea.Cmd {
	return func() tea.Msg {
		runner := shell.NewRunner(shell.Config{WorkDir: dir, Timeout: 120 * time.Second})
		outCh, resCh, _ := runner.Run(context.Background(), command)
		for range outCh {
		}
		res := <-resCh
		out := res.Output
		if len(out) > 8000 {
			out = out[:8000] + "\n…[truncated]"
		}
		return shellDoneMsg{command: command, output: out, exit: res.ExitCode}
	}
}

// ────────────────────────────────────────────────────────────────────────────
// Rendering
// ────────────────────────────────────────────────────────────────────────────

var toolNameStyle = lipgloss.NewStyle().Foreground(display.ColorCyan)

func renderVerify(ok bool, detail string, width int) string {
	title := display.SuccessStyle.Render("✓ verification passed")
	border := display.ColorGreen
	if !ok {
		title = display.ErrorStyle.Render("✗ verification failed")
		border = display.ColorRed
	}
	body := strings.TrimRight(detail, "\n")
	if len(body) > 3000 {
		body = body[:3000] + "\n…[truncated]"
	}
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(min(width-2, 100))
	return box.Render(title + "\n" + display.DimStyle.Render(body))
}

func renderMarkdown(src string, width int) string {
	src = strings.TrimRight(src, "\n")
	if strings.TrimSpace(src) == "" {
		return ""
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(max(40, min(width-4, 100))),
	)
	if err != nil {
		return src
	}
	out, err := r.Render(src)
	if err != nil {
		return src
	}
	return strings.TrimRight(out, "\n")
}

func (m chatModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	if m.activeProse != "" {
		b.WriteString("\n" + m.activeProse)
	}

	status := m.statusLine()

	inputStyle := display.InputBoxStyle
	if m.state == stateRunning {
		inputStyle = display.InputBoxWaitingStyle
	}
	input := inputStyle.Width(max(24, m.w-2)).Render(m.ta.View())

	meter := " " + m.meter.Display()

	b.WriteString("\n\n" + status + "\n" + input + "\n" + meter)
	return b.String()
}

func (m chatModel) statusLine() string {
	if m.approval != nil {
		prompt := "apply this change to " + m.approval.path + "?"
		if m.approval.action != "" {
			prompt = "allow " + m.approval.action + "?"
		} else if m.approval.path == "" {
			prompt = "run this shell command?"
		}
		return " " + display.BoldStyle.Render(prompt) +
			display.DimStyle.Render("  [a/enter] allow once · [A] allow always · [n/esc] deny")
	}
	switch m.state {
	case stateRunning:
		elapsed := time.Since(m.runStart).Round(time.Second)
		what := "thinking"
		if m.curTool != "" {
			what = m.curTool
			if m.curDetail != "" {
				what += " · " + trimTo(m.curDetail, max(10, m.w-40))
			}
		}
		line := fmt.Sprintf(" %s %s %s", m.sp.View(), what, display.DimStyle.Render(elapsed.String()))
		if m.lastLine != "" {
			line += display.DimStyle.Render("  │ " + trimTo(m.lastLine, max(10, m.w-len(what)-30)))
		}
		if m.note != "" {
			line += "  " + display.DimStyle.Render(m.note)
		}
		if m.showBusyWarning {
			line += "  " + display.ErrorStyle.Render("· a task is already running — press Esc to cancel it first")
		}
		return line
	default:
		hint := "Enter to send · Alt+Enter newline · /help"
		if m.permission != agent.PermissionFullAuto {
			hint += " · " + string(m.permission)
		}
		return " " + display.DimStyle.Render(hint)
	}
}

func parsePermissionMode(s string) (agent.PermissionMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case string(agent.PermissionFullAuto):
		return agent.PermissionFullAuto, true
	case string(agent.PermissionAutoSafe):
		return agent.PermissionAutoSafe, true
	case string(agent.PermissionAsk):
		return agent.PermissionAsk, true
	case string(agent.PermissionReadOnly):
		return agent.PermissionReadOnly, true
	default:
		return "", false
	}
}

func configuredPermissionMode(cfg *config.Config) agent.PermissionMode {
	if cfg == nil {
		return agent.PermissionFullAuto
	}
	mode, ok := parsePermissionMode(cfg.Safety.PermissionMode)
	if !ok {
		return agent.PermissionFullAuto
	}
	return mode
}

func trimTo(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if n > 0 && len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func trimLen(s string, n int) string {
	if n >= 0 && len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ────────────────────────────────────────────────────────────────────────────
// Shared helpers (also used by other commands)
// ────────────────────────────────────────────────────────────────────────────

func friendlyError(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "401"):
		return "The provider rejected your API key — check it with `lore config`."
	case strings.Contains(s, "429"):
		return "Rate limited — wait a moment and try again."
	case strings.Contains(s, "context canceled"):
		return "cancelled"
	}
	return s
}

type loreDoc struct {
	Rel   string
	Title string
	Body  string
	Size  int64
	Mod   time.Time
}

func loreMarkdownDocs(cwd string, maxDocBytes int) []loreDoc {
	root := filepath.Join(cwd, ".lore")
	var docs []loreDoc
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			switch info.Name() {
			case "snapshots", "runs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(info.Name()), ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		body := string(data)
		if maxDocBytes > 0 {
			body = trimLen(body, maxDocBytes)
		}
		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			rel = path
		}
		docs = append(docs, loreDoc{
			Rel:   filepath.ToSlash(rel),
			Title: markdownTitle(body, filepath.Base(path)),
			Body:  body,
			Size:  info.Size(),
			Mod:   info.ModTime(),
		})
		return nil
	})
	sort.Slice(docs, func(i, j int) bool {
		pi, pj := loreDocPriority(docs[i].Rel), loreDocPriority(docs[j].Rel)
		if pi != pj {
			return pi < pj
		}
		if !docs[i].Mod.Equal(docs[j].Mod) {
			return docs[i].Mod.After(docs[j].Mod)
		}
		return docs[i].Rel < docs[j].Rel
	})
	return docs
}

func loreDocPriority(rel string) int {
	switch filepath.ToSlash(rel) {
	case ".lore/memory.md":
		return 0
	case ".lore/index.md":
		return 1
	case ".lore/schema.md":
		return 2
	case ".lore/log.md":
		return 3
	default:
		return 10
	}
}

func markdownTitle(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return strings.TrimSpace(strings.TrimLeft(line, "#"))
		}
	}
	return fallback
}

func projectStatus(m chatModel) string {
	docs := loreMarkdownDocs(m.projectDir, 0)
	runs := countFiles(filepath.Join(m.projectDir, ".lore", "runs"), ".json")
	auditEntries := countNonEmptyLines(filepath.Join(m.projectDir, ".lore", "audit.jsonl"))
	memoryNotes := memoryNoteCount(filepath.Join(m.projectDir, ".lore", "memory.md"))

	state := "idle"
	if m.state == stateRunning {
		state = "running"
	}
	return strings.TrimSpace(fmt.Sprintf(`status:
  state: %s
  engine: %s
  permission: %s
  session tokens: %s
  wiki docs: %d
  memory notes: %d
  verification ledgers: %d
  audit entries: %d`,
		state,
		m.engineName,
		m.permission,
		display.FormatCommas(m.meter.TotalSessionTokens()),
		len(docs),
		memoryNotes,
		runs,
		auditEntries,
	))
}

func wikiIndex(cwd string, n int) string {
	docs := loreMarkdownDocs(cwd, 0)
	if len(docs) == 0 {
		return "No Lore wiki documents yet. Run `lore init` to create .lore/."
	}
	if len(docs) > n {
		docs = docs[:n]
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("wiki documents (%d):\n", len(docs)))
	for i, d := range docs {
		fmt.Fprintf(&sb, "  %2d.  %-40s  %s  %s\n",
			i+1,
			trimTo(d.Rel, 40),
			displayBytes(d.Size),
			trimTo(d.Title, 70),
		)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func recallWiki(cwd, query string, n int) string {
	query = strings.TrimSpace(query)
	if query == "" {
		return "usage: /recall <query>"
	}
	docs := loreMarkdownDocs(cwd, 64*1024)
	for _, name := range []string{"LORE.md", "lore.md"} {
		path := filepath.Join(cwd, name)
		data, err := os.ReadFile(path)
		if err == nil {
			info, _ := os.Stat(path)
			size := int64(len(data))
			mod := time.Time{}
			if info != nil {
				size = info.Size()
				mod = info.ModTime()
			}
			docs = append(docs, loreDoc{
				Rel:   name,
				Title: markdownTitle(string(data), name),
				Body:  trimLen(string(data), 64*1024),
				Size:  size,
				Mod:   mod,
			})
			break
		}
	}
	if len(docs) == 0 {
		return "No Lore memory or wiki documents found."
	}
	terms := recallTerms(query)
	type hit struct {
		doc     loreDoc
		score   int
		snippet string
	}
	needle := strings.ToLower(query)
	var hits []hit
	for _, d := range docs {
		bodyLower := strings.ToLower(d.Body)
		titleLower := strings.ToLower(d.Title)
		score := strings.Count(bodyLower, needle) * 4
		if strings.Contains(titleLower, needle) {
			score += 8
		}
		for _, term := range terms {
			score += strings.Count(bodyLower, term)
			if strings.Contains(titleLower, term) {
				score += 3
			}
		}
		if strings.Contains(strings.ToLower(d.Title), needle) {
			score += 3
		}
		if score == 0 {
			continue
		}
		hits = append(hits, hit{doc: d, score: score, snippet: firstMatchingLine(d.Body, append([]string{needle}, terms...))})
	}
	if len(hits) == 0 {
		return "No memory/wiki matches for " + strconv.Quote(query) + "."
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return loreDocPriority(hits[i].doc.Rel) < loreDocPriority(hits[j].doc.Rel)
	})
	if len(hits) > n {
		hits = hits[:n]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("recall matches for %q:\n", query))
	for i, h := range hits {
		fmt.Fprintf(&sb, "  %2d.  %s  score:%d  %s\n      %s\n",
			i+1,
			h.doc.Rel,
			h.score,
			trimTo(h.doc.Title, 60),
			trimTo(h.snippet, 110),
		)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func recallTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, raw := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) {
		raw = strings.TrimSpace(raw)
		if len(raw) < 2 || seen[raw] {
			continue
		}
		seen[raw] = true
		terms = append(terms, raw)
	}
	return terms
}

func firstMatchingLine(body string, needles []string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		for _, needle := range needles {
			if needle != "" && strings.Contains(lower, needle) {
				return line
			}
		}
	}
	return "(match found)"
}

func countFiles(dir, ext string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ext) {
			n++
		}
	}
	return n
}

func countNonEmptyLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func memoryNoteCount(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			n++
		}
	}
	return n
}

func displayBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func snapshotHistory(n int) string {
	cwd, err := os.Getwd()
	if err != nil {
		return display.ErrorStyle.Render(err.Error())
	}
	snapshots, err := snapshot.ListSnapshots(cwd, n)
	if err != nil {
		return display.ErrorStyle.Render("Error listing snapshots: " + err.Error())
	}
	if len(snapshots) == 0 {
		return "No snapshots yet. A snapshot is taken before each agent task."
	}
	hashStyle := lipgloss.NewStyle().Foreground(display.ColorYellow)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("snapshots (last %d):\n", len(snapshots)))
	for i, s := range snapshots {
		sb.WriteString(fmt.Sprintf("  %2d.  %s  %s  %s\n",
			i+1,
			s.Timestamp.Format("2006-01-02 15:04"),
			hashStyle.Render(s.ShortHash),
			s.Message,
		))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func verificationRuns(dir string, n int) string {
	type ledgerStep struct {
		Name    string `json:"name"`
		Passed  bool   `json:"passed"`
		Skipped bool   `json:"skipped"`
	}
	type ledger struct {
		Time   string       `json:"time"`
		Passed bool         `json:"passed"`
		Checks []any        `json:"checks"`
		Steps  []ledgerStep `json:"steps"`
	}
	runsDir := filepath.Join(dir, ".lore", "runs")
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		return "No verification ledgers yet. They are created after verify_app runs."
	}
	type item struct {
		name string
		mod  time.Time
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		items = append(items, item{name: e.Name(), mod: info.ModTime()})
	}
	if len(items) == 0 {
		return "No verification ledgers yet. They are created after verify_app runs."
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	if len(items) > n {
		items = items[:n]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("verification runs (last %d):\n", len(items)))
	for i, it := range items {
		path := filepath.Join(runsDir, it.name)
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(&sb, "  %2d.  %s  unreadable\n", i+1, filepath.ToSlash(filepath.Join(".lore", "runs", it.name)))
			continue
		}
		var l ledger
		if err := json.Unmarshal(data, &l); err != nil {
			fmt.Fprintf(&sb, "  %2d.  %s  invalid JSON\n", i+1, filepath.ToSlash(filepath.Join(".lore", "runs", it.name)))
			continue
		}
		status := "FAIL"
		if l.Passed {
			status = "PASS"
		}
		when := l.Time
		if t, err := time.Parse(time.RFC3339, l.Time); err == nil {
			when = t.Format("2006-01-02 15:04")
		}
		passed, failed, skipped := 0, 0, 0
		for _, s := range l.Steps {
			switch {
			case s.Skipped:
				skipped++
			case s.Passed:
				passed++
			default:
				failed++
			}
		}
		fmt.Fprintf(&sb, "  %2d.  %-4s  %s  checks:%d  steps:%d  pass:%d fail:%d skip:%d  %s\n",
			i+1,
			status,
			when,
			len(l.Checks),
			len(l.Steps),
			passed,
			failed,
			skipped,
			filepath.ToSlash(filepath.Join(".lore", "runs", it.name)),
		)
	}
	return strings.TrimRight(sb.String(), "\n")
}

func auditTrail(dir string, n int) string {
	data, err := os.ReadFile(filepath.Join(dir, ".lore", "audit.jsonl"))
	if err != nil {
		return "No audit trail yet. Tool activity is logged after agent tool calls."
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var recent []string
	for i := len(lines) - 1; i >= 0 && len(recent) < n; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			recent = append(recent, line)
		}
	}
	if len(recent) == 0 {
		return "No audit trail yet. Tool activity is logged after agent tool calls."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("tool audit (last %d):\n", len(recent)))
	for i := len(recent) - 1; i >= 0; i-- {
		var entry struct {
			Time          string `json:"time"`
			Tool          string `json:"tool"`
			OK            bool   `json:"ok"`
			InputSummary  string `json:"input_summary"`
			ResultSummary string `json:"result_summary"`
		}
		if err := json.Unmarshal([]byte(recent[i]), &entry); err != nil {
			fmt.Fprintf(&sb, "  - invalid audit entry\n")
			continue
		}
		status := "FAIL"
		if entry.OK {
			status = "OK"
		}
		when := entry.Time
		if t, err := time.Parse(time.RFC3339, entry.Time); err == nil {
			when = t.Format("15:04:05")
		}
		detail := entry.InputSummary
		if detail == "" {
			detail = entry.ResultSummary
		}
		fmt.Fprintf(&sb, "  - %s  %-4s  %-13s %s\n", when, status, entry.Tool, trimTo(detail, 96))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func doRollback(dir, indexStr string) string {
	snapshots, err := snapshot.ListSnapshots(dir, 10)
	if err != nil || len(snapshots) == 0 {
		return "No snapshots available."
	}
	idx, convErr := strconv.Atoi(strings.TrimSpace(indexStr))
	if convErr != nil || idx < 1 || idx > len(snapshots) {
		return fmt.Sprintf("Invalid number %q — use /history to see snapshots.", indexStr)
	}
	s := snapshots[idx-1]
	if err := snapshot.RestoreSnapshot(dir, s.Hash); err != nil {
		return "Restore failed: " + err.Error()
	}
	return "Rolled back to " + s.ShortHash + " (" + s.Message + ")"
}
