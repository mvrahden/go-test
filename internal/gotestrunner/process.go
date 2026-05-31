package gotestrunner

import (
	"os"
	"os/exec"
	"time"
)

const (
	// GracefulShutdownDelay is the time a test process has to exit after
	// receiving SIGTERM before it is forcibly killed. Must cover the
	// longest fixture teardown (FixtureConfig.Timeout up to 5 min for
	// container fixtures, SuiteConfig.SetupTimeout up to 5 min for AfterAll).
	GracefulShutdownDelay = 5*time.Minute + 30*time.Second

	// BuildShutdownDelay is the WaitDelay for build/compile commands that
	// have no teardown work.
	BuildShutdownDelay = 10 * time.Second
)

func setProcessGroup(cmd *exec.Cmd) {
	setProcessGroupAttr(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := TerminateProcessGroup(cmd.Process.Pid); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	cmd.WaitDelay = GracefulShutdownDelay
}

func setBuildProcessGroup(cmd *exec.Cmd) {
	setProcessGroupAttr(cmd)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := ForceKillProcessGroup(cmd.Process.Pid); err != nil {
			return os.ErrProcessDone
		}
		return nil
	}
	cmd.WaitDelay = BuildShutdownDelay
}
