//go:build !windows

package gotestrunner

import (
	"os/exec"
	"syscall"
)

func setProcessGroupAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// TerminateProcessGroup sends SIGTERM to the process group led by pid.
func TerminateProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}

// ForceKillProcessGroup sends SIGKILL to the process group led by pid.
func ForceKillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
