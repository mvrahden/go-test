package stopgrace

import (
	"os"
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
)

// GraceTestSuite holds in its first method until the run is stopped.
type GraceTestSuite struct {
	Hold *HoldFixture
}

func (s *GraceTestSuite) AfterAll(t *gotest.T) { mark("suite.AfterAll") }

func (s *GraceTestSuite) AfterEach(t *gotest.T) {
	if os.Getenv("STOPGRACE_SLOW_AFTEREACH") != "1" {
		return
	}
	mark("AfterEach began")
	time.Sleep(7 * time.Second)
	mark("AfterEach ended")
}

func (s *GraceTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	if os.Getenv("STOPGRACE_IGNORE") == "1" {
		time.Sleep(60 * time.Second)
		mark("body returned")
		return
	}
	<-t.Context().Done()
	mark("context ended")
}

func (s *GraceTestSuite) TestSecond(t *gotest.T) { mark("second") }

// SetupHangTestSuite's BeforeAll ignores its context, so only cutting it
// short ends a stopped run.
type SetupHangTestSuite struct {
	Hold *HoldFixture
}

func (s *SetupHangTestSuite) BeforeAll(t *gotest.T) {
	mark("running")
	time.Sleep(60 * time.Second)
	mark("BeforeAll returned")
}

func (s *SetupHangTestSuite) AfterAll(t *gotest.T)      { mark("suite.AfterAll") }
func (s *SetupHangTestSuite) TestNeverRuns(t *gotest.T) { mark("method ran") }
