//go:build !windows

package shell

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts the child in a new process group so we can later kill
// the entire group (the child + any grandchildren it spawns).
func setProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// setRawCommand is a no-op on POSIX: sh -c receives the command as a single
// argv element, so no extra quoting layer exists to bypass.
func setRawCommand(_ *exec.Cmd, _ string) {}

// killGroup sends SIGKILL to the process group identified by -pid.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
