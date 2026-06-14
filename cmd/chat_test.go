package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/fayzkk889/lore/internal/agent"
	"github.com/fayzkk889/lore/internal/config"
)

func TestParsePermissionMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want agent.PermissionMode
	}{
		{"full-auto", agent.PermissionFullAuto},
		{"auto-safe", agent.PermissionAutoSafe},
		{"ask", agent.PermissionAsk},
		{"read-only", agent.PermissionReadOnly},
		{"ASK", agent.PermissionAsk},
	} {
		got, ok := parsePermissionMode(tc.in)
		if !ok {
			t.Fatalf("parsePermissionMode(%q) returned !ok", tc.in)
		}
		if got != tc.want {
			t.Fatalf("parsePermissionMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got, ok := parsePermissionMode("danger"); ok || got != "" {
		t.Fatalf("parsePermissionMode(danger) = %q, %v; want empty false", got, ok)
	}
}

func TestConfiguredPermissionMode(t *testing.T) {
	if got := configuredPermissionMode(nil); got != agent.PermissionFullAuto {
		t.Fatalf("nil config permission = %q, want full-auto", got)
	}
	if got := configuredPermissionMode(&config.Config{}); got != agent.PermissionFullAuto {
		t.Fatalf("empty config permission = %q, want full-auto", got)
	}
	if got := configuredPermissionMode(&config.Config{
		Safety: config.SafetyConfig{PermissionMode: "read-only"},
	}); got != agent.PermissionReadOnly {
		t.Fatalf("configured permission = %q, want read-only", got)
	}
	if got := configuredPermissionMode(&config.Config{
		Safety: config.SafetyConfig{PermissionMode: "surprise"},
	}); got != agent.PermissionFullAuto {
		t.Fatalf("invalid configured permission = %q, want full-auto", got)
	}
}

func TestVerificationRunsSummarizesLedger(t *testing.T) {
	dir := t.TempDir()
	runsDir := filepath.Join(dir, ".lore", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ledger := `{
  "time": "2026-06-12T23:55:00+05:30",
  "passed": true,
  "checks": [{"type":"cli","command":"app.exe"}],
  "steps": [
    {"name":"build","passed":true},
    {"name":"runtime","passed":true}
  ]
}`
	if err := os.WriteFile(filepath.Join(runsDir, "verify-20260612-235500.json"), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}

	out := verificationRuns(dir, 10)
	for _, want := range []string{"verification runs", "PASS", "checks:1", "steps:2", ".lore/runs/verify-20260612-235500.json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("verificationRuns output missing %q:\n%s", want, out)
		}
	}
}

func TestAuditTrailSummarizesRecentEntries(t *testing.T) {
	dir := t.TempDir()
	loreDir := filepath.Join(dir, ".lore")
	if err := os.MkdirAll(loreDir, 0o755); err != nil {
		t.Fatal(err)
	}
	log := strings.Join([]string{
		`{"time":"2026-06-12T23:55:01+05:30","tool":"read_file","ok":true,"input_summary":"README.md","result_summary":"12 lines"}`,
		`{"time":"2026-06-12T23:55:02+05:30","tool":"verify_app","ok":false,"input_summary":"1 check","result_summary":"failed"}`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(loreDir, "audit.jsonl"), []byte(log), 0o644); err != nil {
		t.Fatal(err)
	}

	out := auditTrail(dir, 10)
	for _, want := range []string{"tool audit", "read_file", "verify_app", "OK", "FAIL", "README.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("auditTrail output missing %q:\n%s", want, out)
		}
	}
}

func TestWikiRecallAndContextLoading(t *testing.T) {
	dir := t.TempDir()
	mustWriteTestFile(t, filepath.Join(dir, "LORE.md"), "# Local Rules\nPrefer tiny tests.\n")
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "memory.md"), "# Project Memory\n\n- The project uses SQLite for local state.\n")
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "architecture", "storage.md"), "# Storage\nSQLite keeps durable local state.\n")
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "runs", "ignored.md"), "# Ignored\nThis should not enter context.\n")

	wiki := wikiIndex(dir, 10)
	for _, want := range []string{".lore/memory.md", ".lore/architecture/storage.md", "Storage"} {
		if !strings.Contains(wiki, want) {
			t.Fatalf("wikiIndex output missing %q:\n%s", want, wiki)
		}
	}
	if strings.Contains(wiki, "ignored.md") {
		t.Fatalf("wikiIndex included runs directory:\n%s", wiki)
	}

	recall := recallWiki(dir, "sqlite", 5)
	for _, want := range []string{"recall matches", ".lore/memory.md", ".lore/architecture/storage.md"} {
		if !strings.Contains(recall, want) {
			t.Fatalf("recallWiki output missing %q:\n%s", want, recall)
		}
	}

	ctx := loadProjectContext(dir)
	for _, want := range []string{"Project instructions", ".lore/memory.md", ".lore/architecture/storage.md", "SQLite"} {
		if !strings.Contains(ctx, want) {
			t.Fatalf("loadProjectContext output missing %q:\n%s", want, ctx)
		}
	}
	if strings.Contains(ctx, "This should not enter context") {
		t.Fatalf("loadProjectContext included ignored run markdown:\n%s", ctx)
	}
}

func TestProjectStatusSummarizesLocalState(t *testing.T) {
	dir := t.TempDir()
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "memory.md"), "# Project Memory\n\n- Remember one.\n- Remember two.\n")
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "runs", "verify-1.json"), "{}\n")
	mustWriteTestFile(t, filepath.Join(dir, ".lore", "audit.jsonl"), "{}\n{}\n")

	out := projectStatus(chatModel{
		projectDir: dir,
		engineName: "test-engine",
		permission: agent.PermissionAsk,
	})
	for _, want := range []string{"engine: test-engine", "permission: ask", "memory notes: 2", "verification ledgers: 1", "audit entries: 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("projectStatus output missing %q:\n%s", want, out)
		}
	}
}

func TestAppendProjectMemoryUsesPrivatePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX permission bits reliably")
	}
	dir := t.TempDir()
	if err := appendProjectMemory(dir, "Keep this private"); err != nil {
		t.Fatal(err)
	}
	loreInfo, err := os.Stat(filepath.Join(dir, ".lore"))
	if err != nil {
		t.Fatal(err)
	}
	if loreInfo.Mode().Perm() != 0o700 {
		t.Fatalf(".lore permissions = %v, want 0700", loreInfo.Mode().Perm())
	}
	memInfo, err := os.Stat(filepath.Join(dir, ".lore", "memory.md"))
	if err != nil {
		t.Fatal(err)
	}
	if memInfo.Mode().Perm() != 0o600 {
		t.Fatalf("memory.md permissions = %v, want 0600", memInfo.Mode().Perm())
	}
}

func TestRunInitUsesPrivateLorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not report POSIX permission bits reliably")
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	flagInitModernize = false
	if err := runInit(nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(dir, ".lore"),
		filepath.Join(dir, ".lore", "architecture"),
		filepath.Join(dir, ".lore", "snapshots"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s permissions = %v, want 0700", path, info.Mode().Perm())
		}
	}
	for _, path := range []string{
		filepath.Join(dir, ".lore", "index.md"),
		filepath.Join(dir, ".lore", "log.md"),
		filepath.Join(dir, ".lore", "memory.md"),
		filepath.Join(dir, ".lore", "schema.md"),
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %v, want 0600", path, info.Mode().Perm())
		}
	}
}

func mustWriteTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
