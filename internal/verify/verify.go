// Package verify implements Lore's "done means it runs" pipeline. A project
// is only considered working when it builds, its tests pass, AND a runtime
// smoke test of the produced artifact succeeds. Compilation alone is never
// treated as success.
package verify

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lore-cli/internal/shell"
)

// Check is one runtime smoke check, declared by the model via the verify_app
// tool and executed entirely by the harness.
type Check struct {
	Type string `json:"type"` // "cli" | "http"

	// cli: run a command, assert exit code / output.
	Command        string `json:"command,omitempty"`
	ExpectExit     *int   `json:"expect_exit,omitempty"`     // default 0
	ExpectContains string `json:"expect_contains,omitempty"` // substring of stdout+stderr

	// http: start a server, wait for the port, issue requests, kill it.
	StartCommand string        `json:"start_command,omitempty"`
	Port         int           `json:"port,omitempty"`
	Requests     []HTTPRequest `json:"requests,omitempty"`
}

// HTTPRequest is one request issued against a started server. Redirects are
// NOT followed, so 301/302 statuses and Location headers are observable.
type HTTPRequest struct {
	Method             string            `json:"method,omitempty"` // default GET
	Path               string            `json:"path"`             // e.g. "/api/list"
	Body               string            `json:"body,omitempty"`
	ExpectStatus       int               `json:"expect_status,omitempty"` // default 200
	ExpectHeader       map[string]string `json:"expect_header,omitempty"`
	ExpectBodyContains string            `json:"expect_body_contains,omitempty"`
}

// Step is the outcome of one pipeline stage.
type Step struct {
	Name     string
	Command  string
	Passed   bool
	Skipped  bool
	Output   string
	Duration time.Duration
}

// Result aggregates all stages of one verification run.
type Result struct {
	Passed bool
	Steps  []Step
}

// Summary renders the result for feeding back to the model (and the user).
func (r Result) Summary() string {
	var b strings.Builder
	for _, s := range r.Steps {
		status := "PASS"
		if s.Skipped {
			status = "SKIP"
		} else if !s.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "[%s] %s", status, s.Name)
		if s.Command != "" {
			fmt.Fprintf(&b, " — %s", s.Command)
		}
		b.WriteString("\n")
		if !s.Passed && !s.Skipped && s.Output != "" {
			out := s.Output
			if len(out) > 4000 {
				out = out[:2000] + "\n...[truncated]...\n" + out[len(out)-2000:]
			}
			b.WriteString(indent(out) + "\n")
		}
	}
	if r.Passed {
		b.WriteString("All verification steps passed.\n")
	} else {
		b.WriteString("VERIFICATION FAILED — fix the issues above and run verify_app again.\n")
	}
	return b.String()
}

func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// Options configures a verification run.
type Options struct {
	Dir     string   // project root
	Env     []string // build environment (CGO_ENABLED handling etc.)
	Checks  []Check  // runtime smoke checks (may be empty)
	Timeout time.Duration
}

// Run executes the full pipeline: project build+tests, then runtime checks.
// It returns a Result and never an error — failures are data, not errors.
func Run(opts Options) Result {
	if opts.Timeout <= 0 {
		opts.Timeout = 300 * time.Second
	}
	var res Result
	res.Passed = true

	for _, s := range projectSteps(opts.Dir) {
		step := runStep(opts.Dir, opts.Env, s.name, s.command, opts.Timeout)
		res.Steps = append(res.Steps, step)
		if !step.Passed && !step.Skipped {
			res.Passed = false
			return res // later stages are meaningless if the build is broken
		}
	}

	for i, c := range opts.Checks {
		var step Step
		switch c.Type {
		case "http":
			step = runHTTPCheck(opts.Dir, opts.Env, c, opts.Timeout)
		default:
			step = runCLICheck(opts.Dir, opts.Env, c, opts.Timeout)
		}
		if step.Name == "" {
			step.Name = fmt.Sprintf("check %d", i+1)
		}
		res.Steps = append(res.Steps, step)
		if !step.Passed && !step.Skipped {
			res.Passed = false
		}
	}
	return res
}

// ── project build/test stages ─────────────────────────────────────────────────

type stage struct{ name, command string }

// projectSteps picks the build/vet/test commands for the project in dir.
func projectSteps(dir string) []stage {
	switch {
	case exists(dir, "go.mod"):
		steps := []stage{
			{"tidy", "go mod tidy"},
			{"build", "go build ./..."},
			{"vet", "go vet ./..."},
		}
		if hasGoTests(dir) {
			steps = append(steps, stage{"test", "go test ./..."})
		}
		return steps
	case exists(dir, "Cargo.toml"):
		return []stage{{"build", "cargo build"}, {"test", "cargo test"}}
	case exists(dir, "package.json"):
		var steps []stage
		if !exists(dir, "node_modules") {
			steps = append(steps, stage{"install", "npm install --silent"})
		}
		if hasNpmScript(dir, "build") {
			steps = append(steps, stage{"build", "npm run build"})
		}
		if hasNpmScript(dir, "test") {
			steps = append(steps, stage{"test", "npm test"})
		}
		return steps
	}
	return nil
}

func runStep(dir string, env []string, name, command string, timeout time.Duration) Step {
	runner := shell.NewRunner(shell.Config{WorkDir: dir, Timeout: timeout, Env: env})
	outCh, resCh, _ := runner.Run(context.Background(), command)
	for range outCh {
	}
	r := <-resCh
	passed := r.ExitCode == 0 && r.Err == nil
	out := r.Output
	if r.Err != nil {
		out += "\n" + r.Err.Error()
	}
	return Step{Name: name, Command: command, Passed: passed, Output: out, Duration: r.Duration}
}

// ── runtime checks ────────────────────────────────────────────────────────────

func runCLICheck(dir string, env []string, c Check, timeout time.Duration) Step {
	if strings.TrimSpace(c.Command) == "" {
		return Step{Name: "cli check", Passed: false, Output: "empty command"}
	}
	if timeout > 90*time.Second {
		timeout = 90 * time.Second
	}
	runner := shell.NewRunner(shell.Config{WorkDir: dir, Timeout: timeout, Env: env})
	outCh, resCh, _ := runner.Run(context.Background(), c.Command)
	for range outCh {
	}
	r := <-resCh

	want := 0
	if c.ExpectExit != nil {
		want = *c.ExpectExit
	}
	step := Step{
		Name:     "run: " + c.Command,
		Command:  c.Command,
		Output:   r.Output,
		Duration: r.Duration,
	}
	switch {
	case r.Err != nil && r.ExitCode != want:
		step.Output += "\n" + r.Err.Error()
		step.Passed = false
	case r.ExitCode != want:
		step.Output = fmt.Sprintf("exit code %d (expected %d)\n%s", r.ExitCode, want, r.Output)
		step.Passed = false
	case c.ExpectContains != "" && !strings.Contains(r.Output, c.ExpectContains):
		step.Output = fmt.Sprintf("output does not contain %q\n%s", c.ExpectContains, r.Output)
		step.Passed = false
	default:
		step.Passed = true
	}
	return step
}

func runHTTPCheck(dir string, env []string, c Check, timeout time.Duration) Step {
	step := Step{Name: "server: " + c.StartCommand, Command: c.StartCommand}
	if strings.TrimSpace(c.StartCommand) == "" || c.Port <= 0 {
		step.Output = "http check requires start_command and port"
		return step
	}
	if len(c.Requests) == 0 {
		step.Output = "http check requires at least one request that exercises the server"
		return step
	}

	runner := shell.NewRunner(shell.Config{WorkDir: dir, Timeout: timeout, Env: env})
	outCh, resCh, cancel := runner.Run(context.Background(), c.StartCommand)
	defer cancel()

	var serverOut strings.Builder
	go func() {
		for line := range outCh {
			if serverOut.Len() < 16*1024 {
				serverOut.WriteString(line.Text + "\n")
			}
		}
	}()

	// Wait for the port to accept connections (or the server to die).
	addr := fmt.Sprintf("127.0.0.1:%d", c.Port)
	ready := false
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case r := <-resCh:
			step.Output = fmt.Sprintf("server exited before accepting connections (exit %d)\n%s\n%s",
				r.ExitCode, r.Output, serverOut.String())
			return step
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 750*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if !ready {
		step.Output = fmt.Sprintf("port %d did not open within 45s\nserver output:\n%s", c.Port, serverOut.String())
		return step
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // surface 301/302 instead of following
		},
	}

	var b strings.Builder
	allPassed := true
	for _, req := range c.Requests {
		ok, desc := doRequest(client, c.Port, req)
		if !ok {
			allPassed = false
		}
		b.WriteString(desc + "\n")
	}

	cancel()
	<-resCh // reap

	step.Passed = allPassed
	step.Output = b.String()
	if !allPassed {
		step.Output += "\nserver output:\n" + serverOut.String()
	}
	return step
}

func doRequest(client *http.Client, port int, r HTTPRequest) (bool, string) {
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, r.Path)

	var body io.Reader
	if r.Body != "" {
		body = strings.NewReader(r.Body)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return false, fmt.Sprintf("FAIL %s %s — bad request: %v", method, r.Path, err)
	}
	if r.Body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("FAIL %s %s — %v", method, r.Path, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))

	wantStatus := r.ExpectStatus
	if wantStatus == 0 {
		wantStatus = 200
	}
	if res.StatusCode != wantStatus {
		return false, fmt.Sprintf("FAIL %s %s — status %d (expected %d); body: %s",
			method, r.Path, res.StatusCode, wantStatus, trim(string(data), 300))
	}
	for k, v := range r.ExpectHeader {
		got := res.Header.Get(k)
		if !strings.Contains(got, v) {
			return false, fmt.Sprintf("FAIL %s %s — header %s=%q (expected to contain %q)",
				method, r.Path, k, got, v)
		}
	}
	if r.ExpectBodyContains != "" && !strings.Contains(string(data), r.ExpectBodyContains) {
		return false, fmt.Sprintf("FAIL %s %s — body does not contain %q; body: %s",
			method, r.Path, r.ExpectBodyContains, trim(string(data), 300))
	}
	return true, fmt.Sprintf("PASS %s %s — status %d, body: %s", method, r.Path, res.StatusCode, trim(string(data), 200))
}

func trim(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// ── helpers ───────────────────────────────────────────────────────────────────

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func hasGoTests(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			n := info.Name()
			if n == ".git" || n == "node_modules" || n == "vendor" || n == ".lore" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func hasNpmScript(dir, script string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), `"`+script+`"`)
}
