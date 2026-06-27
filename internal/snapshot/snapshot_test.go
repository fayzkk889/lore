package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLocalSnapshotRestore(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "before")
	mustWrite(t, filepath.Join(dir, "sub", "b.txt"), "kept")

	id, warn, err := CreateSnapshot(dir)
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	if warn != "" {
		t.Fatalf("CreateSnapshot warning = %q", warn)
	}
	if id == "" {
		t.Fatal("CreateSnapshot returned empty id")
	}

	mustWrite(t, filepath.Join(dir, "a.txt"), "after")
	mustWrite(t, filepath.Join(dir, "new.txt"), "remove me")
	if err := os.Remove(filepath.Join(dir, "sub", "b.txt")); err != nil {
		t.Fatal(err)
	}

	if err := RestoreSnapshot(dir, id); err != nil {
		t.Fatalf("RestoreSnapshot error: %v", err)
	}
	if got := mustRead(t, filepath.Join(dir, "a.txt")); got != "before" {
		t.Fatalf("a.txt = %q, want before", got)
	}
	if got := mustRead(t, filepath.Join(dir, "sub", "b.txt")); got != "kept" {
		t.Fatalf("sub/b.txt = %q, want kept", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt still exists after restore")
	}
}

func TestSnapshotRestorePreservesSkippedExistingFiles(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "before")
	mustWriteBytes(t, filepath.Join(dir, "logo.png"), []byte{0x89, 'P', 'N', 'G'})
	mustWrite(t, filepath.Join(dir, ".env"), "TOKEN=secret")

	id, warn, err := CreateSnapshot(dir)
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	if warn != "" {
		t.Fatalf("CreateSnapshot warning = %q", warn)
	}

	mustWrite(t, filepath.Join(dir, "a.txt"), "after")
	mustWrite(t, filepath.Join(dir, "new.txt"), "remove me")
	mustWrite(t, filepath.Join(dir, ".env"), "TOKEN=changed")
	mustWriteBytes(t, filepath.Join(dir, "logo.png"), []byte{1, 2, 3})

	if err := RestoreSnapshot(dir, id); err != nil {
		t.Fatalf("RestoreSnapshot error: %v", err)
	}
	if got := mustRead(t, filepath.Join(dir, "a.txt")); got != "before" {
		t.Fatalf("a.txt = %q, want before", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt still exists after restore")
	}
	if got := mustRead(t, filepath.Join(dir, ".env")); got != "TOKEN=changed" {
		t.Fatalf(".env = %q, want changed secret preserved", got)
	}
	if got := mustReadBytes(t, filepath.Join(dir, "logo.png")); !bytes.Equal(got, []byte{1, 2, 3}) {
		t.Fatalf("logo.png = %v, want changed binary preserved", got)
	}
}

func TestSnapshotDoesNotCopySecrets(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, ".env"), "TOKEN=secret")
	mustWrite(t, filepath.Join(dir, "id_rsa"), "private key")
	mustWrite(t, filepath.Join(dir, "settings.json"), `{"api_key":"secret"}`)
	mustWrite(t, filepath.Join(dir, "safe.txt"), "safe")

	id, _, err := CreateSnapshot(dir)
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	root := filepath.Join(dir, ".lore", "snapshots")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var snapDir string
	for _, e := range entries {
		if e.IsDir() && filepath.Base(e.Name()) != "" && len(id) <= len(e.Name()) && e.Name()[:len(id)] == id {
			snapDir = filepath.Join(root, e.Name())
			break
		}
	}
	if snapDir == "" {
		t.Fatalf("snapshot %q not found", id)
	}
	for _, rel := range []string{".env", "id_rsa", "settings.json"} {
		if _, err := os.Stat(filepath.Join(snapDir, "files", rel)); !os.IsNotExist(err) {
			t.Fatalf("secret %s was copied into snapshot", rel)
		}
	}
	if got := mustRead(t, filepath.Join(snapDir, "files", "safe.txt")); got != "safe" {
		t.Fatalf("safe.txt = %q, want safe", got)
	}
}

func TestSnapshotMetadataUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX permission bits reliably")
	}
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "safe.txt"), "safe")

	id, _, err := CreateSnapshot(dir)
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	root := filepath.Join(dir, ".lore", "snapshots")
	var snapDir string
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), id) {
			snapDir = filepath.Join(root, e.Name())
			break
		}
	}
	if snapDir == "" {
		t.Fatalf("snapshot %q not found", id)
	}
	for _, path := range []string{filepath.Join(dir, ".lore"), root, snapDir, filepath.Join(snapDir, "files")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s permissions = %v, want 0700", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(snapDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("manifest permissions = %v, want 0600", info.Mode().Perm())
	}
}

func TestSnapshotSkipsLargeProjectInsteadOfCopying(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i <= maxSnapshotFiles; i++ {
		mustWrite(t, filepath.Join(dir, "files", fmt.Sprintf("file-%04d.txt", i)), "x")
	}

	id, warn, err := CreateSnapshot(dir)
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	if id != "" {
		t.Fatalf("CreateSnapshot id = %q, want empty when project exceeds budget", id)
	}
	if !strings.Contains(warn, "snapshot skipped") {
		t.Fatalf("CreateSnapshot warning = %q, want snapshot skipped warning", warn)
	}
}

func TestLocalSnapshotShortIDIncludesTimeAndRandomPrefix(t *testing.T) {
	id := "20260613123456-abcdef12"
	if got := shortID(id); got != "20260613123456-abcd" {
		t.Fatalf("shortID(%q) = %q, want timestamp plus random prefix", id, got)
	}
}

func TestRestoreLocalSnapshotAmbiguousPrefixDoesNotFallBackToGit(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, ".lore", "snapshots")
	for _, id := range []string{"20260613123456-aaaa1111", "20260613123456-bbbb2222"} {
		snapDir := filepath.Join(root, id)
		if err := os.MkdirAll(filepath.Join(snapDir, "files"), 0o755); err != nil {
			t.Fatal(err)
		}
		m := manifest{
			ID:        id,
			Timestamp: time.Now(),
			Message:   "test",
		}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(snapDir, "manifest.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := RestoreSnapshot(dir, "20260613123456")
	if err == nil {
		t.Fatal("RestoreSnapshot with ambiguous local prefix succeeded")
	}
	if !strings.Contains(err.Error(), "multiple local snapshots") {
		t.Fatalf("RestoreSnapshot error = %v, want ambiguous local snapshot error", err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustWriteBytes(t, path, []byte(content))
}

func mustWriteBytes(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustReadBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
