// Package selfcheck detects the project type and runs the appropriate
// build/check command to verify that applied changes haven't broken anything.
package selfcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Result is returned by Run after the check command completes.
type Result struct {
	// Skipped is true when no recognisable project type was found or when
	// the project type has no build step to verify (e.g. static HTML).
	Skipped bool
	// SkipReason is a human-readable explanation shown when Skipped is true.
	// Empty means "no project type detected".
	SkipReason string
	// Command is the human-readable command that was (or would have been) run.
	Command string
	// Passed is true when the command exited with code 0.
	Passed bool
	// Output is the combined stdout+stderr from the command.
	Output string
	// Duration is how long the command took.
	Duration time.Duration
}

// DefaultTimeout is the default per-command timeout for self-check.
// It is generous enough to accommodate larger projects (npm installs,
// Go builds with many deps, Rust compiles).
var DefaultTimeout = 180 * time.Second

// Run detects the project type in dir and executes the appropriate check.
// It never returns an error — failures are reported inside Result.
// A timeout of 0 selects DefaultTimeout.
func Run(dir string, timeout time.Duration) Result {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	kind, commands := detect(dir)
	if kind == "" {
		if exists(dir, "index.html") {
			return Result{Skipped: true, SkipReason: "Static web project — no build step needed"}
		}
		return Result{Skipped: true}
	}
	if len(commands) == 0 {
		return Result{Skipped: true, SkipReason: "Static web project — no build step needed"}
	}

	var totalOutput bytes.Buffer
	var totalDur time.Duration
	var lastLabel string

	for _, c := range commands {
		label := c.cmd
		for _, a := range c.args {
			label += " " + a
		}
		lastLabel = label

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		ex := exec.CommandContext(ctx, c.cmd, c.args...)
		ex.Dir = dir

		var buf bytes.Buffer
		ex.Stdout = &buf
		ex.Stderr = &buf

		start := time.Now()
		err := ex.Run()
		dur := time.Since(start)
		totalDur += dur
		cancel()

		totalOutput.WriteString(fmt.Sprintf("$ %s\n", label))
		totalOutput.Write(buf.Bytes())
		totalOutput.WriteString("\n")

		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				totalOutput.WriteString(fmt.Sprintf("timed out after %s\n", timeout))
			}
			return Result{
				Command:  label,
				Passed:   false,
				Output:   filterBuildOutput(totalOutput.String()),
				Duration: totalDur,
			}
		}
	}

	if r := checkLegacyReactDOM(dir); r != nil {
		return *r
	}

	return Result{
		Command:  lastLabel,
		Passed:   true,
		Output:   filterBuildOutput(totalOutput.String()),
		Duration: totalDur,
	}
}

// detect returns the project kind, executable, and arguments to run for dir.
// Returns empty strings when the project type cannot be determined.
type command struct {
	cmd  string
	args []string
}

func detect(dir string) (kind string, commands []command) {
	switch {
	case exists(dir, "go.mod"):
		cmds := []command{
			{"go", []string{"mod", "tidy"}},
			{"go", []string{"build", "./..."}},
		}
		// Run tests too, so failing tests (logic/runtime bugs that still
		// compile) are caught and fed back to the auto-fix loop. Only added
		// when test files exist, so projects without tests are not forced to
		// have them.
		if hasGoTests(dir) {
			cmds = append(cmds, command{"go", []string{"test", "./..."}})
		}
		return "go", cmds

	case exists(dir, "Cargo.toml"):
		return "rust", []command{
			{"cargo", []string{"check"}},
		}

	case exists(dir, "package.json"):
		if !hasBuildScript(dir) {
			// No build script, but check if there's a main JS file we can syntax-check.
			// This catches Express backends and similar Node.js servers.
			entryFile := detectNodeEntry(dir)
			if entryFile != "" {
				return "node-server", []command{
					{"node", []string{"--check", entryFile}},
				}
			}
			return "static-web", nil
		}
		var cmds []command
		if _, err := os.Stat(filepath.Join(dir, "node_modules")); os.IsNotExist(err) {
			cmds = append(cmds, command{"npm", []string{"install", "--silent"}})
		}
		cmds = append(cmds, command{"npm", []string{"run", "build"}})
		return "node", cmds

	case exists(dir, "pyproject.toml"):
		return "python", []command{
			{"python", []string{"-m", "py_compile"}},
		}

	case exists(dir, "requirements.txt"):
		return "python", []command{
			{"python", []string{"-m", "py_compile"}},
		}

	case exists(dir, "Makefile"):
		if makeTargetExists(dir, "check") {
			return "make", []command{{"make", []string{"check"}}}
		}
		if makeTargetExists(dir, "build") {
			return "make", []command{{"make", []string{"build"}}}
		}

	case exists(dir, "index.html"):
		return "static-web", nil
	}
	return "", nil
}

// RunMultiDir detects project types in the root AND its immediate subdirectories.
// It runs self-check for each detected project and returns the first failure,
// or the last success if all pass.
func RunMultiDir(dir string, timeout time.Duration) Result {
	// Read entries once — reused for both sub-project detection and scanning.
	entries, err := os.ReadDir(dir)

	rootResult := Run(dir, timeout)

	// Before returning the root result, check whether any immediate subdirectory
	// contains a real project type. Multi-dir layouts (frontend/ + backend/) should
	// take priority over a root package.json that has no build script or node_modules.
	hasSubProjects := false
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" {
				continue
			}
			subDir := filepath.Join(dir, name)
			subKind, _ := detect(subDir)
			if subKind != "" && subKind != "static-web" {
				hasSubProjects = true
				break
			}
		}
	}

	// Single-project layout: root found a project and no subdirectories have their own.
	if !rootResult.Skipped && !hasSubProjects {
		return rootResult
	}

	// Root had no project type, or subdirectories have their own projects —
	// scan immediate subdirectories.
	if err != nil {
		return rootResult
	}

	var lastResult Result
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden dirs and common non-project dirs
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		subDir := filepath.Join(dir, name)
		subResult := Run(subDir, timeout)
		if subResult.Skipped {
			continue
		}
		found = true
		// Prefix the command with the subdirectory name for clarity
		subResult.Command = name + ": " + subResult.Command
		if !subResult.Passed {
			return subResult // return first failure
		}
		lastResult = subResult
	}

	if !found {
		return rootResult // no project types found anywhere
	}
	return lastResult
}

// RunForFiles is like Run but restricts Python compilation to the listed files.
// A timeout of 0 selects DefaultTimeout.
func RunForFiles(dir string, changedFiles []string, timeout time.Duration) Result {
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	if !exists(dir, "pyproject.toml") && !exists(dir, "requirements.txt") {
		return Run(dir, timeout)
	}
	// Collect only .py files from the changed list.
	var pyFiles []string
	for _, f := range changedFiles {
		if filepath.Ext(f) == ".py" {
			pyFiles = append(pyFiles, f)
		}
	}
	if len(pyFiles) == 0 {
		return Result{Skipped: true}
	}
	args := append([]string{"-m", "py_compile"}, pyFiles...)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := exec.CommandContext(ctx, "python", args...)
	c.Dir = dir

	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	label := "python -m py_compile <changed .py files>"
	start := time.Now()
	err := c.Run()
	dur := time.Since(start)

	if err != nil {
		return Result{Command: label, Passed: false, Output: buf.String(), Duration: dur}
	}
	return Result{Command: label, Passed: true, Output: buf.String(), Duration: dur}
}

// ── helpers ───────────────────────────────────────────────────────────────────

// hasGoTests reports whether dir or any subdirectory contains a _test.go file.
func hasGoTests(dir string) bool {
	found := false
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "vendor" || name == ".lore" {
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

func exists(dir, file string) bool {
	_, err := os.Stat(filepath.Join(dir, file))
	return err == nil
}

// hasBuildScript returns true when package.json contains a "build" script.
func hasBuildScript(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	return bytes.Contains(data, []byte(`"build"`))
}

// detectNodeEntry finds the main entry point for a Node.js project without a build script.
// It checks package.json "main" field, then common filenames.
func detectNodeEntry(dir string) string {
	// Check package.json "main" field
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err == nil {
		var pkg struct {
			Main string `json:"main"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Main != "" {
			candidate := filepath.Join(dir, pkg.Main)
			if _, err := os.Stat(candidate); err == nil {
				return pkg.Main
			}
		}
	}
	// Fall back to common entry points
	for _, name := range []string{"server.js", "index.js", "app.js", "main.js"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name
		}
	}
	return ""
}

// filterBuildOutput strips ESLint warning-only lines from build output,
// keeping lines that mention actual errors.
func filterBuildOutput(output string) string {
	var filtered []string
	for _, line := range strings.Split(output, "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "warning") && !strings.Contains(lower, "error") {
			continue
		}
		if strings.Contains(lower, " warnings") && !strings.Contains(lower, " errors") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

// isReact18Plus returns true when package.json declares react at v18 or v19.
func isReact18Plus(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	ver := pkg.Dependencies["react"]
	if ver == "" {
		ver = pkg.DevDependencies["react"]
	}
	ver = strings.TrimLeft(ver, "^~>=v ")
	return strings.HasPrefix(ver, "18") || strings.HasPrefix(ver, "19")
}

// checkLegacyReactDOM scans src/ .js/.jsx files for deprecated ReactDOM.render()
// usage when the project uses React 18+. Returns a failing Result if found.
func checkLegacyReactDOM(dir string) *Result {
	if !isReact18Plus(dir) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(dir, "src"))
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(name)
		if ext != ".js" && ext != ".jsx" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "src", name))
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "ReactDOM.render(") || strings.Contains(content, "require('react-dom').render") {
			return &Result{
				Passed:  false,
				Command: "react18-compat-check",
				Output:  "Found legacy ReactDOM.render() — React 18 projects need createRoot. Update the render call.",
			}
		}
	}
	return nil
}

// makeTargetExists returns true when the Makefile defines the given target.
func makeTargetExists(dir, target string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "Makefile"))
	if err != nil {
		return false
	}
	// Simple heuristic: look for "target:" at the start of a line.
	needle := []byte(target + ":")
	for _, line := range bytes.Split(data, []byte("\n")) {
		if bytes.HasPrefix(bytes.TrimSpace(line), needle) {
			return true
		}
	}
	return false
}
