//go:build windows

package gotestrunner

import (
	"os"
	"os/exec"
	"syscall"
)

func setProcessGroupAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// TerminateProcessGroup kills the process group led by pid.
// On Windows there is no SIGTERM, so the process is killed directly.
func TerminateProcessGroup(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}

// ForceKillProcessGroup forcibly kills the process group led by pid.
func ForceKillProcessGroup(pid int) error {
	return TerminateProcessGroup(pid)
}
