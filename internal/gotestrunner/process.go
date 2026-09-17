package gotestrunner

import (
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/internal/proctree"
)

// GracefulShutdownDelay is the time a test process has to exit after it is
// asked to shut down before it is forcibly killed. Must cover the longest
// fixture teardown (FixtureConfig.Timeout up to 5 min for container fixtures,
// SuiteConfig.SetupTimeout up to 5 min for AfterAll).
const GracefulShutdownDelay = 5*time.Minute + 30*time.Second

// runTree runs cmd as the root of its own process tree. A canceled context
// asks the tree to shut down, and GracefulShutdownDelay later kills the root.
func runTree(cmd *exec.Cmd) error {
	tree := proctree.New(cmd)
	cmd.WaitDelay = GracefulShutdownDelay
	defer tree.Release()
	if err := tree.Start(); err != nil {
		return err
	}
	return cmd.Wait()
}
