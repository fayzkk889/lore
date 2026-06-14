// Package shell runs user-supplied shell commands with line-streamed
// stdout/stderr, a timeout, and external cancellation.
//
// The TUI consumes Run via Bubble Tea messages: the output channel is
// pumped one line at a time, then the result channel delivers the final
// exit status. Cancellation kills the entire process group so child
// processes (e.g. spawned by `npm`) die with their parent.
package shell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout is the wall-clock cap applied when Config.Timeout is unset.
const DefaultTimeout = 600 * time.Second

// Config configures a Runner.
type Config struct {
	WorkDir string        // project root (cwd)
	Timeout time.Duration // default 600s when zero
	Env     []string      // if non-nil, overrides parent env (use os.Environ() to inherit + extend)
}

// OutputLine is a single line of output from the running command.
type OutputLine struct {
	Text     string
	IsStderr bool
	Time     time.Time
}

// Result is delivered once after the command exits, times out, or is cancelled.
type Result struct {
	ExitCode int
	Duration time.Duration
	Output   string // full captured stdout+stderr
	Err      error  // non-nil for timeout, cancel, or non-exit failures
}

// Runner executes shell commands with the configured WorkDir/Timeout/Env.
type Runner struct {
	cfg Config
}

// NewRunner returns a Runner. A zero Timeout is replaced with DefaultTimeout.
func NewRunner(cfg Config) *Runner {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Runner{cfg: cfg}
}

// shellArgs picks the platform-appropriate shell wrapper for `command`.
func shellArgs(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}

// Run starts command and returns:
//   - outCh: each line of stdout/stderr as it arrives. Closed when the
//     command finishes OR is cancelled (use to detect EOF).
//   - resCh: receives exactly one Result, then closes.
//   - cancel: kills the process group early. Safe to call multiple times.
//
// Run never blocks the caller; all I/O happens in goroutines.
func (r *Runner) Run(ctx context.Context, command string) (<-chan OutputLine, <-chan Result, context.CancelFunc) {
	outCh := make(chan OutputLine, 64)
	resCh := make(chan Result, 1)

	// Capture full output for the final Result.
	var (
		outputMu sync.Mutex
		fullOut  []string
	)

	internalCh := make(chan OutputLine, 64)

	// Apply the runner's timeout on top of the caller's context.
	runCtx, ctxCancel := context.WithTimeout(ctx, r.cfg.Timeout)

	failEarly := func(err error) (<-chan OutputLine, <-chan Result, context.CancelFunc) {
		ctxCancel()
		close(outCh)
		resCh <- Result{ExitCode: -1, Err: err}
		close(resCh)
		return outCh, resCh, func() {}
	}

	// We deliberately use exec.Command (NOT exec.CommandContext) so the
	// only path that terminates the process is killGroup below. On Windows,
	// exec.CommandContext would race-call Process.Kill() on cmd.exe and
	// orphan grandchildren before taskkill /T can walk the tree.
	name, args := shellArgs(command)
	cmd := exec.Command(name, args...)
	cmd.Dir = r.cfg.WorkDir
	if r.cfg.Env != nil {
		cmd.Env = r.cfg.Env
	}
	setProcAttr(cmd)
	setRawCommand(cmd, command)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return failEarly(fmt.Errorf("stdout pipe: %w", err))
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return failEarly(fmt.Errorf("stderr pipe: %w", err))
	}

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return failEarly(fmt.Errorf("start: %w", err))
	}

	done := make(chan struct{})

	// Watch the context: when it expires (timeout) or is cancelled, kill
	// the entire process group. exec.CommandContext on Windows only kills
	// the direct child via TerminateProcess; grandchildren spawned by the
	// shell wrapper (e.g. `cmd /c ping ...`) would otherwise survive.
	go func() {
		select {
		case <-runCtx.Done():
			killGroup(cmd)
		case <-done:
		}
	}()

	// Two scanner goroutines feed the same internalCh.
	var wg sync.WaitGroup
	wg.Add(2)
	go scanInto(stdout, internalCh, false, &wg)
	go scanInto(stderr, internalCh, true, &wg)

	// Broadcaster goroutine: pipes internalCh to BOTH outCh (for the TUI)
	// and fullOut (for the final Result).
	go func() {
		for line := range internalCh {
			outputMu.Lock()
			fullOut = append(fullOut, line.Text)
			outputMu.Unlock()
			outCh <- line
		}
		close(outCh)
	}()

	// One reaper goroutine waits for both scanners + the process, then
	// emits Result and closes both channels.
	go func() {
		wg.Wait()
		close(internalCh) // triggers broadcaster EOF
		waitErr := cmd.Wait()

		res := Result{Duration: time.Since(start)}
		outputMu.Lock()
		res.Output = strings.Join(fullOut, "\n")
		outputMu.Unlock()
		if cmd.ProcessState != nil {
			res.ExitCode = cmd.ProcessState.ExitCode()
		} else {
			res.ExitCode = -1
		}

		// Distinguish timeout / external cancel from genuine exec errors.
		// A non-zero exit code is reflected in res.ExitCode; we only set
		// res.Err for things the caller cannot infer from ExitCode alone.
		switch {
		case runCtx.Err() == context.DeadlineExceeded:
			res.Err = fmt.Errorf("timeout after %s", r.cfg.Timeout)
		case runCtx.Err() == context.Canceled:
			res.Err = fmt.Errorf("cancelled")
		case waitErr != nil:
			if _, ok := waitErr.(*exec.ExitError); !ok {
				res.Err = waitErr
			}
		}
		close(done)
		ctxCancel()
		resCh <- res
		close(resCh)
	}()

	// User-facing cancel: cancel the context FIRST so the reaper observes
	// runCtx.Err() == Canceled (and emits a "cancelled" Result.Err) even
	// if the process dies before the reaper hits its switch. The watcher
	// goroutine above will see runCtx.Done() and call killGroup.
	cancelOnce := sync.Once{}
	userCancel := func() {
		cancelOnce.Do(func() {
			ctxCancel()
		})
	}

	return outCh, resCh, userCancel
}

// scanInto reads r line-by-line and pushes each line to outCh.
// Long lines are tolerated up to 1 MiB.
func scanInto(r io.Reader, outCh chan<- OutputLine, isStderr bool, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		outCh <- OutputLine{
			Text:     sc.Text(),
			IsStderr: isStderr,
			Time:     time.Now(),
		}
	}
}
