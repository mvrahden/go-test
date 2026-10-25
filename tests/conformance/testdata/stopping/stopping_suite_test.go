package stopping

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// StopTestSuite holds in its first method; the two after it never start when
// the run is stopped meanwhile.
type StopTestSuite struct {
	Hold *HoldFixture
}

func (s *StopTestSuite) BeforeAll(t *gotest.T) { mark("suite.BeforeAll") }
func (s *StopTestSuite) AfterAll(t *gotest.T)  { mark("suite.AfterAll") }

func (s *StopTestSuite) TestHolds(t *gotest.T) {
	if os.Getenv("STOPPING_CHILD") == "1" {
		// A process of the test's own, left running when the stop comes; it
		// marks an interrupt, which a kill gives it no chance to.
		child := exec.Command("sh", "-c", `trap 'echo "child interrupted" >> "$GOTEST_CANARY_DIR/markers"; exit 0' INT TERM; while :; do sleep 0.1; done`)
		if child.Start() == nil {
			mark(fmt.Sprintf("child=%d", child.Process.Pid))
		}
	}
	mark("running")
	if os.Getenv("STOPPING_IGNORE_CONTEXT") == "1" {
		// Ignores the stop, until something forces it.
		time.Sleep(time.Minute)
	}
	<-t.Context().Done()
	mark("context ended")
}

func (s *StopTestSuite) TestSecond(t *gotest.T) { mark("second") }
func (s *StopTestSuite) TestThird(t *gotest.T)  { mark("third") }
