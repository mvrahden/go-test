package gotestrunner

import (
	"os/exec"
	"syscall"
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = GracefulShutdownDelay
}
