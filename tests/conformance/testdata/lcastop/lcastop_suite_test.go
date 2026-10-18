package lcastop

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
	if os.Getenv("LCA_SLOW_AFTEREACH") != "1" {
		return
	}
	mark("AfterEach began")
	time.Sleep(7 * time.Second)
	mark("AfterEach ended")
}

func (s *GraceTestSuite) TestHolds(t *gotest.T) {
	mark("running")
	if os.Getenv("LCA_IGNORE") == "1" {
		time.Sleep(60 * time.Second)
		mark("body returned")
		return
	}
	<-t.Context().Done()
	mark("context ended")
}

func (s *GraceTestSuite) TestSecond(t *gotest.T) { mark("second") }

// AsyncHoldTestSuite's async method has a helper goroutine that fails at
// once while the owner of done never calls it; only a stop ends the wait.
type AsyncHoldTestSuite struct{}

func (s *AsyncHoldTestSuite) AfterEach(t *gotest.T) { mark("async AfterEach") }
func (s *AsyncHoldTestSuite) AfterAll(t *gotest.T)  { mark("async AfterAll") }

func (s *AsyncHoldTestSuite) TestWaitsAsync(t *gotest.T, done func()) {
	gotest.Go(t, func() { gotest.Fail(t, "a helper goroutine fails") })
	go func() {
		time.Sleep(200 * time.Millisecond)
		mark("running")
		select {}
	}()
}
