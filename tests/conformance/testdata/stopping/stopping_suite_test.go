package stopping

import "github.com/mvrahden/go-test/pkg/gotest"

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
}

func (s *StopTestSuite) TestSecond(t *gotest.T) { mark("second") }
func (s *StopTestSuite) TestThird(t *gotest.T)  { mark("third") }
