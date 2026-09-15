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

// TerminateProcessGroup sends CTRL_BREAK_EVENT to the process group led by pid,
// while that process still runs. Go translates this into os.Interrupt, allowing
// test binaries to run cleanup handlers (signal.NotifyContext, t.Cleanup,
// fixture AfterAll) before exiting.
func TerminateProcessGroup(pid int) error {
	return signalIfRunning(pid, processRunning, func(pid int) error {
		return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(pid)) //nolint:gosec // G115: a Windows process ID is a DWORD
	})
}

// processRunning reports whether pid names a process that has not exited.
func processRunning(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid)) //nolint:gosec // G115: a Windows process ID is a DWORD
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	event, err := windows.WaitForSingleObject(h, 0)
	return err == nil && event != windows.WAIT_OBJECT_0
}

// ForceKillProcessGroup forcibly kills the process.
func ForceKillProcessGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
