package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// prepareCommand rewrites a model-supplied shell command so it runs safely
// and correctly on this machine:
//
//   - Unix-only commands are translated to cmd.exe equivalents on Windows
//     (rm→del/rmdir, cp→copy, mv→move, touch, cat→type, ls→dir).
//   - `cd`/`mkdir` arguments are forced inside the project root: absolute
//     paths and drive letters are relativized, ".." escapes are rejected.
//   - A leading bare `cd <dir>` (whole command) persists: the returned
//     newCwd becomes the working directory for subsequent commands.
//
// cwd is the current working directory relative to the project root ("" =
// root). The returned run string may be empty when the command was only a
// cd/mkdir that prepareCommand fully handled.
type preparedCommand struct {
	run     string   // command line to execute ("" = nothing left to run)
	runDir  string   // directory relative to root in which to run it
	newCwd  string   // persisted cwd after this command
	mkdirs  []string // directories (relative to root) to create before running
	notices []string // human-readable rewrites applied, reported to the model
}

func prepareCommand(cmdLine, cwd string, isWindows bool) (preparedCommand, error) {
	p := preparedCommand{runDir: cwd, newCwd: cwd}

	cmdLine = strings.TrimSpace(cmdLine)
	if cmdLine == "" {
		return p, fmt.Errorf("empty command")
	}

	segs := splitOnChain(cmdLine)

	// Leading cd/mkdir segments are interpreted by the harness so that
	// path safety is guaranteed; everything after runs in one shell.
	i := 0
	for i < len(segs) {
		seg := strings.TrimSpace(segs[i])
		switch {
		case seg == "cd" || strings.HasPrefix(seg, "cd "):
			target := strings.Trim(strings.TrimSpace(strings.TrimPrefix(seg, "cd")), `"'`)
			next, err := resolveRelDir(p.runDir, target)
			if err != nil {
				return p, err
			}
			if next != p.runDir {
				p.notices = appendIfRewritten(p.notices, seg, "cd "+displayDir(next))
			}
			p.runDir = next
			i++
		case seg == "mkdir" || strings.HasPrefix(seg, "mkdir "):
			for _, d := range strings.Fields(seg)[1:] {
				if d == "-p" || strings.HasPrefix(d, "-") {
					continue
				}
				rel, err := resolveRelDir(p.runDir, strings.Trim(d, `"'`))
				if err != nil {
					return p, err
				}
				if rel != "" {
					p.mkdirs = append(p.mkdirs, rel)
				}
			}
			i++
		default:
			goto rest
		}
	}
rest:

	// A command that was nothing but cd (and mkdir) persists the directory.
	if i >= len(segs) {
		p.newCwd = p.runDir
		return p, nil
	}

	var remaining []string
	for _, seg := range segs[i:] {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if isWindows {
			out, notice := winTranslate(seg)
			if notice != "" {
				p.notices = append(p.notices, notice)
			}
			if out == "" {
				continue
			}
			seg = out
		}
		remaining = append(remaining, seg)
	}
	if len(remaining) == 0 {
		p.newCwd = p.runDir
		return p, nil
	}
	p.run = strings.Join(remaining, " && ")

	if isWindows {
		p.run = strings.ReplaceAll(p.run, " 2>/dev/null", "")
		p.run = strings.ReplaceAll(p.run, " >/dev/null", "")
	}
	return p, nil
}

// splitOnChain splits a command line on "&&" while respecting double quotes.
func splitOnChain(s string) []string {
	var segs []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
		}
		if !inQuote && c == '&' && i+1 < len(s) && s[i+1] == '&' {
			segs = append(segs, cur.String())
			cur.Reset()
			i++
			continue
		}
		cur.WriteByte(c)
	}
	segs = append(segs, cur.String())
	return segs
}

// resolveRelDir resolves target against base (both relative to project root)
// and guarantees the result stays inside the root. Absolute paths and drive
// letters are stripped to project-relative form.
func resolveRelDir(base, target string) (string, error) {
	t := strings.TrimSpace(target)
	if t == "" || t == "." {
		return base, nil
	}
	t = filepath.ToSlash(t)
	// Strip drive letter (C:/foo) and leading slashes — treat as root-relative.
	if len(t) >= 2 && t[1] == ':' {
		t = t[2:]
	}
	wasAbs := strings.HasPrefix(t, "/")
	t = strings.TrimLeft(t, "/")

	start := base
	if wasAbs {
		start = "" // absolute paths are re-rooted at the project root
	}
	joined := filepath.ToSlash(filepath.Join(start, t))
	if joined == "." {
		joined = ""
	}
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", fmt.Errorf("cd/mkdir outside the project root is not allowed: %q", target)
	}
	return joined, nil
}

func displayDir(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

func appendIfRewritten(notices []string, from, to string) []string {
	if strings.TrimSpace(from) == to {
		return notices
	}
	return append(notices, fmt.Sprintf("rewrote %q -> %q", strings.TrimSpace(from), to))
}

// winTranslate converts a single Unix-style command segment to its cmd.exe
// equivalent. Returns the rewritten segment ("" = drop it) and an optional
// notice describing the rewrite.
func winTranslate(seg string) (string, string) {
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return "", ""
	}
	args := fields[1:]

	// strip common Unix flags that have no cmd.exe meaning
	stripFlags := func(args []string) []string {
		var out []string
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				continue
			}
			out = append(out, winPath(a))
		}
		return out
	}

	switch fields[0] {
	case "rm":
		rest := stripFlags(args)
		if len(rest) == 0 {
			return "", "dropped bare rm"
		}
		recursive := strings.Contains(seg, "-r")
		if recursive {
			return "rmdir /s /q " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
		}
		return "del /q " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "cp":
		rest := stripFlags(args)
		if len(rest) < 2 {
			return "", "dropped malformed cp"
		}
		if strings.Contains(seg, "-r") {
			return "xcopy /e /i /y " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
		}
		return "copy /y " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "mv":
		rest := stripFlags(args)
		if len(rest) < 2 {
			return "", "dropped malformed mv"
		}
		return "move /y " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "touch":
		rest := stripFlags(args)
		if len(rest) == 0 {
			return "", "dropped bare touch"
		}
		var parts []string
		for _, f := range rest {
			parts = append(parts, "type nul > "+f)
		}
		return strings.Join(parts, " && "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "cat":
		rest := stripFlags(args)
		if len(rest) == 0 {
			return "", "dropped bare cat"
		}
		return "type " + strings.Join(rest, " "), fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "ls":
		return "dir", fmt.Sprintf("rewrote %q for cmd.exe", seg)
	case "which":
		if len(args) == 1 {
			return "where " + args[0], fmt.Sprintf("rewrote %q for cmd.exe", seg)
		}
	}

	// Bare "prog.exe args" → ".\prog.exe args": some Windows setups do not
	// search the working directory for executables.
	if strings.HasSuffix(strings.ToLower(fields[0]), ".exe") &&
		!strings.ContainsAny(fields[0], `\/`) {
		return ".\\" + seg, ""
	}
	return seg, ""
}

// winPath converts forward slashes to backslashes for cmd.exe built-ins.
func winPath(p string) string {
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	return strings.ReplaceAll(p, "/", "\\")
}
