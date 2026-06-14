package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"lore-cli/internal/engine"
	"lore-cli/internal/lorefs"
	"lore-cli/internal/pathutil"
	"lore-cli/internal/shell"
	"lore-cli/internal/verify"
)

// moduleFiles are owned by the harness: the model must never author them.
// Hand-written module files routinely contain hallucinated transitive
// dependencies that poison `go mod tidy`.
var moduleFiles = map[string]bool{
	"go.mod":            true,
	"go.sum":            true,
	"go.work":           true,
	"go.work.sum":       true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
}

const (
	maxReadBytes   = 192 * 1024
	maxToolOutput  = 24 * 1024
	maxSearchHits  = 60
	maxTreeEntries = 250
)

// toolDefs returns the tool definitions advertised to the model.
func toolDefs() []engine.ToolDef {
	mk := func(name, desc, schema string) engine.ToolDef {
		return engine.ToolDef{Name: name, Description: desc, InputSchema: json.RawMessage(schema)}
	}
	return []engine.ToolDef{
		mk("write_file",
			"Create or overwrite one file with the COMPLETE file content. Always send the entire file, never a fragment or diff. Paths are relative to the project root. Never write go.mod/go.sum or lockfiles — use setup_project for dependencies.",
			`{"type":"object","properties":{
				"path":{"type":"string","description":"Relative path of the file, e.g. cmd/root.go"},
				"content":{"type":"string","description":"Complete file content"}
			},"required":["path","content"]}`),
		mk("read_file",
			"Read a file from the project. Returns the full content (large files are truncated).",
			`{"type":"object","properties":{
				"path":{"type":"string","description":"Relative path of the file"}
			},"required":["path"]}`),
		mk("delete_file",
			"Delete one file from the project.",
			`{"type":"object","properties":{
				"path":{"type":"string","description":"Relative path of the file"}
			},"required":["path"]}`),
		mk("list_files",
			"List the project file tree (directories and files), rooted at an optional subdirectory.",
			`{"type":"object","properties":{
				"path":{"type":"string","description":"Optional subdirectory, relative to the project root"}
			}}`),
		mk("search_code",
			"Search all project files for a literal text query. Returns file:line matches with the matching line.",
			`{"type":"object","properties":{
				"query":{"type":"string","description":"Literal text to find (not a regex)"}
			},"required":["query"]}`),
		mk("run_shell",
			"Run one shell command from the project root (or current directory after a previous `cd`). A bare `cd <dir>` persists for later commands; `cd x && cmd` affects only that line. Use && to chain. Commands must be valid for the OS/shell stated in your context. Output and exit code are returned; non-zero exit codes are reported as errors for you to fix.",
			`{"type":"object","properties":{
				"command":{"type":"string","description":"The command line to execute"}
			},"required":["command"]}`),
		mk("setup_project",
			"Initialize the project module and install dependencies. THE ONLY way to create or change go.mod / package.json dependencies. For Go it runs `go mod init` (when needed), `go get <dep>@latest` for each dependency, then `go mod tidy`. Call it again any time you need another dependency.",
			`{"type":"object","properties":{
				"language":{"type":"string","enum":["go","node","python"]},
				"module":{"type":"string","description":"Module/package name, e.g. github.com/user/todo or just todo"},
				"deps":{"type":"array","items":{"type":"string"},"description":"Dependencies to install, e.g. [\"github.com/spf13/cobra\",\"modernc.org/sqlite\"]"}
			},"required":["language"]}`),
		mk("verify_app",
			"REQUIRED before declaring any coding task complete. Runs the full pipeline: build, vet, all tests, then YOUR runtime smoke checks, which must actually exercise the produced artifact (run real CLI commands; start the server and hit real endpoints). The task is not done until this passes. Returns each step's real output. cli checks: {type:'cli', command, expect_exit?, expect_contains?}. http checks: {type:'http', start_command, port, requests:[{method?, path, body?, expect_status?, expect_header?, expect_body_contains?}]} — redirects are not followed, so 301s and Location headers are directly observable.",
			`{"type":"object","properties":{
				"checks":{"type":"array","minItems":1,"items":{"type":"object","properties":{
					"type":{"type":"string","enum":["cli","http"]},
					"command":{"type":"string"},
					"expect_exit":{"type":"integer"},
					"expect_contains":{"type":"string"},
					"start_command":{"type":"string"},
					"port":{"type":"integer"},
					"requests":{"type":"array","items":{"type":"object","properties":{
						"method":{"type":"string"},
						"path":{"type":"string"},
						"body":{"type":"string"},
						"expect_status":{"type":"integer"},
						"expect_header":{"type":"object","additionalProperties":{"type":"string"}},
						"expect_body_contains":{"type":"string"}
					},"required":["path"]}}
				},"required":["type"]}}
			},"required":["checks"]}`),
	}
}

// execTool runs one tool call and returns the result text for the model.
// ok=false marks the result as an error block so the model self-corrects.
func (a *Agent) execTool(ctx context.Context, name string, input json.RawMessage) (out string, ok bool) {
	if strings.Contains(string(input), `"_malformed_input"`) {
		return "your tool input was not valid JSON (broken string escaping) and was discarded — resend the complete tool call with correctly escaped content", false
	}
	switch name {
	case "write_file":
		return a.toolWriteFile(input)
	case "read_file":
		return a.toolReadFile(input)
	case "delete_file":
		return a.toolDeleteFile(input)
	case "list_files":
		return a.toolListFiles(input)
	case "search_code":
		return a.toolSearchCode(input)
	case "run_shell":
		return a.toolRunShell(ctx, input)
	case "setup_project":
		return a.toolSetupProject(ctx, input)
	case "verify_app":
		return a.toolVerifyApp(input)
	default:
		return fmt.Sprintf("unknown tool %q", name), false
	}
}

// ── file tools ────────────────────────────────────────────────────────────────

func (a *Agent) toolWriteFile(input json.RawMessage) (string, bool) {
	var in struct{ Path, Content string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	if !a.actionAllowed("write") {
		return "permission denied: current mode is read-only", false
	}
	if strings.TrimSpace(in.Path) == "" {
		return "path is required", false
	}
	base := strings.ToLower(filepath.Base(filepath.ToSlash(in.Path)))
	if moduleFiles[base] {
		return fmt.Sprintf("writing %s directly is not allowed: the harness owns module files. Use the setup_project tool to initialize the module and add dependencies.", base), false
	}

	rel := relativizePath(in.Path)
	abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(rel))
	if err != nil {
		return "unsafe path rejected: " + err.Error(), false
	}

	old, _ := os.ReadFile(abs)

	if a.wantsApproval("write") && !a.autoApprovedFiles[rel] {
		if a.Approver == nil {
			return "approval required but no approver is available", false
		}
		decision := a.Approver.ApproveWrite(rel, string(old), in.Content)
		if decision == ApproveDeny {
			return "the user rejected this file write. Ask what they would like changed.", false
		}
		if decision == ApproveAlways {
			if a.autoApprovedFiles == nil {
				a.autoApprovedFiles = make(map[string]bool)
			}
			a.autoApprovedFiles[rel] = true
		}
	}

	if err := os.MkdirAll(filepath.Dir(abs), lorefs.PublicDirMode); err != nil {
		return "mkdir: " + err.Error(), false
	}
	if err := os.WriteFile(abs, []byte(in.Content), lorefs.PublicFileMode); err != nil {
		return "write failed: " + err.Error(), false
	}
	a.markDirty()

	a.emit(Event{Kind: "file_diff", Path: rel, OldContent: string(old), NewContent: in.Content})

	action := "created"
	if len(old) > 0 {
		action = "overwrote"
	}
	return fmt.Sprintf("%s %s (%d lines)", action, rel, countLines(in.Content)), true
}

func (a *Agent) toolReadFile(input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(relativizePath(in.Path)))
	if err != nil {
		return "unsafe path rejected: " + err.Error(), false
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "read failed: " + err.Error(), false
	}
	if len(data) > maxReadBytes {
		return string(data[:maxReadBytes]) + "\n...[file truncated]", true
	}
	return string(data), true
}

func (a *Agent) toolDeleteFile(input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	if !a.actionAllowed("write") {
		return "permission denied: current mode is read-only", false
	}
	rel := relativizePath(in.Path)
	if ok, msg := a.approveAction("delete_file", rel, "delete", "delete:"+rel); !ok {
		return msg, false
	}
	abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(rel))
	if err != nil {
		return "unsafe path rejected: " + err.Error(), false
	}
	if err := os.Remove(abs); err != nil {
		if os.IsNotExist(err) {
			return "file does not exist: " + rel, true
		}
		return "delete failed: " + err.Error(), false
	}
	a.markDirty()
	return "deleted " + rel, true
}

func (a *Agent) toolListFiles(input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	_ = json.Unmarshal(input, &in)
	root := a.Dir
	if strings.TrimSpace(in.Path) != "" {
		abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(relativizePath(in.Path)))
		if err != nil {
			return "unsafe path rejected: " + err.Error(), false
		}
		root = abs
	}
	tree := ProjectTree(root, maxTreeEntries)
	if tree == "" {
		tree = "(empty directory)"
	}
	return tree, true
}

func (a *Agent) toolSearchCode(input json.RawMessage) (string, bool) {
	var in struct{ Query string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	q := in.Query
	if strings.TrimSpace(q) == "" {
		return "query is required", false
	}

	var hits []string
	_ = filepath.Walk(a.Dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || len(hits) >= maxSearchHits {
			if len(hits) >= maxSearchHits {
				return filepath.SkipAll
			}
			return nil
		}
		if info.IsDir() {
			if skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Size() > 2*1024*1024 || isBinaryName(info.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), q) {
			return nil
		}
		rel, _ := filepath.Rel(a.Dir, path)
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, q) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), i+1, strings.TrimSpace(trimLen(line, 200))))
				if len(hits) >= maxSearchHits {
					break
				}
			}
		}
		return nil
	})
	if len(hits) == 0 {
		return "no matches for " + strconvQuote(q), true
	}
	return strings.Join(hits, "\n"), true
}

// ── shell tool ────────────────────────────────────────────────────────────────

func (a *Agent) toolRunShell(ctx context.Context, input json.RawMessage) (string, bool) {
	var in struct{ Command string }
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	if !a.actionAllowed("shell") {
		return "permission denied: current mode does not allow shell commands", false
	}
	if a.wantsApproval("shell") && !a.autoApprovedCmds[in.Command] {
		ca, ok := a.Approver.(CommandApprover)
		if !ok {
			return "shell command approval required but no command approver is available", false
		}
		decision := ca.ApproveCommand(in.Command)
		if decision == ApproveDeny {
			return "the user rejected this shell command. Ask what they would like changed.", false
		}
		if decision == ApproveAlways {
			if a.autoApprovedCmds == nil {
				a.autoApprovedCmds = make(map[string]bool)
			}
			a.autoApprovedCmds[in.Command] = true
		}
	}

	prep, err := prepareCommand(in.Command, a.shellCwd, runtime.GOOS == "windows")
	if err != nil {
		return err.Error(), false
	}
	for _, d := range prep.mkdirs {
		abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(d))
		if err != nil {
			return "unsafe mkdir rejected: " + err.Error(), false
		}
		if err := os.MkdirAll(abs, lorefs.PublicDirMode); err != nil {
			return "mkdir failed: " + err.Error(), false
		}
	}
	a.shellCwd = prep.newCwd

	var header string
	if len(prep.notices) > 0 {
		header = "[harness] " + strings.Join(prep.notices, "; ") + "\n"
	}
	if prep.run == "" {
		cwdMsg := "working directory is now " + displayDir(a.shellCwd)
		return header + cwdMsg, true
	}

	runDirAbs := a.Dir
	if prep.runDir != "" {
		abs, err := pathutil.SafeJoin(a.Dir, filepath.FromSlash(prep.runDir))
		if err != nil {
			return "unsafe cd rejected: " + err.Error(), false
		}
		if err := os.MkdirAll(abs, lorefs.PublicDirMode); err != nil {
			return "could not create working directory: " + err.Error(), false
		}
		runDirAbs = abs
	}

	runner := shell.NewRunner(shell.Config{
		WorkDir: runDirAbs,
		Timeout: a.ShellTimeout,
		Env:     a.Env.BuildEnv(),
	})
	outCh, resCh, cancel := runner.Run(ctx, prep.run)
	defer cancel()

	for line := range outCh {
		a.emit(Event{Kind: "tool_output", Tool: "run_shell", Text: line.Text})
	}
	res := <-resCh

	out := res.Output
	if len(out) > maxToolOutput {
		out = out[:maxToolOutput/2] + "\n...[output truncated]...\n" + out[len(out)-maxToolOutput/2:]
	}

	if res.Err != nil {
		return fmt.Sprintf("%scommand failed: %v\nexit code: %d\noutput:\n%s", header, res.Err, res.ExitCode, out), false
	}
	if res.ExitCode != 0 {
		return fmt.Sprintf("%sexit code: %d\noutput:\n%s\nThe command failed — diagnose and fix the problem, then retry.", header, res.ExitCode, out), false
	}
	if strings.TrimSpace(out) == "" {
		out = "(no output)"
	}
	return fmt.Sprintf("%sexit code: 0\n%s", header, out), true
}

// ── setup_project ─────────────────────────────────────────────────────────────

func (a *Agent) toolSetupProject(ctx context.Context, input json.RawMessage) (string, bool) {
	var in struct {
		Language string
		Module   string
		Deps     []string
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	if !a.actionAllowed("write") {
		return "permission denied: current mode is read-only", false
	}
	if ok, msg := a.approveAction("setup_project", summarizeInput("setup_project", input), "setup", "setup:"+string(input)); !ok {
		return msg, false
	}

	run := func(cmdline string) (string, bool) {
		runner := shell.NewRunner(shell.Config{WorkDir: a.Dir, Timeout: 4 * time.Minute, Env: a.Env.BuildEnv()})
		outCh, resCh, cancel := runner.Run(ctx, cmdline)
		defer cancel()
		for line := range outCh {
			a.emit(Event{Kind: "tool_output", Tool: "setup_project", Text: line.Text})
		}
		res := <-resCh
		return res.Output, res.ExitCode == 0 && res.Err == nil
	}

	var b strings.Builder
	switch strings.ToLower(in.Language) {
	case "go":
		if a.Env.GoVersion == "" {
			return "Go is not installed on this machine", false
		}
		if _, err := os.Stat(filepath.Join(a.Dir, "go.mod")); os.IsNotExist(err) {
			mod := strings.TrimSpace(in.Module)
			if mod == "" {
				mod = sanitizeModuleName(filepath.Base(a.Dir))
			}
			out, ok := run("go mod init " + mod)
			fmt.Fprintf(&b, "$ go mod init %s\n%s\n", mod, out)
			if !ok {
				return b.String(), false
			}
		}
		for _, dep := range in.Deps {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			if !strings.Contains(dep, "@") {
				dep += "@latest"
			}
			out, ok := run("go get " + dep)
			fmt.Fprintf(&b, "$ go get %s\n%s\n", dep, out)
			if !ok {
				return b.String() + "\ndependency installation failed — pick a different package or fix the name.", false
			}
		}
		// `go mod tidy` before any .go files exist would PRUNE the deps that
		// were just added ("all" matches no packages). Only tidy once source
		// files are present; bootstrap-time installs stay in go.mod.
		if hasGoSources(a.Dir) {
			out, ok := run("go mod tidy")
			fmt.Fprintf(&b, "$ go mod tidy\n%s\n", out)
			if !ok {
				return b.String(), false
			}
		}
	case "node":
		if a.Env.NodeVer == "" {
			return "Node.js is not installed on this machine", false
		}
		if _, err := os.Stat(filepath.Join(a.Dir, "package.json")); os.IsNotExist(err) {
			out, ok := run("npm init -y")
			fmt.Fprintf(&b, "$ npm init -y\n%s\n", out)
			if !ok {
				return b.String(), false
			}
		}
		if len(in.Deps) > 0 {
			cmdline := "npm install " + strings.Join(in.Deps, " ")
			out, ok := run(cmdline)
			fmt.Fprintf(&b, "$ %s\n%s\n", cmdline, out)
			if !ok {
				return b.String(), false
			}
		}
	case "python":
		if len(in.Deps) > 0 {
			cmdline := "pip install " + strings.Join(in.Deps, " ")
			out, ok := run(cmdline)
			fmt.Fprintf(&b, "$ %s\n%s\n", cmdline, out)
			if !ok {
				return b.String(), false
			}
		}
	default:
		return fmt.Sprintf("unsupported language %q (go, node, python)", in.Language), false
	}

	a.markDirty()
	s := b.String()
	if len(s) > maxToolOutput {
		s = s[:maxToolOutput] + "\n...[truncated]"
	}
	return s + "\nproject setup complete", true
}

// ── verify_app ────────────────────────────────────────────────────────────────

func (a *Agent) toolVerifyApp(input json.RawMessage) (string, bool) {
	var in struct {
		Checks []verify.Check
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "invalid input JSON: " + err.Error(), false
	}
	if !a.actionAllowed("shell") {
		return "permission denied: current mode does not allow verification commands", false
	}
	if ok, msg := a.approveAction("verify_app", summarizeInput("verify_app", input), "verify", "verify:"+string(input)); !ok {
		return msg, false
	}
	if len(in.Checks) == 0 {
		return "verify_app requires at least one runtime check that exercises the artifact (cli or http)", false
	}
	for i := range in.Checks {
		c := &in.Checks[i]
		// Trivial checks prove nothing; reject them so verification cannot
		// be satisfied by `echo done`-style filler.
		cmd := strings.ToLower(strings.TrimSpace(c.Command))
		if c.Type != "http" && (cmd == "echo" || strings.HasPrefix(cmd, "echo ")) {
			return "check " + fmt.Sprint(i+1) + " is a bare echo — checks must actually exercise the artifact (run the binary, hit the server). Replace it with a real check.", false
		}
		c.Command = winifyInvocation(c.Command)
		c.StartCommand = winifyInvocation(c.StartCommand)
	}

	a.emit(Event{Kind: "verify", OK: true, Detail: "running verification: build, vet, tests, runtime checks..."})
	res := verify.Run(verify.Options{
		Dir:    a.Dir,
		Env:    a.Env.BuildEnv(),
		Checks: in.Checks,
	})
	summary := res.Summary()
	if ledger := a.writeVerifyLedger(res, in.Checks); ledger != "" {
		summary += "Verification ledger: " + ledger + "\n"
		a.emit(Event{Kind: "info", Text: "verification ledger saved to " + ledger})
	}
	a.emit(Event{Kind: "verify", OK: res.Passed, Detail: summary})

	if res.Passed {
		a.dirty = false
		a.verifiedOnce = true
	}
	return summary, res.Passed
}

func (a *Agent) writeVerifyLedger(res verify.Result, checks []verify.Check) string {
	if strings.TrimSpace(a.Dir) == "" {
		return ""
	}
	type ledgerStep struct {
		Name     string `json:"name"`
		Command  string `json:"command,omitempty"`
		Passed   bool   `json:"passed"`
		Skipped  bool   `json:"skipped"`
		Output   string `json:"output_summary,omitempty"`
		Duration string `json:"duration,omitempty"`
	}
	type ledger struct {
		Time   string         `json:"time"`
		Passed bool           `json:"passed"`
		Checks []verify.Check `json:"checks"`
		Steps  []ledgerStep   `json:"steps"`
	}
	entry := ledger{
		Time:   time.Now().Format(time.RFC3339),
		Passed: res.Passed,
		Checks: checks,
	}
	for _, s := range res.Steps {
		entry.Steps = append(entry.Steps, ledgerStep{
			Name:     s.Name,
			Command:  s.Command,
			Passed:   s.Passed,
			Skipped:  s.Skipped,
			Output:   trimLen(redactSensitive(s.Output), 2000),
			Duration: s.Duration.String(),
		})
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return ""
	}
	dir := filepath.Join(a.Dir, ".lore", "runs")
	if err := lorefs.MkdirPrivate(filepath.Join(a.Dir, ".lore")); err != nil {
		return ""
	}
	if err := lorefs.MkdirPrivate(dir); err != nil {
		return ""
	}
	name := "verify-" + time.Now().Format("20060102-150405.000000000") + ".json"
	abs := filepath.Join(dir, name)
	if err := lorefs.WritePrivate(abs, append(data, '\n')); err != nil {
		return ""
	}
	rel, err := filepath.Rel(a.Dir, abs)
	if err != nil {
		return filepath.ToSlash(filepath.Join(".lore", "runs", name))
	}
	return filepath.ToSlash(rel)
}

func redactSensitive(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "api_key") ||
			strings.Contains(lower, "apikey") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "password") ||
			strings.Contains(lower, "authorization:") {
			lines[i] = "[redacted sensitive output line]"
		}
	}
	return strings.Join(lines, "\n")
}

// winifyInvocation rewrites a leading "./prog" to ".\prog" on Windows, and
// prefixes ".\" to a bare "prog.exe" so binaries in the working directory
// resolve even when cmd.exe does not search the current directory.
func winifyInvocation(cmd string) string {
	if runtime.GOOS != "windows" {
		return cmd
	}
	cmd = strings.TrimSpace(cmd)
	if strings.HasPrefix(cmd, "./") {
		return ".\\" + cmd[2:]
	}
	first, _, _ := strings.Cut(cmd, " ")
	if strings.HasSuffix(strings.ToLower(first), ".exe") &&
		!strings.ContainsAny(first, `\/`) {
		return ".\\" + cmd
	}
	return cmd
}

func (a *Agent) approveAction(action, detail, kind, cacheKey string) (bool, string) {
	if !a.wantsApproval(kind) {
		return true, ""
	}
	if a.autoApprovedActs[cacheKey] {
		return true, ""
	}
	aa, ok := a.Approver.(ActionApprover)
	if !ok {
		return false, "approval required but no action approver is available"
	}
	decision := aa.ApproveAction(action, detail)
	if decision == ApproveDeny {
		return false, "the user rejected this action. Ask what they would like changed."
	}
	if decision == ApproveAlways {
		if a.autoApprovedActs == nil {
			a.autoApprovedActs = make(map[string]bool)
		}
		a.autoApprovedActs[cacheKey] = true
	}
	return true, ""
}

// hasGoSources reports whether any .go file exists under dir (excluding
// vendored/hidden trees).
func hasGoSources(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDir(info.Name()) || strings.HasPrefix(info.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), ".go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ── helpers ───────────────────────────────────────────────────────────────────

// relativizePath strips drive letters and leading slashes so model-supplied
// paths are always treated as relative to the project root.
func relativizePath(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(filepath.ToSlash(p)), `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	p = strings.TrimLeft(p, "/")
	return strings.TrimPrefix(p, "./")
}

func sanitizeModuleName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '/', r == '.':
			return r
		default:
			return '-'
		}
	}, s)
	if s == "" {
		s = "app"
	}
	return s
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func trimLen(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

func strconvQuote(s string) string { return fmt.Sprintf("%q", s) }

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".lore", "dist", "build", "__pycache__", ".wrangler":
		return true
	}
	return false
}

func isBinaryName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".dll", ".so", ".dylib", ".bin", ".png", ".jpg", ".jpeg", ".gif", ".ico",
		".zip", ".tar", ".gz", ".pdf", ".db", ".sqlite", ".woff", ".woff2", ".ttf":
		return true
	}
	return false
}

// goMainDir locates the main package of a Go project: the root itself, or
// the first cmd/<name> subdirectory containing `package main`. Returns a
// path usable in `go build <dir>` ("." or "./cmd/<name>"), or "".
func goMainDir(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return ""
	}
	hasMain := func(d string) bool {
		entries, err := os.ReadDir(d)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(d, e.Name()))
			if err == nil && strings.Contains(string(data), "package main") {
				return true
			}
		}
		return false
	}
	if hasMain(dir) {
		return "."
	}
	cmdDir := filepath.Join(dir, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && hasMain(filepath.Join(cmdDir, e.Name())) {
				return "./cmd/" + e.Name()
			}
		}
	}
	return ""
}

// ProjectTree renders a compact file tree of dir, capped at maxEntries.
func ProjectTree(dir string, maxEntries int) string {
	var b strings.Builder
	count := 0
	var walk func(d, prefix string, depth int)
	walk = func(d, prefix string, depth int) {
		if depth > 5 || count >= maxEntries {
			return
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].IsDir() != entries[j].IsDir() {
				return entries[i].IsDir()
			}
			return entries[i].Name() < entries[j].Name()
		})
		for _, e := range entries {
			if count >= maxEntries {
				b.WriteString(prefix + "...\n")
				return
			}
			name := e.Name()
			if e.IsDir() {
				if skipDir(name) || strings.HasPrefix(name, ".") {
					continue
				}
				b.WriteString(prefix + name + "/\n")
				count++
				walk(filepath.Join(d, name), prefix+"  ", depth+1)
			} else {
				if isBinaryName(name) {
					continue
				}
				b.WriteString(prefix + name + "\n")
				count++
			}
		}
	}
	walk(dir, "", 0)
	return b.String()
}
