package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModernizePreservesAppJS(t *testing.T) {
	dir := setupModernizeTestProject(t)
	original := readFile(t, filepath.Join(dir, "src", "App.js"))

	modernizeProject(dir, false)

	after := readFile(t, filepath.Join(dir, "src", "App.js"))
	if after != original {
		t.Fatal("App.js was modified without --force")
	}
}

func TestModernizePreservesRoutes(t *testing.T) {
	dir := setupModernizeTestProject(t)
	routesDir := filepath.Join(dir, "src", "routes")

	modernizeProject(dir, false)

	if _, err := os.Stat(routesDir); os.IsNotExist(err) {
		t.Fatal("src/routes was removed without --force")
	}
}

func TestModernizePreservesPages(t *testing.T) {
	dir := setupModernizeTestProject(t)
	pagesDir := filepath.Join(dir, "src", "pages")

	modernizeProject(dir, false)

	if _, err := os.Stat(pagesDir); os.IsNotExist(err) {
		t.Fatal("src/pages was removed without --force")
	}
}

func TestModernizePreservesAssets(t *testing.T) {
	dir := setupModernizeTestProject(t)
	assetsDir := filepath.Join(dir, "src", "assets")

	modernizeProject(dir, false)

	if _, err := os.Stat(assetsDir); os.IsNotExist(err) {
		t.Fatal("src/assets was removed without --force")
	}
}

func TestModernizePreservesEslintConfig(t *testing.T) {
	dir := setupModernizeTestProject(t)
	eslintPath := filepath.Join(dir, ".eslintrc.js")

	modernizeProject(dir, false)

	if _, err := os.Stat(eslintPath); os.IsNotExist(err) {
		t.Fatal(".eslintrc.js was removed without --force")
	}
}

func TestModernizeNoDestructiveActionWithoutForce(t *testing.T) {
	dir := setupModernizeTestProject(t)

	// Snapshot all file contents before
	before := snapshotDir(t, dir)

	modernizeProject(dir, false)

	// Snapshot after
	after := snapshotDir(t, dir)

	for path, content := range before {
		afterContent, ok := after[path]
		if !ok {
			t.Fatalf("file %s was deleted without --force", path)
		}
		if afterContent != content {
			t.Fatalf("file %s was modified without --force", path)
		}
	}
}

// setupModernizeTestProject creates a project that would trigger all
// modernize paths: uninstalled npm deps, broken imports, eslint config
// with missing extends, and empty stub directories.
func setupModernizeTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// package.json with react 18 — triggers React upgrade path
	mustWriteTestFile(t, filepath.Join(dir, "package.json"), `{
  "dependencies": {"react": "^18.2.0", "react-dom": "^18.2.0"},
  "scripts": {"build": "react-scripts build"}
}`)

	// src/index.js with legacy ReactDOM.render — triggers createRoot upgrade
	mustWriteTestFile(t, filepath.Join(dir, "src", "index.js"),
		`import React from 'react';
import ReactDOM from 'react-dom';
import App from './App';
ReactDOM.render(<App />, document.getElementById('root'));
`)

	// src/App.js with broken relative import — triggers rewrite
	mustWriteTestFile(t, filepath.Join(dir, "src", "App.js"),
		`import React from 'react';
import Dashboard from './components/Dashboard';
function App() { return <Dashboard />; }
export default App;
`)

	// .eslintrc.js extending a config that is NOT in node_modules
	mustWriteTestFile(t, filepath.Join(dir, ".eslintrc.js"),
		`module.exports = { extends: ['react-app'] };`)

	// Empty stub directories
	for _, rel := range []string{"src/routes", "src/pages", "src/assets"} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		snapshot[rel] = string(data)
		return nil
	})
	return snapshot
}
