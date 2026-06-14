package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvInfo captures facts about the local machine that change how the agent
// must operate: which shell commands are valid, whether C code can be
// compiled (CGO), and which toolchains are installed.
type EnvInfo struct {
	OS        string // runtime.GOOS
	Shell     string // "cmd.exe" or "sh"
	HasCC     bool   // true when a working C compiler is present
	CCName    string // compiler that answered the probe ("gcc", "clang", "cc")
	GoVersion string // e.g. "go1.25.4", empty when Go is absent
	NodeVer   string // e.g. "v22.1.0", empty when Node is absent
	PythonVer string // e.g. "Python 3.12.1", empty when Python is absent
}

// ProbeEnv inspects the local machine. The C-compiler probe actually
// compiles a one-line program, because a `gcc` on PATH that cannot link
// (missing headers, broken MinGW install) must count as "no C compiler".
func ProbeEnv() EnvInfo {
	info := EnvInfo{OS: runtime.GOOS, Shell: "sh"}
	if runtime.GOOS == "windows" {
		info.Shell = "cmd.exe"
	}

	info.HasCC, info.CCName = probeCCompiler()
	info.GoVersion = firstLine(runQuick("go", "version"))
	info.NodeVer = firstLine(runQuick("node", "--version"))
	info.PythonVer = firstLine(runQuick("python", "--version"))
	return info
}

// BuildEnv returns the environment for build/test commands: the parent
// environment plus CGO_ENABLED=0 when no C compiler is available, so CGO
// packages fail loudly at build time instead of with cryptic linker errors.
func (e EnvInfo) BuildEnv() []string {
	env := os.Environ()
	if !e.HasCC {
		env = append(env, "CGO_ENABLED=0")
	}
	return env
}

// Describe renders the environment facts for the model's context.
func (e EnvInfo) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "OS: %s; shell: %s.\n", e.OS, e.Shell)
	if e.OS == "windows" {
		b.WriteString("Commands run via `cmd /c`. NEVER use Unix-only commands (rm, cp, mv, touch, cat, ls, grep, sed, awk) or /dev/null. Use Windows equivalents (del, copy, move, type, dir) — or better, avoid shell file manipulation entirely and use the file tools.\n")
	} else {
		b.WriteString("Commands run via `sh -c`. POSIX commands are available.\n")
	}
	if e.HasCC {
		fmt.Fprintf(&b, "C compiler: available (%s). CGO-based packages can compile.\n", e.CCName)
	} else {
		b.WriteString("C compiler: NOT available. CGO is disabled (CGO_ENABLED=0). Never choose packages that require CGO; pick pure-Go alternatives (e.g. modernc.org/sqlite instead of mattn/go-sqlite3).\n")
	}
	if e.GoVersion != "" {
		fmt.Fprintf(&b, "Go: %s\n", e.GoVersion)
	}
	if e.NodeVer != "" {
		fmt.Fprintf(&b, "Node: %s\n", e.NodeVer)
	}
	if e.PythonVer != "" {
		fmt.Fprintf(&b, "Python: %s\n", e.PythonVer)
	}
	return b.String()
}

// probeCCompiler tries cc, gcc, clang in turn. A candidate counts only if it
// (a) compiles a trivial C file AND (b) targets the architecture Go builds
// for — a 32-bit MinGW gcc compiles the probe fine but cannot build
// runtime/cgo on amd64 ("cc1.exe: 64-bit mode not compiled in"), so target
// arch is checked via -dumpmachine. Results are not cached (startup only).
func probeCCompiler() (bool, string) {
	tmp, err := os.MkdirTemp("", "lore-ccprobe")
	if err != nil {
		return false, ""
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, "p.c")
	out := filepath.Join(tmp, "p.exe")
	if err := os.WriteFile(src, []byte("int main(void){return 0;}\n"), 0o644); err != nil {
		return false, ""
	}

	for _, cc := range []string{"cc", "gcc", "clang"} {
		if _, err := exec.LookPath(cc); err != nil {
			continue
		}
		if !ccTargetsGoArch(cc) {
			continue
		}
		cmd := exec.Command(cc, "-o", out, src)
		cmd.Dir = tmp
		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			continue
		}
		go func() { done <- cmd.Wait() }()
		select {
		case err := <-done:
			if err == nil {
				return true, cc
			}
		case <-time.After(20 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	return false, ""
}

// ccTargetsGoArch reports whether the compiler's default target matches the
// architecture this Go toolchain builds for (per `<cc> -dumpmachine`).
func ccTargetsGoArch(cc string) bool {
	out, err := exec.Command(cc, "-dumpmachine").Output()
	if err != nil {
		// clang/cc variants without -dumpmachine: assume compatible and let
		// the compile step decide.
		return true
	}
	machine := strings.ToLower(strings.TrimSpace(string(out)))
	switch runtime.GOARCH {
	case "amd64":
		return strings.Contains(machine, "x86_64") || strings.Contains(machine, "amd64")
	case "arm64":
		return strings.Contains(machine, "aarch64") || strings.Contains(machine, "arm64")
	case "386":
		return strings.Contains(machine, "i686") || strings.Contains(machine, "i386") ||
			strings.Contains(machine, "mingw32") || strings.Contains(machine, "x86_64")
	}
	return true
}

func runQuick(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	cmd := exec.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstLine(s string) string {
	head, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(head)
}
