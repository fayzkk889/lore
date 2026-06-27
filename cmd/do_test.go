package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoRejectsNonexistentDirectory(t *testing.T) {
	withoutEngineConfig(t)
	cmd := newDoCmd()
	// Point --dir at a path that does not exist.
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	cmd.SetArgs([]string{"--dir", missing, "hello"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("error = %q, want it to mention 'does not exist'", err.Error())
	}
}

func TestDoRejectsFilePath(t *testing.T) {
	withoutEngineConfig(t)
	f := filepath.Join(t.TempDir(), "afile.txt")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDoCmd()
	cmd.SetArgs([]string{"--dir", f, "hello"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for file path, got nil")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q, want it to mention 'not a directory'", err.Error())
	}
}

func TestDoAcceptsExistingDirectory(t *testing.T) {
	withoutEngineConfig(t)
	dir := t.TempDir()
	cmd := newDoCmd()
	cmd.SetArgs([]string{"--dir", dir, "hello"})
	// This will fail later (no engine configured), but it should pass
	// directory validation and auto-create .lore first.
	err := cmd.Execute()
	if err == nil {
		return // unexpectedly passed (unlikely without config); that's fine
	}
	if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("existing directory rejected: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".lore")); statErr != nil {
		t.Fatalf(".lore was not created before provider resolution: %v", statErr)
	}
}

func TestDoCreatesLoreDirInsideExistingProject(t *testing.T) {
	dir := t.TempDir()
	// ensureLoreWiki is called after directory validation; verify it creates .lore/
	_, err := ensureLoreWiki(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, ".lore"))
	if err != nil {
		t.Fatalf(".lore not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".lore is not a directory")
	}
}

func withoutEngineConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	for _, name := range []string{
		"LORE_PROVIDER",
		"LORE_MODEL",
		"LORE_BASE_URL",
		"LORE_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"OPENROUTER_API_KEY",
		"DEEPSEEK_API_KEY",
	} {
		t.Setenv(name, "")
	}
}
