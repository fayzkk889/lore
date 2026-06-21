package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoRejectsNonexistentDirectory(t *testing.T) {
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
	dir := t.TempDir()
	cmd := newDoCmd()
	cmd.SetArgs([]string{"--dir", dir, "hello"})
	// This will fail later (no engine configured), but it should pass
	// the directory validation step. Check that the error is NOT about
	// the directory.
	err := cmd.Execute()
	if err == nil {
		return // unexpectedly passed (unlikely without config); that's fine
	}
	if strings.Contains(err.Error(), "does not exist") || strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("existing directory rejected: %v", err)
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
