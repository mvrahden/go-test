// Package orphan starts a process the test never waits for.
package orphan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// OrphanTestSuite leaves a sleeping process behind on purpose.
type OrphanTestSuite struct{}

func (s *OrphanTestSuite) TestLeavesAProcess(t *gotest.T) {
	cmd := exec.Command("sleep", "4242")
	gotest.NoError(t, cmd.Start())
	// Its pid, so the check ends this process alone should it outlive the run.
	pidFile := filepath.Join(os.Getenv("GOTEST_CANARY_DIR"), "orphan-pid")
	gotest.NoError(t, os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600))
}
