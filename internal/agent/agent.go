// Package agent implements Lore's tool-calling harness: it streams model
// turns, executes the structured tool calls they contain, feeds results
// back, and refuses to accept "done" until verification has actually passed.
//
// Because a tool call ends the model's message, the old failure mode of a
// premature "Done! ✅" printed before files exist is structurally impossible:
// prose and actions can no longer interleave inside one text blob.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lore-cli/internal/engine"
	"lore-cli/internal/lorefs"
	"lore-cli/internal/verify"
)

// Agent drives one conversation against a Provider.
type Agent struct {
	Provider engine.Provider
	Dir      string  // project root (absolute)
	Env      EnvInfo // machine facts (OS, shell, CGO, toolchains)

	// Optional hooks / tuning.
	Approver     Approver      // gate file writes (nil = auto-approve)
	EventSink    func(Event)   // receives every UI event (required)
	ExtraContext string        // extra project context injected into the system prompt
	MaxTokens    int           // per-turn output cap (default 16384)
	MaxTurns     int           // hard cap on model turns per request (default 100)
	MaxFixRounds int           // harness-driven fix rounds after a failed final verification (default 6)
	ShellTimeout time.Duration // per-command timeout for run_shell (default 600s)
	RequireWork  bool          // headless mode: a request is assumed to need tool work; always nudge a workless stop once
	Permission   PermissionMode

	history []engine.Message
	usage   engine.Usage

	// verification gating
	dirty        bool // files changed since the last passing verify_app
	verifiedOnce bool // a verify_app has passed at least once this session

	shellCwd          string          // run_shell working directory, relative to Dir
	lastStop          string          // stop reason of the most recent streamed turn
	autoApprovedFiles map[string]bool // files that the user has chosen to 'Allow Always'
	autoApprovedCmds  map[string]bool // shell commands the user has chosen to 'Allow Always'
	autoApprovedActs  map[string]bool // generic actions the user has chosen to 'Allow Always'
}

// Outcome reports how a request finished.
type Outcome struct {
	OK        bool   // true when the work is verified (or no files were touched)
	FinalText string // the model's final prose
	Detail    string // honest explanation when OK is false
}

func (a *Agent) emit(ev Event) {
	if a.EventSink != nil {
		a.EventSink(ev)
	}
}

func (a *Agent) markDirty() { a.dirty = true }

// Usage returns cumulative token usage for the session.
func (a *Agent) Usage() engine.Usage { return a.usage }

// History returns the conversation so far (for session persistence).
func (a *Agent) History() []engine.Message { return a.history }

// Reset clears the conversation.
func (a *Agent) Reset() {
	a.history = nil
	a.dirty = false
	a.verifiedOnce = false
	a.shellCwd = ""
}

func (a *Agent) permissionMode() PermissionMode {
	if a.Permission == "" {
		return PermissionFullAuto
	}
	return a.Permission
}

func (a *Agent) actionAllowed(kind string) bool {
	mode := a.permissionMode()
	switch mode {
	case PermissionFullAuto:
		return true
	case PermissionReadOnly:
		return kind == "read"
	case PermissionAsk:
		return true
	case PermissionAutoSafe:
		return true
	default:
		return true
	}
}

func (a *Agent) wantsApproval(kind string) bool {
	mode := a.permissionMode()
	if kind == "read" {
		return false
	}
	if mode == PermissionAsk {
		return true
	}
	if mode == PermissionAutoSafe {
		switch kind {
		case "shell", "delete", "setup", "verify":
			return true
		}
	}
	return a.Approver != nil
}

func (a *Agent) audit(tool string, input json.RawMessage, ok bool, summary string) {
	if strings.TrimSpace(a.Dir) == "" {
		return
	}
	entry := map[string]any{
		"time":           time.Now().Format(time.RFC3339),
		"tool":           tool,
		"ok":             ok,
		"input_summary":  trimLen(summarizeInput(tool, input), 500),
		"result_summary": trimLen(summary, 500),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	dir := filepath.Join(a.Dir, ".lore")
	if err := lorefs.MkdirPrivate(dir); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "audit.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, lorefs.PrivateFileMode)
	if err != nil {
		return
	}
	_ = os.Chmod(filepath.Join(dir, "audit.jsonl"), lorefs.PrivateFileMode)
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

// Run executes one user request to completion: model turns, tool execution,
// verification gating, and the fix loop. It blocks until the request is
// finished or ctx is cancelled.
func (a *Agent) Run(ctx context.Context, userInput string) Outcome {
	if a.MaxTokens <= 0 {
		a.MaxTokens = 16384
	}
	if a.MaxTurns <= 0 {
		a.MaxTurns = 100
	}
	if a.MaxFixRounds <= 0 {
		a.MaxFixRounds = 6
	}
	if a.ShellTimeout <= 0 {
		a.ShellTimeout = 600 * time.Second
	}

	a.history = append(a.history, engine.UserText(userInput))

	var (
		finalText  string
		nudges     int
		fixRounds  int
		toolsUsed  bool
		workNudged bool
	)

	for turn := 0; turn < a.MaxTurns; turn++ {
		msg, err := a.streamTurn(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return Outcome{OK: false, Detail: "cancelled"}
			}
			a.emit(Event{Kind: "error", Err: err})
			return Outcome{OK: false, Detail: err.Error()}
		}

		sanitizeToolInputs(msg)
		a.history = append(a.history, *msg)
		calls := msg.ToolCalls()

		if len(calls) > 0 {
			toolsUsed = true
			results := a.execCalls(ctx, calls)
			if ctx.Err() != nil {
				return Outcome{OK: false, Detail: "cancelled"}
			}
			a.history = append(a.history, engine.Message{Role: "user", Blocks: results})
			a.compressHistory()
			continue
		}

		// No tool calls: the model considers its turn complete.
		finalText = msg.Text()

		if a.lastStop == "max_tokens" {
			a.history = append(a.history, engine.UserText(
				"Your response was cut off by the output limit. Continue exactly where you left off."))
			continue
		}

		// A turn that announced work ("Let me start...") but called no tools
		// is a stall, not an answer — weaker models do this. Push back once;
		// a genuine question gets its answer restated and accepted.
		if !toolsUsed && !workNudged && (a.RequireWork || looksUnfinished(finalText)) {
			workNudged = true
			a.emit(Event{Kind: "info", Text: "model stopped without doing any work — nudging it to proceed"})
			a.history = append(a.history, engine.UserText(
				"You stopped without calling any tools. If this request requires creating or changing files or running commands, do that work NOW via tool calls (for a new project, start with setup_project). "+
					"Only reply without tools if the request is a pure question — in that case give the complete final answer."))
			continue
		}

		if !a.dirty {
			// Nothing unverified — accept the completion.
			a.emit(Event{Kind: "done", OK: true, Text: finalText})
			return Outcome{OK: true, FinalText: finalText}
		}

		// Files changed but verification has not passed since. Refuse the
		// completion and push the model back to work (capped), then take
		// over verification ourselves.
		if nudges < 2 {
			nudges++
			a.emit(Event{Kind: "info", Text: "completion refused: unverified changes — requesting verification"})
			a.history = append(a.history, engine.UserText(
				"You have file changes that have not passed verification. Do not summarize or claim completion. "+
					"Run verify_app now with runtime checks that exercise the real artifact, and fix anything that fails."))
			continue
		}

		// The model would not verify; the harness verifies directly.
		a.emit(Event{Kind: "verify", OK: true, Detail: "running harness verification (build, vet, tests, smoke)..."})
		res := a.harnessVerify()
		a.emit(Event{Kind: "verify", OK: res.Passed, Detail: res.Summary()})
		if res.Passed {
			a.dirty = false
			done := finalText
			if done == "" {
				done = "Task complete — verification passed."
			}
			a.emit(Event{Kind: "done", OK: true, Text: done})
			return Outcome{OK: true, FinalText: done}
		}
		if fixRounds < a.MaxFixRounds {
			fixRounds++
			a.emit(Event{Kind: "info", Text: fmt.Sprintf("verification failed — starting fix round %d/%d", fixRounds, a.MaxFixRounds)})
			a.history = append(a.history, engine.UserText(
				"Verification of your changes FAILED. Real output below. Fix the code, then run verify_app until it passes.\n\n"+res.Summary()))
			continue
		}

		detail := "Verification still failing after " + fmt.Sprint(a.MaxFixRounds) + " fix rounds. Last results:\n" + res.Summary()
		a.emit(Event{Kind: "done", OK: false, Text: finalText, Detail: detail})
		return Outcome{OK: false, FinalText: finalText, Detail: detail}
	}

	// Turn budget exhausted. If the last verify_app passed and nothing
	// changed since, the work itself is verified — the model just never got
	// a turn for its closing prose. Don't fail verified work on a formality.
	if !a.dirty && a.verifiedOnce {
		text := finalText
		if text == "" {
			text = "Task complete — verification passed (turn limit reached before a final summary)."
		}
		a.emit(Event{Kind: "done", OK: true, Text: text})
		return Outcome{OK: true, FinalText: text}
	}

	detail := fmt.Sprintf("stopped after %d model turns without completing — the task may be too large for one request", a.MaxTurns)
	a.emit(Event{Kind: "done", OK: false, Text: finalText, Detail: detail})
	return Outcome{OK: false, FinalText: finalText, Detail: detail}
}

// streamTurn streams one model turn, forwarding deltas to the UI. The turn
// is retried on transient stream errors (history is unchanged until the
// turn lands, so retrying is safe).
func (a *Agent) streamTurn(ctx context.Context) (*engine.Message, error) {
	const turnAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= turnAttempts; attempt++ {
		if attempt > 1 {
			a.emit(Event{Kind: "retry", Attempt: attempt})
			select {
			case <-time.After(4 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req := engine.Request{
			System:    a.systemPrompt(),
			Messages:  a.history,
			Tools:     toolDefs(),
			MaxTokens: a.MaxTokens,
		}

		var done *engine.Event
		failed := false
		for ev := range a.Provider.Stream(ctx, req) {
			switch ev.Type {
			case "text":
				a.emit(Event{Kind: "text", Text: ev.Text})
			case "tool_start":
				a.emit(Event{Kind: "tool_start", Tool: ev.ToolName, ToolID: ev.ToolID})
			case "retry":
				a.emit(Event{Kind: "retry", Attempt: ev.Attempt})
			case "done":
				e := ev
				done = &e
			case "error":
				lastErr = ev.Err
				failed = true
			}
		}
		if failed || done == nil {
			if lastErr == nil {
				lastErr = fmt.Errorf("engine stream ended without a result")
			}
			continue
		}

		a.usage.Add(done.Usage)
		a.emit(Event{Kind: "usage", Usage: a.usage})
		a.lastStop = done.StopReason
		a.emit(Event{Kind: "turn", Detail: done.StopReason})
		return done.Message, nil
	}
	return nil, fmt.Errorf("the AI engine is unreachable after %d attempts: %w", turnAttempts, lastErr)
}

// execCalls runs each tool call in order and returns the tool_result blocks.
func (a *Agent) execCalls(ctx context.Context, calls []engine.Block) []engine.Block {
	var results []engine.Block
	for _, call := range calls {
		if ctx.Err() != nil {
			results = append(results, engine.ToolResultBlock(call.ID, "cancelled by user", true))
			continue
		}
		a.emit(Event{Kind: "tool_start", Tool: call.Name, ToolID: call.ID, Detail: summarizeInput(call.Name, call.Input)})

		out, ok := a.execTool(ctx, call.Name, call.Input)
		a.audit(call.Name, call.Input, ok, summarizeResult(call.Name, out, ok))

		a.emit(Event{Kind: "tool_done", Tool: call.Name, ToolID: call.ID, OK: ok, Detail: summarizeResult(call.Name, out, ok)})
		results = append(results, engine.ToolResultBlock(call.ID, out, !ok))
	}
	return results
}

// harnessVerify is the fallback pipeline when the model won't verify:
// build/vet/tests plus a generic runtime smoke check of the artifact.
func (a *Agent) harnessVerify() verify.Result {
	var checks []verify.Check
	if mainDir := goMainDir(a.Dir); mainDir != "" {
		bin := "lore-smoke-bin.exe"
		checks = append(checks,
			verify.Check{Type: "cli", Command: fmt.Sprintf("go build -o %s %s", bin, mainDir)},
			verify.Check{Type: "cli", Command: ".\\" + bin + " --help"},
		)
	}
	return verify.Run(verify.Options{Dir: a.Dir, Env: a.Env.BuildEnv(), Checks: checks})
}

// ── history compression ───────────────────────────────────────────────────────

// compressHistory keeps context growth bounded on long sessions. When the
// history exceeds a byte budget, older tool results AND older write_file
// inputs (whole files, by far the largest blocks) are elided — the files
// are on disk and the model can re-read whatever it needs.
func (a *Agent) compressHistory() {
	const budget = 240_000 // ~60k tokens
	const keepTail = 16    // never touch the most recent messages

	total := 0
	for _, m := range a.history {
		for _, b := range m.Blocks {
			total += len(b.Text) + len(b.Content) + len(b.Input)
		}
	}
	if total <= budget || len(a.history) <= keepTail {
		return
	}

	cutoff := len(a.history) - keepTail
	for i := 0; i < cutoff && total > budget; i++ {
		m := &a.history[i]
		for j := range m.Blocks {
			b := &m.Blocks[j]
			switch b.Type {
			case "tool_result":
				if len(b.Content) > 240 {
					total -= len(b.Content) - 200
					b.Content = b.Content[:160] + "\n...[elided older tool output; re-run the command if needed]"
				}
			case "tool_use":
				if b.Name == "write_file" && len(b.Input) > 400 {
					var in struct {
						Path string `json:"path"`
					}
					_ = json.Unmarshal(b.Input, &in)
					repl, err := json.Marshal(map[string]string{
						"path":    in.Path,
						"content": "[elided older version — use read_file for the current content]",
					})
					if err == nil {
						total -= len(b.Input) - len(repl)
						b.Input = repl
					}
				}
			}
		}
	}
}

// sanitizeToolInputs replaces tool_use inputs that are not valid JSON with a
// valid marker object. Models occasionally emit broken string escapes; if the
// raw bytes were kept, re-marshaling the history would fail on every later
// request and kill the session. The marker makes the executor return a clear
// "resend the call" error instead.
func sanitizeToolInputs(msg *engine.Message) {
	for i := range msg.Blocks {
		b := &msg.Blocks[i]
		if b.Type != "tool_use" || json.Valid(b.Input) {
			continue
		}
		raw := string(b.Input)
		if len(raw) > 160 {
			raw = raw[:160]
		}
		repl, err := json.Marshal(map[string]string{"_malformed_input": raw})
		if err != nil {
			repl = []byte(`{"_malformed_input":"unrepresentable"}`)
		}
		b.Input = repl
	}
}

// looksUnfinished reports whether final prose reads like an announced
// intention rather than a completed answer (trailing colon, "let me", etc.).
func looksUnfinished(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	if strings.HasSuffix(t, ":") {
		return true
	}
	tail := strings.ToLower(t)
	if len(tail) > 240 {
		tail = tail[len(tail)-240:]
	}
	for _, marker := range []string{"let me ", "let's ", "i'll ", "i will ", "now i ", "next, i ", "first, i "} {
		if strings.Contains(tail, marker) {
			return true
		}
	}
	return false
}

// ── input/result summaries for the UI ────────────────────────────────────────

func summarizeInput(name string, input json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	get := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	switch name {
	case "write_file":
		content := get("content")
		return fmt.Sprintf("%s (%d lines)", get("path"), countLines(content))
	case "read_file", "delete_file", "list_files":
		return get("path")
	case "search_code":
		return get("query")
	case "run_shell":
		return get("command")
	case "setup_project":
		lang := get("language")
		var deps []string
		if arr, ok := m["deps"].([]any); ok {
			for _, d := range arr {
				if s, ok := d.(string); ok {
					deps = append(deps, s)
				}
			}
		}
		if len(deps) > 0 {
			return lang + ": " + strings.Join(deps, ", ")
		}
		return lang
	case "verify_app":
		if arr, ok := m["checks"].([]any); ok {
			return fmt.Sprintf("%d runtime checks", len(arr))
		}
	}
	return ""
}

func summarizeResult(name, out string, ok bool) string {
	if name == "verify_app" {
		if ok {
			return "verification passed"
		}
		return "verification failed"
	}
	first := strings.TrimSpace(out)
	if i := strings.IndexByte(first, '\n'); i > 0 {
		first = first[:i]
	}
	return trimLen(first, 120)
}
