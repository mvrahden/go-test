//go:build windows

package gotestrunner

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func setProcessGroupAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// TerminateProcessGroup sends CTRL_BREAK_EVENT to the process group led by pid.
// Go translates this into os.Interrupt, allowing test binaries to run cleanup
// handlers (signal.NotifyContext, t.Cleanup, fixture AfterAll) before exiting.
func TerminateProcessGroup(pid int) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid))
}

// ForceKillProcessGroup forcibly kills the process.
func ForceKillProcessGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
