//go:build windows

package shell

import (
	"os/exec"
	"strconv"
	"syscall"
)

// setProcAttr puts the child in a new console process group so its children
// can be terminated together via taskkill /T.
func setProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// setRawCommand bypasses Go's argv quoting and hands cmd.exe the command
// line verbatim. Go escapes embedded double quotes as \" — cmd.exe does not
// understand backslash escaping, so quoted arguments (`app add "two words"`)
// were split incorrectly. `cmd /s /c "<line>"` is the canonical form: /s
// strips the outer quotes and the rest is parsed exactly as typed.
func setRawCommand(cmd *exec.Cmd, command string) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = `cmd /s /c "` + command + `"`
}

// killGroup terminates the process tree rooted at cmd.Process via taskkill.
func killGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	kill := exec.Command("taskkill", "/F", "/T", "/PID", pid)
	_ = kill.Run()
}
