package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"lore-cli/internal/display"
	"lore-cli/internal/lorefs"
	"lore-cli/internal/selfcheck"
)

// ── Cobra command ─────────────────────────────────────────────────────────────

var flagInitModernize bool

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a Lore wiki in the current directory",
		Long: `Creates the .lore/ directory structure in the current project folder.
Run this once per project before starting a chat session.`,
		RunE:         runInit,
		SilenceUsage: true,
	}
	cmd.Flags().BoolVar(&flagInitModernize, "modernize", false, "also run legacy React compatibility fixes before the baseline check")
	return cmd
}

// ── Main entry ────────────────────────────────────────────────────────────────

func runInit(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	loreDir := filepath.Join(cwd, ".lore")

	// Already initialised?
	if _, err := os.Stat(loreDir); err == nil {
		fmt.Println(display.DimStyle.Render("Already initialized — .lore/ exists."))
		return nil
	}

	// ── Create base directories ───────────────────────────────────────────────
	dirs := []string{
		loreDir,
		filepath.Join(loreDir, "architecture"),
		filepath.Join(loreDir, "entities"),
		filepath.Join(loreDir, "decisions"),
		filepath.Join(loreDir, "bugs"),
		filepath.Join(loreDir, "learnings"),
		filepath.Join(loreDir, "snapshots"),
	}
	for _, dir := range dirs {
		if err := lorefs.MkdirPrivate(dir); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	// ── Write wiki seed files ─────────────────────────────────────────────────
	check := lipgloss.NewStyle().Foreground(display.ColorGreen).Render("✓")

	seedFiles := map[string]string{
		filepath.Join(loreDir, "index.md"):  indexContent,
		filepath.Join(loreDir, "log.md"):    logContent,
		filepath.Join(loreDir, "memory.md"): memoryContent,
		filepath.Join(loreDir, "schema.md"): schemaContent,
	}
	for path, content := range seedFiles {
		if err := lorefs.WritePrivate(path, []byte(content)); err != nil {
			return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
		}
	}

	fmt.Printf("%s %s\n", check, display.SuccessStyle.Render("Initialized Lore wiki in .lore/"))

	runBaselineCheck(cwd)

	fmt.Println(display.DimStyle.Render("  Next: run `lore` and ask the agent to explore the project"))
	return nil
}

// ── Baseline build check ──────────────────────────────────────────────────────

// runBaselineCheck runs the project's build command right after wiki init.
// The status is written to .lore/config.json; the chat agent fixes broken
// builds with full verification.
func runBaselineCheck(cwd string) {
	if flagInitModernize {
		modernizeProject(cwd)
	}

	result := selfcheck.Run(cwd, 0)
	if result.Skipped {
		writeBaselineConfig(cwd, true, "")
		return
	}

	if result.Passed {
		writeBaselineConfig(cwd, true, result.Command)
		return
	}

	buildOutput := result.Output
	firstLine := buildOutput
	if i := strings.IndexByte(buildOutput, '\n'); i >= 0 {
		firstLine = buildOutput[:i]
	}
	fmt.Println()
	fmt.Printf("Found pre-existing build issues in this project.\n\n  Error: %s\n  Suggested fix: %s\n\nRun `lore` and ask the agent to fix the build — it verifies fixes automatically.\n",
		firstLine,
		buildFixSuggestion(buildOutput),
	)
	writeBaselineConfig(cwd, false, result.Command)
}

// buildFixSuggestion returns a copy-paste command based on the build error text.
func buildFixSuggestion(output string) string {
	lower := strings.ToLower(output)

	if strings.Contains(lower, "module not found") || strings.Contains(lower, "can't resolve") {
		// Extract the quoted package name that follows the error keyword.
		re := regexp.MustCompile(`['"]([@\w][\w./\-]*)['"]`)
		if m := re.FindStringSubmatch(output); m != nil {
			return "npm install " + m[1]
		}
		return "npm install <missing-package>"
	}

	if strings.Contains(lower, "failed to load config") && strings.Contains(lower, "eslint") {
		return "Delete .eslintrc.js (or .eslintrc.*) and run lore init again"
	}

	return "Fix the build error manually, then run lore init again"
}

// loreProjectConfig is the schema for .lore/config.json — a per-project
// metadata file written by lore init and updated at runtime.
type loreProjectConfig struct {
	BaselinePassed  bool   `json:"baseline_passed"`
	BaselineCommand string `json:"baseline_command,omitempty"`
	OnboardingShown bool   `json:"onboarding_shown,omitempty"`
	AutoShell       *bool  `json:"auto_shell,omitempty"`
}

// readLoreProjectConfig loads .lore/config.json from cwd.
// Returns a zero-value struct when the file is absent or unreadable.
func readLoreProjectConfig(cwd string) loreProjectConfig {
	data, err := os.ReadFile(filepath.Join(cwd, ".lore", "config.json"))
	if err != nil {
		return loreProjectConfig{}
	}
	var cfg loreProjectConfig
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// writeLoreProjectConfig persists cfg to .lore/config.json, creating the
// directory if necessary (chat can start before init in some workflows).
func writeLoreProjectConfig(cwd string, cfg loreProjectConfig) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	if err := lorefs.MkdirPrivate(filepath.Join(cwd, ".lore")); err != nil {
		return
	}
	_ = lorefs.WritePrivate(filepath.Join(cwd, ".lore", "config.json"), data)
}

// writeBaselineConfig merges the baseline build result into .lore/config.json,
// preserving any other fields already stored there.
func writeBaselineConfig(cwd string, passed bool, command string) {
	cfg := readLoreProjectConfig(cwd)
	cfg.BaselinePassed = passed
	cfg.BaselineCommand = command
	writeLoreProjectConfig(cwd, cfg)
}

// ── Project modernisation (React / ESLint compatibility) ─────────────────────

// modernizeProject performs silent, filesystem-only compatibility fixes for
// React projects after the build check passes. No model calls are made.
func modernizeProject(cwd string) {
	changed := false

	// ── 1. React 18: replace ReactDOM.render with createRoot ─────────────────
	pkgJSONPath := filepath.Join(cwd, "package.json")
	if data, err := os.ReadFile(pkgJSONPath); err == nil {
		var pkg struct {
			Dependencies    map[string]string `json:"dependencies"`
			DevDependencies map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			reactVer := pkg.Dependencies["react"]
			if reactVer == "" {
				reactVer = pkg.DevDependencies["react"]
			}
			if reactMajorVer(reactVer) >= 18 {
				indexJS := filepath.Join(cwd, "src", "index.js")
				if src, err := os.ReadFile(indexJS); err == nil {
					if updated, ok := upgradeToCreateRoot(string(src)); ok {
						_ = os.WriteFile(indexJS, []byte(updated), 0o644)
						changed = true
					}
				}
			}
		}
	}

	// ── 2. App.js: rewrite if it imports uninstalled npm packages ─────────────
	appJS := filepath.Join(cwd, "src", "App.js")
	if src, err := os.ReadFile(appJS); err == nil {
		rewrite := hasUninstalledNPMDeps(cwd, string(src))
		if !rewrite {
			if idxSrc, err := os.ReadFile(filepath.Join(cwd, "src", "index.js")); err == nil {
				rewrite = hasUninstalledNPMDeps(cwd, string(idxSrc))
			}
		}
		if rewrite {
			minimal := "import React from 'react';\nfunction App() { return <div className=\"App\"></div>; }\nexport default App;\n"
			_ = os.WriteFile(appJS, []byte(minimal), 0o644)
			for _, rel := range []string{"src/routes", "src/pages", "src/assets"} {
				_ = os.RemoveAll(filepath.Join(cwd, filepath.FromSlash(rel)))
			}
			changed = true
		}
	}

	// ── 3. App.js: rewrite if it contains relative imports that don't exist ──
	if src, err := os.ReadFile(appJS); err == nil {
		if hasBrokenImports(cwd, string(src)) {
			minimal := "import React from 'react';\nfunction App() { return <div className=\"App\"></div>; }\nexport default App;\n"
			_ = os.WriteFile(appJS, []byte(minimal), 0o644)
			changed = true
		}
	}

	// ── 4. ESLint: delete config that extends uninstalled packages ────────────
	eslintRC := filepath.Join(cwd, ".eslintrc.js")
	if src, err := os.ReadFile(eslintRC); err == nil {
		if hasUninstalledExtends(cwd, string(src)) {
			_ = os.Remove(eslintRC)
			changed = true
		}
	}

	// ── 5. Remove empty stub directories ─────────────────────────────────────
	for _, rel := range []string{"src/routes", "src/pages", "src/assets"} {
		dir := filepath.Join(cwd, filepath.FromSlash(rel))
		if empty, err := isDirEmpty(dir); err == nil && empty {
			_ = os.Remove(dir)
			changed = true
		}
	}

	if changed {
		check := lipgloss.NewStyle().Foreground(display.ColorGreen).Render("✓")
		fmt.Printf("%s %s\n", check, display.SuccessStyle.Render("Modernized project files for compatibility."))
	}
}

// reactMajorVer extracts the major version number from a semver range string
// such as "^18.2.0", "~18", or ">=17.0.1". Returns 0 if unparseable.
func reactMajorVer(ver string) int {
	re := regexp.MustCompile(`\d+`)
	if m := re.FindString(ver); m != "" {
		var n int
		fmt.Sscanf(m, "%d", &n)
		return n
	}
	return 0
}

// upgradeToCreateRoot rewrites a src/index.js from the React 17 ReactDOM.render
// API to the React 18 createRoot API. Returns the new source and true when a
// change was made; ("", false) when no change is needed.
func upgradeToCreateRoot(src string) (string, bool) {
	if !strings.Contains(src, "ReactDOM.render(") {
		return "", false
	}
	// Replace the import line.
	src = regexp.MustCompile(`import ReactDOM from ['"]react-dom['"]`).
		ReplaceAllString(src, `import ReactDOM from 'react-dom/client'`)
	// Replace the render call. The first arg is the JSX tree (greedy); the second
	// is always document.getElementById(...), so backtracking lands correctly.
	renderRe := regexp.MustCompile(`(?s)ReactDOM\.render\(([\s\S]+),\s*(document\.getElementById\([^)]+\))\s*\)`)
	src = renderRe.ReplaceAllStringFunc(src, func(match string) string {
		sub := renderRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		component := sub[1]
		container := strings.TrimSpace(sub[2])
		return fmt.Sprintf("const root = ReactDOM.createRoot(%s);\nroot.render(%s)", container, component)
	})
	return src, true
}

// hasBrokenImports returns true when src contains a relative import whose
// target does not exist on disk (checked with common JS/TS extensions).
func hasBrokenImports(cwd, src string) bool {
	re := regexp.MustCompile(`(?m)from\s+['"](\.[^'"]+)['"]`)
	appDir := filepath.Join(cwd, "src")
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		target := filepath.Join(appDir, filepath.FromSlash(m[1]))
		if !pathExistsFS(target) {
			return true
		}
	}
	return false
}

// pathExistsFS returns true if p exists on disk as-is or with any common
// JavaScript/TypeScript extension (including /index variants) appended.
func pathExistsFS(p string) bool {
	if _, err := os.Stat(p); err == nil {
		return true
	}
	for _, ext := range []string{".js", ".jsx", ".ts", ".tsx", "/index.js", "/index.jsx", "/index.ts", "/index.tsx"} {
		if _, err := os.Stat(p + ext); err == nil {
			return true
		}
	}
	return false
}

// hasUninstalledExtends returns true when .eslintrc.js extends a config whose
// eslint-config-<name> package is absent from node_modules.
func hasUninstalledExtends(cwd, src string) bool {
	// Match the value of the extends key: a string or an array.
	extendsRe := regexp.MustCompile(`(?s)extends\s*:\s*(\[.*?\]|'[^']*'|"[^"]*")`)
	m := extendsRe.FindStringSubmatch(src)
	if m == nil {
		return false
	}
	itemRe := regexp.MustCompile(`['"]([^'"]+)['"]`)
	for _, item := range itemRe.FindAllStringSubmatch(m[1], -1) {
		name := item[1]
		// Built-in eslint namespaces are always available.
		if strings.HasPrefix(name, "eslint:") || strings.HasPrefix(name, "plugin:") {
			continue
		}
		configPkg := "eslint-config-" + name
		if strings.HasPrefix(name, "eslint-config-") {
			configPkg = name
		}
		if _, err := os.Stat(filepath.Join(cwd, "node_modules", configPkg)); os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// isDirEmpty returns (true, nil) when dir exists and contains no entries.
func isDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()
	entries, _ := f.Readdirnames(1)
	return len(entries) == 0, nil
}

// hasUninstalledNPMDeps returns true when src contains a non-relative import
// whose package is absent from node_modules. 'react' and 'react-dom' are
// skipped since CRA always installs them.
func hasUninstalledNPMDeps(cwd, src string) bool {
	re := regexp.MustCompile(`(?m)from\s+['"]([^'"]+)['"]`)
	for _, m := range re.FindAllStringSubmatch(src, -1) {
		imp := m[1]
		if strings.HasPrefix(imp, "./") || strings.HasPrefix(imp, "../") {
			continue
		}
		pkg := npmPackageName(imp)
		if pkg == "react" || pkg == "react-dom" {
			continue
		}
		if _, err := os.Stat(filepath.Join(cwd, "node_modules", filepath.FromSlash(pkg))); os.IsNotExist(err) {
			return true
		}
	}
	return false
}

// npmPackageName extracts the npm package name from an import path.
// Scoped packages (@scope/name/sub) return "@scope/name".
// Regular packages (name/sub) return "name".
func npmPackageName(importPath string) string {
	if strings.HasPrefix(importPath, "@") {
		parts := strings.SplitN(importPath, "/", 3)
		if len(parts) >= 2 {
			return parts[0] + "/" + parts[1]
		}
		return importPath
	}
	if i := strings.Index(importPath, "/"); i >= 0 {
		return importPath[:i]
	}
	return importPath
}

// ── Static seed content (fallback) ───────────────────────────────────────────

const indexContent = `# Project Wiki

Initialized. Run ` + "`lore`" + ` to start building project knowledge.
`

const logContent = `# Session Log

`

const memoryContent = `# Project Memory

Persistent notes Lore should remember across sessions.
`

const schemaContent = `# Wiki Schema

SCHEMA PLACEHOLDER — define your wiki schema rules here.

## Sections
- **index.md** — high-level project overview and entry point
- **architecture/** — system design documents
- **entities/** — key domain entities and their relationships
- **decisions/** — architecture decision records (ADRs)
- **bugs/** — tracked bugs and their resolutions
- **learnings/** — insights discovered during development
- **snapshots/** — point-in-time project state captures
`
