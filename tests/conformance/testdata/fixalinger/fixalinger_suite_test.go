package fixalinger

import (
	"fmt"
	"os/exec"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// LingerTestSuite returns while a process it started still runs.
type LingerTestSuite struct {
	A *ASharedFixture
}

func (s *LingerTestSuite) TestLeavesAChild(t *gotest.T) {
	gotest.Equal(t, "up", s.A.State)
	child := exec.Command("sh", "-c", "while :; do sleep 0.1; done")
	gotest.NoError(t, child.Start())
	mark(fmt.Sprintf("child=%d", child.Process.Pid))
	mark("suite returns")
}
