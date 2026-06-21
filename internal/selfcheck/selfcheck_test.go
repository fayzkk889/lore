package selfcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectPythonWithPyprojectToml(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"example\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, commands := detect(dir)
	if kind != "python" {
		t.Fatalf("detect() kind = %q, want \"python\"", kind)
	}
	if len(commands) == 0 {
		t.Fatal("detect() returned no commands for Python project")
	}
	label := commands[0].cmd
	for _, a := range commands[0].args {
		label += " " + a
	}
	if !strings.Contains(label, "compileall") {
		t.Fatalf("Python command = %q, want compileall-based verification", label)
	}
}

func TestDetectPythonWithRequirementsTxt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	kind, commands := detect(dir)
	if kind != "python" {
		t.Fatalf("detect() kind = %q, want \"python\"", kind)
	}
	if len(commands) == 0 {
		t.Fatal("detect() returned no commands for Python project")
	}
	label := commands[0].cmd
	for _, a := range commands[0].args {
		label += " " + a
	}
	if !strings.Contains(label, "compileall") {
		t.Fatalf("Python command = %q, want compileall-based verification", label)
	}
}

func TestDetectPythonCommandNotBarePyCompile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, commands := detect(dir)
	if len(commands) == 0 {
		t.Fatal("detect() returned no commands")
	}
	// py_compile without args is the broken pattern — ensure it's not used
	for _, c := range commands {
		if c.cmd == "python" && len(c.args) == 2 && c.args[0] == "-m" && c.args[1] == "py_compile" {
			t.Fatal("detect() still uses bare 'python -m py_compile' without file targets")
		}
	}
}

func TestRunForFilesPyprojectOnlyChangeDoesNotSkip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Only pyproject.toml changed, no .py files in the changed list.
	// RunForFiles should NOT return Skipped: it should fall through to Run.
	result := RunForFiles(dir, []string{"pyproject.toml"}, 1)
	// We expect it to attempt verification (not skip). The command may fail
	// because python isn't installed in CI, but it must NOT be marked skipped.
	// A timeout or command failure is acceptable; skipping is not.
	if result.Skipped {
		t.Fatal("RunForFiles skipped verification when only pyproject.toml changed — should have attempted full verification")
	}
}

func TestRunForFilesRequirementsTxtOnlyChangeDoesNotSkip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := RunForFiles(dir, []string{"requirements.txt"}, 1)
	if result.Skipped {
		t.Fatal("RunForFiles skipped verification when only requirements.txt changed — should have attempted full verification")
	}
}
