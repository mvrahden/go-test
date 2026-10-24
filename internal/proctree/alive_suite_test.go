package proctree_test

import (
	"os"
	"os/exec"

	"github.com/mvrahden/go-test/internal/proctree"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// AliveTestSuite covers telling a running process from one that is gone.
type AliveTestSuite struct{}

func (s *AliveTestSuite) SuiteConfig() gotest.SuiteConfig {
	cfg := gotest.DefaultSuiteConfig()
	cfg.Parallel = true
	return cfg
}

func (s *AliveTestSuite) TestAlive(t *gotest.T) {
	t.It("reports this process as running", func(it *gotest.T) {
		gotest.True(it, proctree.Alive(os.Getpid()))
	})

	t.It("reports a process that exited and was reaped as gone", func(it *gotest.T) {
		cmd := exec.Command(os.Args[0]) //nolint:gosec // G204: this test binary
		cmd.Env = childEnv("exit")
		gotest.NoError(it, cmd.Run())
		gotest.False(it, proctree.Alive(cmd.Process.Pid))
	})
}
