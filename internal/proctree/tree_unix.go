//go:build !windows

package proctree

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type sysTree struct{}

func prepare(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func (sysTree) adopt(int) {}

func (sysTree) interrupt(pid int) error { return signalGroup(pid, syscall.SIGTERM) }

func (sysTree) kill(pid int) error { return signalGroup(pid, syscall.SIGKILL) }

func (sysTree) release() {}

// alive reports whether any process of the group led by pid is left. The
// group's id is not reused while it has a member, so probing it after the
// root was reaped reaches no stranger.
func (sysTree) alive(pid int) bool {
	return !errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH)
}

// signalGroup signals the process group led by pid.
func signalGroup(pid int, sig syscall.Signal) error {
	err := syscall.Kill(-pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return os.ErrProcessDone
	}
	return err
}
