package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"lore-cli/internal/verify"
)

func TestWriteVerifyLedgerRedactsAndUsesPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	a := &Agent{Dir: dir}
	rel := a.writeVerifyLedger(verify.Result{
		Passed: false,
		Steps: []verify.Step{{
			Name:     "check",
			Command:  "print-secret",
			Passed:   false,
			Output:   "ok\nTOKEN=secret\nAuthorization: bearer abc\nstill useful",
			Duration: time.Millisecond,
		}},
	}, []verify.Check{{Type: "cli", Command: "print-secret"}})
	if rel == "" {
		t.Fatal("writeVerifyLedger returned empty path")
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "TOKEN=secret") || strings.Contains(text, "bearer abc") {
		t.Fatalf("ledger contains sensitive output:\n%s", text)
	}
	if !strings.Contains(text, "[redacted sensitive output line]") {
		t.Fatalf("ledger missing redaction marker:\n%s", text)
	}
	info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("ledger permissions = %v, want 0600", info.Mode().Perm())
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{filepath.Join(dir, ".lore"), filepath.Join(dir, ".lore", "runs")} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o700 {
				t.Fatalf("%s permissions = %v, want 0700", path, info.Mode().Perm())
			}
		}
	}
}

func TestRedactSensitive(t *testing.T) {
	out := redactSensitive("hello\npassword=hunter2\nworld")
	if strings.Contains(out, "hunter2") {
		t.Fatalf("redactSensitive leaked password: %q", out)
	}
	if !strings.Contains(out, "hello") || !strings.Contains(out, "world") {
		t.Fatalf("redactSensitive removed safe lines: %q", out)
	}
}
