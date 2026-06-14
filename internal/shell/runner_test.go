package shell

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

// drain reads outCh until closed and returns the lines.
func drain(t *testing.T, outCh <-chan OutputLine) []OutputLine {
	t.Helper()
	var lines []OutputLine
	for line := range outCh {
		lines = append(lines, line)
	}
	return lines
}

// sleepCmd returns a shell command that sleeps for `seconds` seconds.
func sleepCmd(seconds int) string {
	if runtime.GOOS == "windows" {
		// `ping -n N+1 127.0.0.1 >nul` delays roughly N seconds and works
		// non-interactively (timeout /t hangs without a console).
		return "ping -n " + itoa(seconds+1) + " 127.0.0.1 >nul"
	}
	return "sleep " + itoa(seconds)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestRun_EchoSucceeds(t *testing.T) {
	r := NewRunner(Config{Timeout: 5 * time.Second})
	outCh, resCh, _ := r.Run(context.Background(), "echo hello")

	lines := drain(t, outCh)
	res := <-resCh

	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.ExitCode)
	}
	if len(lines) == 0 {
		t.Fatalf("got no output lines")
	}
	got := strings.TrimSpace(lines[0].Text)
	if got != "hello" {
		t.Fatalf("first line = %q, want %q", got, "hello")
	}
	if lines[0].IsStderr {
		t.Fatalf("first line marked as stderr")
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	r := NewRunner(Config{Timeout: 5 * time.Second})
	cmd := "exit 7"
	if runtime.GOOS == "windows" {
		cmd = "exit /b 7"
	}
	outCh, resCh, _ := r.Run(context.Background(), cmd)
	drain(t, outCh)
	res := <-resCh

	if res.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", res.ExitCode)
	}
	// Non-zero exit is not an Err — caller distinguishes via ExitCode.
	if res.Err != nil {
		t.Fatalf("Err set for non-zero exit: %v", res.Err)
	}
}

func TestRun_Timeout(t *testing.T) {
	r := NewRunner(Config{Timeout: 500 * time.Millisecond})
	start := time.Now()
	outCh, resCh, _ := r.Run(context.Background(), sleepCmd(5))
	drain(t, outCh)
	res := <-resCh
	elapsed := time.Since(start)

	if res.Err == nil || !strings.Contains(res.Err.Error(), "timeout") {
		t.Fatalf("expected timeout err, got %v", res.Err)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("timeout did not kill process: elapsed=%s", elapsed)
	}
}

func TestRun_StreamingOutput(t *testing.T) {
	// Multi-line output should arrive line-by-line.
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo line1 & echo line2 & echo line3"
	} else {
		cmd = "echo line1; echo line2; echo line3"
	}
	r := NewRunner(Config{Timeout: 5 * time.Second})
	outCh, resCh, _ := r.Run(context.Background(), cmd)

	var got []string
	for line := range outCh {
		got = append(got, strings.TrimSpace(line.Text))
	}
	res := <-resCh
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if len(got) < 3 {
		t.Fatalf("expected >=3 lines, got %d: %v", len(got), got)
	}
	want := []string{"line1", "line2", "line3"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
}

func TestRun_Cancel(t *testing.T) {
	r := NewRunner(Config{Timeout: 30 * time.Second})
	start := time.Now()
	outCh, resCh, cancel := r.Run(context.Background(), sleepCmd(10))

	// Cancel after a short delay.
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	drain(t, outCh)
	res := <-resCh
	elapsed := time.Since(start)

	if res.Err == nil {
		t.Fatalf("expected cancel err, got nil")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("cancel did not kill process promptly: elapsed=%s", elapsed)
	}
}

func TestRun_Stderr(t *testing.T) {
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "echo err 1>&2"
	} else {
		cmd = "echo err >&2"
	}
	r := NewRunner(Config{Timeout: 5 * time.Second})
	outCh, resCh, _ := r.Run(context.Background(), cmd)
	lines := drain(t, outCh)
	<-resCh

	if len(lines) == 0 {
		t.Fatalf("no lines captured")
	}
	if !lines[0].IsStderr {
		t.Fatalf("expected IsStderr=true, got false. text=%q", lines[0].Text)
	}
	if !strings.Contains(lines[0].Text, "err") {
		t.Fatalf("stderr line = %q, want contains 'err'", lines[0].Text)
	}
}

func TestRun_WorkDir(t *testing.T) {
	dir := t.TempDir()
	var cmd string
	if runtime.GOOS == "windows" {
		cmd = "cd"
	} else {
		cmd = "pwd"
	}
	r := NewRunner(Config{Timeout: 5 * time.Second, WorkDir: dir})
	outCh, resCh, _ := r.Run(context.Background(), cmd)
	lines := drain(t, outCh)
	res := <-resCh
	if res.Err != nil {
		t.Fatalf("unexpected err: %v", res.Err)
	}
	if len(lines) == 0 {
		t.Fatalf("no output")
	}
	got := strings.TrimSpace(lines[0].Text)
	// macOS resolves symlinks (/tmp -> /private/tmp), so check suffix containment loosely.
	if !strings.Contains(got, strings.TrimPrefix(dir, "/private")) && !strings.EqualFold(got, dir) {
		t.Fatalf("WorkDir not respected: pwd=%q want=%q", got, dir)
	}
}
