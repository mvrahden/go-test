package stopping

import (
	"os"
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
