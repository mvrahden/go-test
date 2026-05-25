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

// SetProcessGroup configures cmd to run in its own process group and
// to receive SIGTERM (then SIGKILL after GracefulShutdownDelay) when
// its associated context is cancelled.
func SetProcessGroup(cmd *exec.Cmd) {
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

// SetBuildProcessGroup configures cmd to run in its own process group
// with immediate kill on cancellation. Build processes (compilers,
// linkers) have no cleanup work, so graceful shutdown is unnecessary.
func SetBuildProcessGroup(cmd *exec.Cmd) {
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
