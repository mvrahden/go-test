package slow_test

import (
	"time"

	"github.com/mvrahden/go-test/pkg/gotest"
	"github.com/mvrahden/go-test/tests/conformance/testdata/fixaneverrun/fixtures"
)

// SlowTestSuite reads no fixture and runs until the fixture is released, or
// for 20s when it is held to the end of the run.
type SlowTestSuite struct{}

func (s *SlowTestSuite) TestTakesItsTime(t *gotest.T) {
	fixtures.Await("a.AfterAll", 20*time.Second)
	fixtures.Mark("slow ends")
}
