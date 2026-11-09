package stopping

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"testing"
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
		child := exec.Command("sh", "-c", `trap 'echo "child interrupted" >> "$GOTEST_CANARY_DIR/markers"; exit 0' INT TERM; echo ready; while :; do sleep 0.1; done`)
		if startTrapped(child) {
			mark(fmt.Sprintf("child=%d", child.Process.Pid))
		}
	}
	mark("running")
	<-t.Context().Done()
	mark("context ended")
	// AfterAll waits for this return; a release the stop began early is
	// marked before it, every time.
	time.Sleep(500 * time.Millisecond)
}

func (s *StopTestSuite) TestSecond(t *gotest.T) { mark("second") }
func (s *StopTestSuite) TestThird(t *gotest.T)  { mark("third") }

// TestMain keeps the process up for a while after the tests when
// STOPPING_LINGER is set, as a TestMain releasing what it set up would, and
// marks its end, which a signal that ended the process never reaches.
func TestMain(m *testing.M) {
	code := m.Run()
	if os.Getenv("STOPPING_LINGER") == "1" {
		time.Sleep(2 * time.Second)
		mark("lingered")
	}
	os.Exit(code)
}

// startTrapped starts child, a shell that prints a line once its traps are
// set, and returns once it has: a signal sent before then finds no trap and
// ends the shell unmarked.
func startTrapped(child *exec.Cmd) bool {
	out, err := child.StdoutPipe()
	if err != nil || child.Start() != nil {
		return false
	}
	_, err = bufio.NewReader(out).ReadString('\n')
	return err == nil
}
