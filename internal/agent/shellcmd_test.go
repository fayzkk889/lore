package agent

import "testing"

func TestPrepareCommandBareCdPersists(t *testing.T) {
	p, err := prepareCommand("cd backend", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.newCwd != "backend" {
		t.Fatalf("newCwd = %q, want backend", p.newCwd)
	}
	if p.run != "" {
		t.Fatalf("run = %q, want empty", p.run)
	}
}

func TestPrepareCommandChainedCdDoesNotPersist(t *testing.T) {
	p, err := prepareCommand("cd backend && go build", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.newCwd != "" {
		t.Fatalf("newCwd = %q, want root", p.newCwd)
	}
	if p.runDir != "backend" {
		t.Fatalf("runDir = %q, want backend", p.runDir)
	}
	if p.run != "go build" {
		t.Fatalf("run = %q, want 'go build'", p.run)
	}
}

func TestPrepareCommandRejectsEscape(t *testing.T) {
	if _, err := prepareCommand("cd ..\\..\\outside && dir", "", true); err == nil {
		t.Fatal("expected error for cd outside project root")
	}
}

func TestPrepareCommandRelativizesAbsolute(t *testing.T) {
	p, err := prepareCommand(`cd C:\evil\path && dir`, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if p.runDir != "evil/path" {
		t.Fatalf("runDir = %q, want evil/path (re-rooted)", p.runDir)
	}
}

func TestPrepareCommandMkdir(t *testing.T) {
	p, err := prepareCommand("mkdir -p cmd internal/storage && dir", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.mkdirs) != 2 || p.mkdirs[0] != "cmd" || p.mkdirs[1] != "internal/storage" {
		t.Fatalf("mkdirs = %v", p.mkdirs)
	}
	if p.run != "dir" {
		t.Fatalf("run = %q", p.run)
	}
}

func TestPrepareCommandWindowsTranslation(t *testing.T) {
	cases := map[string]string{
		"rm -rf node_modules":  "rmdir /s /q node_modules",
		"rm file.txt":          "del /q file.txt",
		"cat hello.txt":        "type hello.txt",
		"ls":                   "dir",
		"mv a.txt b.txt":       "move /y a.txt b.txt",
		"touch newfile.go":     "type nul > newfile.go",
		"cp -r src/dist out":   "xcopy /e /i /y src\\dist out",
	}
	for in, want := range cases {
		p, err := prepareCommand(in, "", true)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if p.run != want {
			t.Errorf("prepare(%q) = %q, want %q", in, p.run, want)
		}
	}
}

func TestPrepareCommandUnixUntouched(t *testing.T) {
	p, err := prepareCommand("rm -rf node_modules && ls", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if p.run != "rm -rf node_modules && ls" {
		t.Fatalf("run = %q — unix commands must pass through", p.run)
	}
}

func TestRelativizePath(t *testing.T) {
	cases := map[string]string{
		`C:\Users\x\project\main.go`: "Users/x/project/main.go",
		"/etc/passwd":                "etc/passwd",
		"./cmd/root.go":              "cmd/root.go",
		"src/app.js":                 "src/app.js",
	}
	for in, want := range cases {
		if got := relativizePath(in); got != want {
			t.Errorf("relativizePath(%q) = %q, want %q", in, got, want)
		}
	}
}
