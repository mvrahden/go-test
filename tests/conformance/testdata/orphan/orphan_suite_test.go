// Package orphan starts a process the test never waits for.
package orphan

import (
	"os/exec"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// OrphanTestSuite leaves a sleeping process behind on purpose.
type OrphanTestSuite struct{}

func (s *OrphanTestSuite) TestLeavesAProcess(t *gotest.T) {
	gotest.NoError(t, exec.Command("sleep", "4242").Start())
}
